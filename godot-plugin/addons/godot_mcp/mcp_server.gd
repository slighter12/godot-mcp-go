@tool
extends Node
# MCP 2026-07-28 Streamable HTTP client.

const VARIANT_UTILS := preload("res://addons/godot_mcp/variant_utils.gd")

signal connected
signal disconnected
signal error(error: String)
signal message_received(message: Dictionary)

const DEFAULT_PROTOCOL_VERSION := "2026-07-28"
const PLUGIN_CLIENT_VERSION := "0.3.0"
const GODOT_EXTENSION_ID := "com.slighter12/godot-mcp"
const COMMAND_STREAM_EXTENSION_ID := "com.slighter12/godot-mcp-command-stream"
const SSE_RECONNECT_DELAY_MS: int = 1500

var streamable_http_url: String = "http://localhost:9080/mcp"
var is_connecting: bool = false
var post_http_connection: HTTPRequest
var editor_session_id: String = ""
var negotiated_protocol_version: String = DEFAULT_PROTOCOL_VERSION

var is_connected: bool = false
var request_in_flight: bool = false
var pending_messages: Array[Dictionary] = []
var pending_connect_url: String = ""
var ignore_post_result_once: bool = false

var subscription_http_connection: HTTPClient
var subscription_request_sent: bool = false
var subscription_response_validated: bool = false
var subscription_reconnect_at_msec: int = 0
var subscription_request_path: String = "/mcp"
var subscription_id: String = ""
var subscription_line_buffer: String = ""
var subscription_event_name: String = "message"
var subscription_data_lines: Array[String] = []
var subscription_should_run: bool = false
var sequence_counter: int = 0

func _ready() -> void:
	_ensure_editor_session_id()
	load_settings()
	post_http_connection = HTTPRequest.new()
	add_child(post_http_connection)
	post_http_connection.request_completed.connect(_on_post_request_completed)
	set_process(false)

func _exit_tree() -> void:
	disconnect_from_server()

func load_settings() -> void:
	var config := ConfigFile.new()
	var err := config.load("res://addons/godot_mcp/config.cfg")
	if err == OK:
		streamable_http_url = str(config.get_value("mcp", "streamable_http_url", streamable_http_url)).strip_edges()

func connect_to_server() -> void:
	connect_streamable_http(streamable_http_url)

func connect_streamable_http(url: String) -> void:
	if is_connecting or request_in_flight:
		pending_connect_url = url
		return
	if is_connected:
		_mark_disconnected()
	streamable_http_url = url
	is_connecting = true
	negotiated_protocol_version = DEFAULT_PROTOCOL_VERSION
	_drop_pending_messages("reconnecting to MCP server")
	var discover := {
		"jsonrpc": "2.0",
		"id": "discover-%s" % editor_session_id,
		"method": "server/discover",
		"params": {"_meta": _build_request_meta()}
	}
	_send_raw_message(discover)

func send_message(message: Dictionary) -> bool:
	if is_connecting or request_in_flight:
		pending_messages.append(message.duplicate(true))
		return true
	return _send_raw_message(message)

func _send_raw_message(message: Dictionary) -> bool:
	if post_http_connection == null:
		emit_signal("error", "MCP HTTP client is not initialized")
		return false
	var prepared := _prepare_request(message)
	var json_message := JSON.stringify(prepared)
	var headers := PackedStringArray([
		"Content-Type: application/json",
		"Accept: application/json, text/event-stream",
		"MCP-Protocol-Version: " + negotiated_protocol_version,
		"Mcp-Method: " + str(prepared.get("method", ""))
	])
	var name := _request_name(prepared)
	if name != "":
		headers.append("Mcp-Name: " + name)
	request_in_flight = true
	ignore_post_result_once = false
	var request_err := post_http_connection.request(streamable_http_url, headers, HTTPClient.METHOD_POST, json_message)
	if request_err != OK:
		request_in_flight = false
		emit_signal("error", "Failed to send MCP request: " + str(request_err))
		return false
	return true

func _prepare_request(message: Dictionary) -> Dictionary:
	var prepared := message.duplicate(true)
	var method := str(prepared.get("method", ""))
	if method.begins_with("notifications/"):
		return prepared
	var params := prepared.get("params", {})
	if not (params is Dictionary):
		params = {}
	var params_copy: Dictionary = params.duplicate(true)
	var meta := params_copy.get("_meta", {})
	if not (meta is Dictionary):
		meta = {}
	var meta_copy: Dictionary = meta.duplicate(true)
	var base_meta := _build_request_meta()
	for key in base_meta.keys():
		if not meta_copy.has(key):
			meta_copy[key] = base_meta[key]
	params_copy["_meta"] = meta_copy
	prepared["params"] = params_copy
	return prepared

