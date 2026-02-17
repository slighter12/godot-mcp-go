@tool
extends Node
# NOTE: this node acts as an MCP client.
# The filename is kept for backward compatibility with existing plugin installs.

signal connected
signal disconnected
signal error(error: String)
signal message_received(message: Dictionary)

const DEFAULT_PROTOCOL_VERSION := "2025-11-25"
const SSE_RECONNECT_DELAY_MS: int = 1500

var streamable_http_url: String = "http://localhost:9080/mcp"
var is_connecting: bool = false
var post_http_connection: HTTPRequest
var session_id: String = ""
var negotiated_protocol_version: String = DEFAULT_PROTOCOL_VERSION

var is_connected: bool = false
var request_in_flight: bool = false
var pending_messages: Array[Dictionary] = []
var pending_connect_url: String = ""
var ignore_post_result_once: bool = false

var sse_http_connection: HTTPClient
var sse_should_run: bool = false
var sse_request_sent: bool = false
var sse_response_validated: bool = false
var sse_reconnect_at_msec: int = 0
var sse_request_path: String = "/mcp"
var sse_line_buffer: String = ""
var sse_event_name: String = "message"
var sse_data_lines: Array[String] = []

func _ready() -> void:
    print("MCP Client: Initializing...")
    load_settings()

    post_http_connection = HTTPRequest.new()
    add_child(post_http_connection)
    post_http_connection.request_completed.connect(_on_post_request_completed)
    set_process(false)

func _exit_tree() -> void:
    disconnect_from_server()

func load_settings() -> void:
    print("MCP Client: Loading settings...")
    var config = ConfigFile.new()
    var err = config.load("res://addons/godot_mcp/config.cfg")
    if err == OK:
        streamable_http_url = config.get_value("mcp", "streamable_http_url", "http://localhost:9080/mcp")
        var configured_type = config.get_value("mcp", "connection_type", "streamable_http")
        if configured_type != "streamable_http":
            print("MCP Client: connection_type '%s' is unsupported in the plugin. Using streamable_http." % configured_type)
        print("MCP Client: Settings loaded - type: streamable_http, url: ", streamable_http_url)
    else:
        print("MCP Client: Failed to load settings, using defaults")

func connect_to_server() -> void:
    print("MCP Client: Attempting to connect...")
    connect_streamable_http(streamable_http_url)

func connect_streamable_http(url: String) -> void:
    if ignore_post_result_once:
        pending_connect_url = url
        print("MCP Client: Waiting for canceled request callback, queued reconnect to ", url)
        return

    if is_connecting:
        pending_connect_url = url
        print("MCP Client: Connection attempt in progress, queued reconnect to ", url)
        return
    if request_in_flight:
        pending_connect_url = url
        print("MCP Client: Request in flight, queued reconnect to ", url)
        return

    if is_connected:
        _mark_disconnected()
    else:
        _stop_sse_stream()

    streamable_http_url = url
    is_connecting = true
    session_id = ""
    negotiated_protocol_version = DEFAULT_PROTOCOL_VERSION
    _drop_pending_messages("reconnecting to a new MCP session")
    print("MCP Client: Connecting to Streamable HTTP (POST + SSE) at ", url)

    var init_message = {
        "jsonrpc": "2.0",
        "id": "init",
        "method": "initialize",
        "params": {
            "protocolVersion": negotiated_protocol_version,
            "capabilities": {},
            "clientInfo": {
                "name": "godot-mcp",
                "version": "0.1.0"
            }
        }
    }

    _send_raw_message(init_message)

