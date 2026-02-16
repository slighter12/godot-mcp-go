@tool
extends Node

signal connected
signal disconnected
signal error(error: String)
signal message_received(message: Dictionary)

const DEFAULT_PROTOCOL_VERSION := "2025-11-25"

var streamable_http_url: String = "http://localhost:9080/mcp"
var is_connecting: bool = false
var post_http_connection: HTTPRequest
var session_id: String = ""
var negotiated_protocol_version: String = DEFAULT_PROTOCOL_VERSION

var is_connected: bool = false
var request_in_flight: bool = false
var pending_messages: Array = []
var pending_connect_url: String = ""

func _ready():
    print("MCP Server: Initializing...")
    load_settings()

    post_http_connection = HTTPRequest.new()
    add_child(post_http_connection)
    post_http_connection.request_completed.connect(_on_post_request_completed)

func load_settings():
    print("MCP Server: Loading settings...")
    var config = ConfigFile.new()
    var err = config.load("res://addons/godot_mcp/config.cfg")
    if err == OK:
        streamable_http_url = config.get_value("mcp", "streamable_http_url", "http://localhost:9080/mcp")
        var configured_type = config.get_value("mcp", "connection_type", "streamable_http")
        if configured_type != "streamable_http":
            print("MCP Server: connection_type '%s' is unsupported in the plugin. Using streamable_http." % configured_type)
        print("MCP Server: Settings loaded - type: streamable_http, url: ", streamable_http_url)
    else:
        print("MCP Server: Failed to load settings, using defaults")

func connect_to_server():
    print("MCP Server: Attempting to connect...")
    connect_streamable_http(streamable_http_url)

func connect_streamable_http(url: String):
    if is_connecting:
        pending_connect_url = url
        print("MCP Server: Connection attempt in progress, queued reconnect to ", url)
        return
    if request_in_flight:
        pending_connect_url = url
        print("MCP Server: Request in flight, queued reconnect to ", url)
        return

    if is_connected:
        _mark_disconnected()

    streamable_http_url = url
    is_connecting = true
    session_id = ""
    negotiated_protocol_version = DEFAULT_PROTOCOL_VERSION
    _drop_pending_messages("reconnecting to a new MCP session")
    print("MCP Server: Connecting to Streamable HTTP (POST-only) at ", url)

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

func _on_post_request_completed(result: int, response_code: int, headers: PackedStringArray, body: PackedByteArray):
    request_in_flight = false

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

func _handle_initialize_response(response_code: int, headers: PackedStringArray, body: PackedByteArray):
    print("MCP Server: Initialize response received - code: ", response_code)
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
        print("MCP Server: Initialize response has no MCP-Session-Id (stateless mode)")
    else:
        print("MCP Server: Session ID received: ", session_id)

    print("MCP Server: Initialization successful")
    is_connecting = false
    is_connected = true
    emit_signal("connected")

func _handle_post_response(response_code: int, headers: PackedStringArray, body: PackedByteArray):
    print("MCP Server: Streamable HTTP response received - code: ", response_code)

    var latest_session_id = _extract_session_id(headers)
    if latest_session_id != "":
        session_id = latest_session_id

    if response_code != 200 and response_code != 202:
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
    print("MCP Server: Sending message: ", message)
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

func _flush_pending_messages():
    if is_connecting or request_in_flight or pending_connect_url != "":
        return
    if pending_messages.is_empty():
        return

    var next_message = pending_messages.pop_front()
    if next_message is Dictionary:
        if not _send_raw_message(next_message):
            _drop_pending_messages("failed to send queued MCP message")

func _flush_reconnect():
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

func _emit_jsonrpc_payload(payload: Variant):
    if payload is Dictionary:
        _emit_jsonrpc_object(payload)
        return

    if payload is Array:
        for item in payload:
            if item is Dictionary:
                _emit_jsonrpc_object(item)

func _emit_jsonrpc_object(message: Dictionary):
    if message.get("jsonrpc", "") != "2.0":
        return
    emit_signal("message_received", message)

func _process_sse_payload(payload_text: String):
    var event_name = "message"
    var data_lines: Array = []
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

func _dispatch_sse_event(event_name: String, data_lines: Array):
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

func _fail_connect(message: String):
    is_connecting = false
    _mark_disconnected()
    request_in_flight = false
    _drop_pending_messages("MCP initialization failed")
    print("MCP Server: ", message)
    emit_signal("error", message)

func disconnect_from_server():
    is_connecting = false
    request_in_flight = false
    _drop_pending_messages("plugin disconnect requested")
    pending_connect_url = ""
    _mark_disconnected()

func _mark_disconnected():
    session_id = ""
    if is_connected:
        is_connected = false
        emit_signal("disconnected")

func _drop_pending_messages(reason: String):
    var dropped_count = pending_messages.size()
    if dropped_count == 0:
        return
    pending_messages.clear()
    emit_signal("error", "Dropped %d queued MCP message(s): %s" % [dropped_count, reason])