func _build_request_meta() -> Dictionary:
	return {
		"io.modelcontextprotocol/protocolVersion": negotiated_protocol_version,
		"io.modelcontextprotocol/clientInfo": {
			"name": "godot-mcp",
			"version": PLUGIN_CLIENT_VERSION
		},
		"io.modelcontextprotocol/clientCapabilities": {
			"extensions": {
				GODOT_EXTENSION_ID: {
					"version": "1",
					"role": "editor",
					"editor_session_id": editor_session_id,
					# The editor client supports mutating tools; the server still
					# enforces per-tool permissions and capability checks.
					"mutating": true
				},
				COMMAND_STREAM_EXTENSION_ID: {"version": "1"}
			}
		}
	}

func _request_name(message: Dictionary) -> String:
	var method := str(message.get("method", ""))
	var params := message.get("params", {})
	if not (params is Dictionary):
		return ""
	if method == "tools/call" or method == "prompts/get":
		return str(params.get("name", "")).strip_edges()
	if method == "resources/read":
		return str(params.get("uri", "")).strip_edges()
	return ""

func _on_post_request_completed(result: int, response_code: int, headers: PackedStringArray, body: PackedByteArray) -> void:
	request_in_flight = false
	if ignore_post_result_once:
		ignore_post_result_once = false
		_flush_reconnect()
		_flush_pending_messages()
		return
	if result != HTTPRequest.RESULT_SUCCESS:
		if is_connecting:
			_fail_connect("MCP request failed: " + str(result))
		else:
			emit_signal("error", "MCP request failed: " + str(result))
		_flush_reconnect()
		return
	if is_connecting:
		_handle_discover_response(response_code, body)
	else:
		_handle_post_response(response_code, headers, body)
	_flush_reconnect()
	_flush_pending_messages()

func _handle_discover_response(response_code: int, body: PackedByteArray) -> void:
	if response_code != 200:
		_fail_connect("server/discover failed with status: " + str(response_code))
		return
	var response := _parse_json_payload(body.get_string_from_utf8())
	if not (response is Dictionary) or response.get("jsonrpc", "") != "2.0" or response.get("error", null) != null:
		_fail_connect("Invalid server/discover response")
		return
	is_connecting = false
	is_connected = true
	emit_signal("connected")
	_start_command_subscription()

func _handle_post_response(response_code: int, headers: PackedStringArray, body: PackedByteArray) -> void:
	if response_code != 200 and response_code != 202:
		emit_signal("error", "MCP request failed with status: " + str(response_code))
		return
	if body.is_empty():
		return
	var content_type := _extract_header_value(headers, "content-type")
	if content_type.find("text/event-stream") != -1:
		_process_sse_payload(body.get_string_from_utf8())
		return
	var payload := _parse_json_payload(body.get_string_from_utf8())
	if payload == null:
		emit_signal("error", "Failed to parse MCP response JSON")
		return
	_emit_jsonrpc_payload(payload)

func _start_command_subscription() -> void:
	if not is_connected:
		return
	subscription_should_run = true
	subscription_reconnect_at_msec = 0
	_reset_subscription_parser_state()
	_open_subscription_connection()
	set_process(true)

func _stop_command_subscription() -> void:
	subscription_should_run = false
	subscription_request_sent = false
	subscription_response_validated = false
	subscription_reconnect_at_msec = 0
	_reset_subscription_parser_state()
	if subscription_http_connection:
		subscription_http_connection.close()
	subscription_http_connection = null
	set_process(false)

func _open_subscription_connection() -> void:
	if not subscription_should_run:
		return
	var endpoint := _parse_http_endpoint(streamable_http_url)
	if endpoint.is_empty():
		_handle_subscription_disconnect("Invalid MCP URL")
		return
	if subscription_http_connection:
		subscription_http_connection.close()
	subscription_http_connection = HTTPClient.new()
	subscription_request_sent = false
	subscription_response_validated = false
	_reset_subscription_parser_state()
	subscription_request_path = str(endpoint.get("path", "/mcp"))
	var host := str(endpoint.get("host", ""))
	var port := int(endpoint.get("port", 0))
	var connect_err: int
	if VARIANT_UTILS.to_bool(endpoint.get("use_tls", false), false):
		connect_err = subscription_http_connection.connect_to_host(host, port, TLSOptions.client())
	else:
		connect_err = subscription_http_connection.connect_to_host(host, port)
	if connect_err != OK:
		_handle_subscription_disconnect("Failed to open command subscription: " + str(connect_err))