func _on_post_request_completed(result: int, response_code: int, headers: PackedStringArray, body: PackedByteArray) -> void:
    request_in_flight = false

    if ignore_post_result_once:
        ignore_post_result_once = false
        _flush_reconnect()
        _flush_pending_messages()
        return

    if result != HTTPRequest.RESULT_SUCCESS:
        if is_connecting:
            _fail_connect("Streamable HTTP request failed: " + str(result))
        else:
            _mark_disconnected()
            emit_signal("error", "Streamable HTTP request failed: " + str(result))
            _drop_pending_messages("connection failure")
            _flush_reconnect()
        return

    if is_connecting:
        _handle_initialize_response(response_code, headers, body)
        _flush_reconnect()
        _flush_pending_messages()
        return

    _handle_post_response(response_code, headers, body)
    _flush_reconnect()
    _flush_pending_messages()

func _handle_initialize_response(response_code: int, headers: PackedStringArray, body: PackedByteArray) -> void:
    print("MCP Client: Initialize response received - code: ", response_code)
    if response_code != 200:
        _fail_connect("Streamable HTTP initialize failed with status: " + str(response_code))
        return

    var response = _parse_json_payload(body.get_string_from_utf8())
    if not (response is Dictionary):
        _fail_connect("Invalid initialize response payload")
        return

    if response.get("jsonrpc", "") != "2.0":
        _fail_connect("Invalid initialize response: missing jsonrpc 2.0")
        return

    if response.get("id", null) != "init":
        _fail_connect("Invalid initialize response: mismatched id")
        return

    if response.has("error"):
        var err_obj = response.get("error", {})
        if err_obj is Dictionary:
            _fail_connect("Initialize failed: " + str(err_obj.get("message", "unknown error")))
        else:
            _fail_connect("Initialize failed")
        return

    var result_payload = response.get("result", null)
    if not (result_payload is Dictionary):
        _fail_connect("Invalid initialize response format")
        return

    var negotiated = result_payload.get("protocolVersion", "")
    if negotiated is String and negotiated != "":
        negotiated_protocol_version = negotiated

    session_id = _extract_session_id(headers)
    if session_id == "":
        print("MCP Client: Initialize response has no MCP-Session-Id (stateless mode)")
    else:
        print("MCP Client: Session ID received: ", session_id)

    print("MCP Client: Initialization successful")
    is_connecting = false
    is_connected = true
    emit_signal("connected")
    _start_sse_stream()

func _handle_post_response(response_code: int, headers: PackedStringArray, body: PackedByteArray) -> void:
    print("MCP Client: Streamable HTTP response received - code: ", response_code)
    var had_session = session_id != ""

    var latest_session_id = _extract_session_id(headers)
    if latest_session_id != "":
        session_id = latest_session_id

    if response_code != 200 and response_code != 202:
        if response_code == 404 and had_session:
            print("MCP Client: Session not found (404), reinitializing Streamable HTTP session")
            session_id = ""
            connect_streamable_http(streamable_http_url)
            return
        _mark_disconnected()
        emit_signal("error", "Streamable HTTP request failed with status: " + str(response_code))
        return

    if body.is_empty():
        return

    var content_type = _extract_header_value(headers, "content-type")
    if content_type.find("text/event-stream") != -1:
        _process_sse_payload(body.get_string_from_utf8())
    else:
        var payload = _parse_json_payload(body.get_string_from_utf8())
        if payload == null:
            emit_signal("error", "Failed to parse response JSON")
            return
        _emit_jsonrpc_payload(payload)

func send_message(message: Dictionary) -> bool:
    if is_connecting:
        pending_messages.append(message)
        return true

    if request_in_flight:
        pending_messages.append(message)
        return true

    return _send_raw_message(message)

func _send_raw_message(message: Dictionary) -> bool:
    print("MCP Client: Sending message: ", message)
    if not post_http_connection:
        emit_signal("error", "Streamable HTTP connection is not initialized")
        return false

    var json_message = JSON.stringify(message)
    var headers = [
        "Content-Type: application/json",
        "Accept: application/json, text/event-stream",
        "MCP-Protocol-Version: " + negotiated_protocol_version
    ]
    if session_id != "":
        headers.append("MCP-Session-Id: " + session_id)

    request_in_flight = true
    ignore_post_result_once = false
    var request_err = post_http_connection.request(streamable_http_url, headers, HTTPClient.METHOD_POST, json_message)
    if request_err != OK:
        request_in_flight = false
        if is_connecting:
            _fail_connect("Failed to send initialize request: " + str(request_err))
        else:
            _mark_disconnected()
            emit_signal("error", "Failed to send MCP message: " + str(request_err))
            _flush_reconnect()
        return false
    return true

func _flush_pending_messages() -> void:
    if is_connecting or request_in_flight or pending_connect_url != "":
        return
    if pending_messages.is_empty():
        return

    var next_message: Dictionary = pending_messages.pop_front()
    if not _send_raw_message(next_message):
        _drop_pending_messages("failed to send queued MCP message")

func _flush_reconnect() -> void:
    if pending_connect_url == "":
        return
    var next_url = pending_connect_url
    pending_connect_url = ""
    connect_streamable_http(next_url)

func _parse_json_payload(payload_text: String) -> Variant:
    var text = payload_text.strip_edges()
    if text == "":
        return null

    var json = JSON.new()
    var err = json.parse(text)
    if err != OK:
        return null
    return json.get_data()

func _emit_jsonrpc_payload(payload: Variant) -> void:
    if payload is Dictionary:
        _emit_jsonrpc_object(payload)
        return

    if payload is Array:
        for item in payload:
            if item is Dictionary:
                _emit_jsonrpc_object(item)

func _emit_jsonrpc_object(message: Dictionary) -> void:
    if message.get("jsonrpc", "") != "2.0":
        return
    emit_signal("message_received", message)

func _process(_delta: float) -> void:
    _poll_sse_stream()

func _start_sse_stream() -> void:
    if not is_connected:
        return
    sse_should_run = true
    sse_reconnect_at_msec = 0
    _reset_sse_parser_state()
    _open_sse_stream_connection()
    set_process(true)

func _stop_sse_stream() -> void:
    sse_should_run = false
    sse_request_sent = false
    sse_response_validated = false
    sse_reconnect_at_msec = 0
    _reset_sse_parser_state()
    if sse_http_connection:
        sse_http_connection.close()
    sse_http_connection = null
    set_process(false)

func _poll_sse_stream() -> void:
    if not sse_should_run:
        return

    if not sse_http_connection:
        if sse_reconnect_at_msec == 0 or Time.get_ticks_msec() >= sse_reconnect_at_msec:
            _open_sse_stream_connection()
        return

    var poll_err = sse_http_connection.poll()
    if poll_err != OK:
        _handle_sse_disconnect("SSE poll failed: " + str(poll_err))
        return

    var status = sse_http_connection.get_status()
    match status:
        HTTPClient.STATUS_CONNECTED:
            if not sse_request_sent:
                _send_sse_get_request()
        HTTPClient.STATUS_BODY:
            if _validate_sse_response():
                _read_sse_chunks()
        HTTPClient.STATUS_DISCONNECTED, HTTPClient.STATUS_CANT_RESOLVE, HTTPClient.STATUS_CANT_CONNECT, HTTPClient.STATUS_CONNECTION_ERROR:
            _handle_sse_disconnect("SSE connection closed with status: " + str(status))
        _:
            pass

func _open_sse_stream_connection() -> void:
    if not sse_should_run:
        return

    var endpoint: Dictionary = _parse_http_endpoint(streamable_http_url)
    if endpoint.is_empty():
        _handle_sse_disconnect("Invalid Streamable HTTP URL for SSE: " + streamable_http_url)
        return

    var host: String = str(endpoint.get("host", ""))
    var port: int = int(endpoint.get("port", 0))
    var use_tls: bool = bool(endpoint.get("use_tls", false))
    sse_request_path = str(endpoint.get("path", "/mcp"))

    if sse_http_connection:
        sse_http_connection.close()

    sse_http_connection = HTTPClient.new()
    sse_request_sent = false
    sse_response_validated = false
    _reset_sse_parser_state()

    var connect_err: int
    if use_tls:
        connect_err = sse_http_connection.connect_to_host(host, port, TLSOptions.client())
    else:
        connect_err = sse_http_connection.connect_to_host(host, port)

    if connect_err != OK:
        _handle_sse_disconnect("Failed to open SSE connection: " + str(connect_err))
        return

    sse_reconnect_at_msec = 0