func _poll_subscription() -> void:
	if not subscription_should_run:
		return
	if not subscription_http_connection:
		if subscription_reconnect_at_msec == 0 or Time.get_ticks_msec() >= subscription_reconnect_at_msec:
			_open_subscription_connection()
		return
	var poll_err := subscription_http_connection.poll()
	if poll_err != OK:
		_handle_subscription_disconnect("Subscription poll failed: " + str(poll_err))
		return
	match subscription_http_connection.get_status():
		HTTPClient.STATUS_CONNECTED:
			if not subscription_request_sent:
				_send_subscription_request()
		HTTPClient.STATUS_BODY:
			if _validate_subscription_response():
				_read_subscription_chunks()
		HTTPClient.STATUS_DISCONNECTED, HTTPClient.STATUS_CANT_RESOLVE, HTTPClient.STATUS_CANT_CONNECT, HTTPClient.STATUS_CONNECTION_ERROR:
			_handle_subscription_disconnect("Command subscription disconnected")
		_:
			pass

func _send_subscription_request() -> void:
	if not subscription_http_connection:
		return
	subscription_id = "subscription-%s" % _next_sequence()
	var body := {
		"jsonrpc": "2.0",
		"id": subscription_id,
		"method": "subscriptions/listen",
		"params": {
			"_meta": _build_request_meta_with_command_extension(),
			"notifications": {
				COMMAND_STREAM_EXTENSION_ID: {"command": true}
			}
		}
	}
	var headers := PackedStringArray([
		"Content-Type: application/json",
		"Accept: application/json, text/event-stream",
		"MCP-Protocol-Version: " + negotiated_protocol_version,
		"Mcp-Method: subscriptions/listen"
	])
	var request_err := subscription_http_connection.request(HTTPClient.METHOD_POST, subscription_request_path, headers, JSON.stringify(body))
	if request_err != OK:
		_handle_subscription_disconnect("Failed to send command subscription: " + str(request_err))
		return
	subscription_request_sent = true

func _build_request_meta_with_command_extension() -> Dictionary:
	var meta := _build_request_meta()
	meta[COMMAND_STREAM_EXTENSION_ID] = {
		"version": "1",
		"editor_session_id": editor_session_id,
		"events": ["command"]
	}
	return meta

func _validate_subscription_response() -> bool:
	if subscription_response_validated:
		return true
	subscription_response_validated = true
	if not subscription_http_connection or subscription_http_connection.get_response_code() != 200:
		_handle_subscription_disconnect("Command subscription returned an unexpected status")
		return false
	var content_type := _extract_header_value(subscription_http_connection.get_response_headers(), "content-type")
	if content_type.find("text/event-stream") == -1:
		_handle_subscription_disconnect("Command subscription returned unexpected content type")
		return false
	return true

func _read_subscription_chunks() -> void:
	if not subscription_http_connection:
		return
	while true:
		var chunk := subscription_http_connection.read_response_body_chunk()
		if chunk.is_empty():
			break
		_process_subscription_chunk(chunk.get_string_from_utf8())

func _process_subscription_chunk(chunk_text: String) -> void:
	if chunk_text == "":
		return
	subscription_line_buffer += chunk_text
	var lines: PackedStringArray = subscription_line_buffer.split("\n")
	if not subscription_line_buffer.ends_with("\n"):
		subscription_line_buffer = lines[lines.size() - 1] if not lines.is_empty() else ""
		if not lines.is_empty():
			lines.resize(lines.size() - 1)
	else:
		subscription_line_buffer = ""
	for raw_line in lines:
		var line := str(raw_line).trim_suffix("\r")
		_consume_subscription_line(line)

func _consume_subscription_line(line: String) -> void:
	if line.begins_with(":"):
		return
	if line == "":
		_dispatch_sse_event(subscription_event_name, subscription_data_lines)
		subscription_event_name = "message"
		subscription_data_lines.clear()
		return
	if line.begins_with("event:"):
		subscription_event_name = line.substr(6).strip_edges()
	elif line.begins_with("data:"):
		subscription_data_lines.append(line.substr(5).strip_edges())

func _reset_subscription_parser_state() -> void:
	subscription_line_buffer = ""
	subscription_event_name = "message"
	subscription_data_lines.clear()

func _dispatch_sse_event(_event_name: String, data_lines: Array[String]) -> void:
	if data_lines.is_empty():
		return
	var payload_text := "\n".join(data_lines)
	var payload := _parse_json_payload(payload_text)
	if payload != null:
		_emit_jsonrpc_payload(payload)