func _send_sse_get_request() -> void:
    if not sse_http_connection:
        return

    var headers := PackedStringArray([
        "Accept: text/event-stream",
        "Cache-Control: no-cache",
        "MCP-Protocol-Version: " + negotiated_protocol_version
    ])
    if session_id != "":
        headers.append("MCP-Session-Id: " + session_id)

    var request_err = sse_http_connection.request(HTTPClient.METHOD_GET, sse_request_path, headers)
    if request_err != OK:
        _handle_sse_disconnect("Failed to send SSE GET request: " + str(request_err))
        return

    sse_request_sent = true

func _validate_sse_response() -> bool:
    if sse_response_validated:
        return true
    if not sse_http_connection:
        return false

    sse_response_validated = true
    var had_session = session_id != ""

    var response_code = sse_http_connection.get_response_code()
    var headers = sse_http_connection.get_response_headers()
    var latest_session_id = _extract_session_id(headers)
    if latest_session_id != "":
        session_id = latest_session_id

    if response_code != 200:
        if response_code == 404 and had_session:
            print("MCP Client: SSE session not found (404), reinitializing Streamable HTTP session")
            session_id = ""
            connect_streamable_http(streamable_http_url)
            return false
        _handle_sse_disconnect("SSE request failed with status: " + str(response_code))
        return false

    var content_type = _extract_header_value(headers, "content-type")
    if content_type.find("text/event-stream") == -1:
        _handle_sse_disconnect("SSE request returned unexpected content type: " + content_type)
        return false

    return true

func _read_sse_chunks() -> void:
    if not sse_http_connection:
        return

    while true:
        var chunk = sse_http_connection.read_response_body_chunk()
        if chunk.is_empty():
            break
        _process_sse_chunk(chunk.get_string_from_utf8())

func _process_sse_chunk(chunk_text: String) -> void:
    if chunk_text == "":
        return

    sse_line_buffer += chunk_text
    var lines: PackedStringArray = sse_line_buffer.split("\n")
    if not sse_line_buffer.ends_with("\n"):
        if lines.is_empty():
            return
        sse_line_buffer = lines[lines.size() - 1]
        lines.resize(lines.size() - 1)
    else:
        sse_line_buffer = ""

    for raw_line in lines:
        var line: String = str(raw_line)
        if line.ends_with("\r"):
            line = line.substr(0, line.length() - 1)
        _consume_sse_line(line)

func _consume_sse_line(line: String) -> void:
    if line.begins_with(":"):
        return

    if line == "":
        _dispatch_sse_event(sse_event_name, sse_data_lines)
        sse_event_name = "message"
        sse_data_lines.clear()
        return

    if line.begins_with("event:"):
        sse_event_name = line.substr(6).strip_edges()
        return

    if line.begins_with("data:"):
        sse_data_lines.append(line.substr(5).strip_edges())

func _reset_sse_parser_state() -> void:
    sse_line_buffer = ""
    sse_event_name = "message"
    sse_data_lines.clear()

func _handle_sse_disconnect(reason: String) -> void:
    if sse_http_connection:
        sse_http_connection.close()
    sse_http_connection = null
    sse_request_sent = false
    sse_response_validated = false
    _reset_sse_parser_state()

    if not sse_should_run or not is_connected:
        return

    print("MCP Client: ", reason)
    emit_signal("error", reason)
    sse_reconnect_at_msec = Time.get_ticks_msec() + SSE_RECONNECT_DELAY_MS
    set_process(true)

func _process_sse_payload(payload_text: String) -> void:
    var event_name = "message"
    var data_lines: Array[String] = []
    for line in payload_text.split("\n"):
        if line.begins_with(":"):
            continue
        if line.strip_edges() == "":
            _dispatch_sse_event(event_name, data_lines)
            event_name = "message"
            data_lines = []
            continue
        if line.begins_with("event:"):
            event_name = line.substr(6).strip_edges()
            continue
        if line.begins_with("data:"):
            data_lines.append(line.substr(5).strip_edges())

    _dispatch_sse_event(event_name, data_lines)

func _dispatch_sse_event(event_name: String, data_lines: Array[String]) -> void:
    if data_lines.is_empty():
        return
    if event_name == "heartbeat":
        return

    var payload_text = ""
    for i in range(data_lines.size()):
        if i > 0:
            payload_text += "\n"
        payload_text += str(data_lines[i])

    var payload = _parse_json_payload(payload_text)
    if payload == null:
        return
    _emit_jsonrpc_payload(payload)

func _parse_http_endpoint(url: String) -> Dictionary:
    var trimmed_url = url.strip_edges()
    if trimmed_url == "":
        return {}

    var use_tls = false
    var remainder = ""
    if trimmed_url.begins_with("https://"):
        use_tls = true
        remainder = trimmed_url.substr(8)
    elif trimmed_url.begins_with("http://"):
        remainder = trimmed_url.substr(7)
    else:
        return {}

    var host_port = remainder
    var path = "/"

    var path_index = remainder.find("/")
    if path_index != -1:
        host_port = remainder.substr(0, path_index)
        path = remainder.substr(path_index)
    else:
        var query_index = remainder.find("?")
        if query_index != -1:
            host_port = remainder.substr(0, query_index)
            path = "/" + remainder.substr(query_index)

    if host_port == "":
        return {}

    var host = host_port
    var port = 443 if use_tls else 80

    if host_port.begins_with("["):
        var closing_bracket = host_port.find("]")
        if closing_bracket == -1:
            return {}
        host = host_port.substr(1, closing_bracket - 1)
        var ipv6_port_suffix = host_port.substr(closing_bracket + 1)
        if ipv6_port_suffix != "":
            if not ipv6_port_suffix.begins_with(":"):
                return {}
            var ipv6_port_text = ipv6_port_suffix.substr(1)
            if not ipv6_port_text.is_valid_int():
                return {}
            port = int(ipv6_port_text)
    else:
        var first_colon = host_port.find(":")
        var last_colon = host_port.rfind(":")
        if first_colon != -1 and first_colon == last_colon:
            host = host_port.substr(0, last_colon)
            var port_text = host_port.substr(last_colon + 1)
            if not port_text.is_valid_int():
                return {}
            port = int(port_text)

    if host == "" or port <= 0 or port > 65535:
        return {}

    return {
        "host": host,
        "port": port,
        "use_tls": use_tls,
        "path": path
    }

func _extract_session_id(headers: PackedStringArray) -> String:
    for header in headers:
        if header.to_lower().begins_with("mcp-session-id:"):
            var parts = header.split(":", false, 1)
            if parts.size() == 2:
                return parts[1].strip_edges()
    return ""

func _extract_header_value(headers: PackedStringArray, header_name_lower: String) -> String:
    for header in headers:
        var lower = header.to_lower()
        if lower.begins_with(header_name_lower + ":"):
            var parts = header.split(":", false, 1)
            if parts.size() == 2:
                return parts[1].strip_edges().to_lower()
    return ""

func _fail_connect(message: String) -> void:
    is_connecting = false
    _mark_disconnected()
    request_in_flight = false
    _drop_pending_messages("MCP initialization failed")
    print("MCP Client: ", message)
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
    _stop_sse_stream()
    session_id = ""
    if is_connected:
        is_connected = false
        emit_signal("disconnected")

func _drop_pending_messages(reason: String) -> void:
    var dropped_count = pending_messages.size()
    if dropped_count == 0:
        return
    pending_messages.clear()
    emit_signal("error", "Dropped %d queued MCP message(s): %s" % [dropped_count, reason])