func _handle_subscription_disconnect(reason: String) -> void:
	if subscription_http_connection:
		subscription_http_connection.close()
	subscription_http_connection = null
	subscription_request_sent = false
	subscription_response_validated = false
	_reset_subscription_parser_state()
	if not subscription_should_run or not is_connected:
		return
	emit_signal("error", reason)
	subscription_reconnect_at_msec = Time.get_ticks_msec() + SSE_RECONNECT_DELAY_MS

func _process(_delta: float) -> void:
	_poll_subscription()

func _emit_jsonrpc_payload(payload: Variant) -> void:
	if payload is Dictionary:
		if payload.get("jsonrpc", "") == "2.0":
			emit_signal("message_received", payload)
	elif payload is Array:
		for item in payload:
			if item is Dictionary and item.get("jsonrpc", "") == "2.0":
				emit_signal("message_received", item)

func _parse_json_payload(payload_text: String) -> Variant:
	var text := payload_text.strip_edges()
	if text == "":
		return null
	var json := JSON.new()
	if json.parse(text) != OK:
		return null
	return json.get_data()

func _parse_http_endpoint(url: String) -> Dictionary:
	var trimmed := url.strip_edges()
	var use_tls := false
	var remainder := ""
	if trimmed.begins_with("https://"):
		use_tls = true
		remainder = trimmed.substr(8)
	elif trimmed.begins_with("http://"):
		remainder = trimmed.substr(7)
	else:
		return {}
	var host_port := remainder
	var path := "/"
	var path_index := remainder.find("/")
	var query_index := remainder.find("?")
	if path_index == -1 or (query_index != -1 and query_index < path_index):
		path_index = query_index
	if path_index != -1:
		host_port = remainder.substr(0, path_index)
		path = "/" + remainder.substr(path_index) if remainder[path_index] == "?" else remainder.substr(path_index)
	if host_port == "" or host_port.find("@") != -1:
		return {}
	var host := host_port
	var port := 443 if use_tls else 80
	if host_port.begins_with("["):
		var close_bracket := host_port.find("]")
		if close_bracket <= 1:
			return {}
		host = host_port.substr(1, close_bracket - 1)
		var suffix := host_port.substr(close_bracket + 1)
		if suffix != "":
			if not suffix.begins_with(":"):
				return {}
			var bracket_port := suffix.substr(1)
			if not bracket_port.is_valid_int():
				return {}
			port = int(bracket_port)
	else:
		if host_port.count(":") > 1:
			return {}
		var colon := host_port.find(":")
		if colon != -1:
			host = host_port.substr(0, colon)
			var host_port_text := host_port.substr(colon + 1)
			if not host_port_text.is_valid_int():
				return {}
			port = int(host_port_text)
	if host == "" or port <= 0 or port > 65535:
		return {}
	return {"host": host, "port": port, "use_tls": use_tls, "path": path}

func _extract_header_value(headers: PackedStringArray, header_name_lower: String) -> String:
	for header in headers:
		var lower := header.to_lower()
		if lower.begins_with(header_name_lower + ":"):
			var parts := header.split(":", false, 1)
			if parts.size() == 2:
				return parts[1].strip_edges().to_lower()
	return ""

func _ensure_editor_session_id() -> void:
	if editor_session_id != "":
		return
	editor_session_id = "editor-%s-%s" % [str(Time.get_unix_time_from_system()), str(randi())]

func _next_sequence() -> int:
	sequence_counter += 1
	return sequence_counter

func _fail_connect(message: String) -> void:
	is_connecting = false
	request_in_flight = false
	_mark_disconnected()
	_drop_pending_messages("MCP discovery failed")
	emit_signal("error", message)

func disconnect_from_server() -> void:
	is_connecting = false
	pending_connect_url = ""
	if request_in_flight and post_http_connection:
		ignore_post_result_once = true
		post_http_connection.cancel_request()
	request_in_flight = false
	_drop_pending_messages("plugin disconnect requested")
	_mark_disconnected()

func _mark_disconnected() -> void:
	_stop_command_subscription()
	if is_connected:
		is_connected = false
		emit_signal("disconnected")

func _flush_pending_messages() -> void:
	if is_connecting or request_in_flight or pending_connect_url != "" or pending_messages.is_empty():
		return
	var next_message: Dictionary = pending_messages.pop_front()
	if not _send_raw_message(next_message):
		_drop_pending_messages("failed to send queued MCP request")

func _flush_reconnect() -> void:
	if pending_connect_url == "":
		return
	var next_url := pending_connect_url
	pending_connect_url = ""
	connect_streamable_http(next_url)

func _drop_pending_messages(reason: String) -> void:
	var dropped := pending_messages.size()
	if dropped > 0:
		pending_messages.clear()
		emit_signal("error", "Dropped %d queued MCP request(s): %s" % [dropped, reason])
