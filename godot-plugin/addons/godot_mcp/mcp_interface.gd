extends Node

signal tool_called(tool_name: String, arguments: Dictionary)
signal tool_result(tool_name: String, result: Dictionary)
signal tool_error(tool_name: String, error: String)

var mcp_client: Node
var tools: Dictionary = {}
var client_id: String = ""
var request_counter: int = 0

var pending_requests: Dictionary = {}
var tools_request_in_progress: bool = false
var tools_refresh_buffer: Dictionary = {}

func _ready():
    if mcp_client == null:
        mcp_client = get_parent().get_node_or_null("mcp_client")
    if mcp_client == null:
        mcp_client = get_parent().get_node_or_null("mcp_server")
    if mcp_client == null:
        push_error("MCP Interface: MCP client node not found")
        return

    mcp_client.connected.connect(_on_connected)
    mcp_client.disconnected.connect(_on_disconnected)
    mcp_client.error.connect(_on_error)

    var rng = RandomNumberGenerator.new()
    rng.randomize()
    client_id = "%s_%s" % [str(Time.get_unix_time_from_system()), str(rng.randi())]

func set_mcp_client(client: Node):
    mcp_client = client

func set_mcp_server(server: Node):
    # Backward compatibility for older plugin wiring.
    set_mcp_client(server)

func _on_connected():
    tools.clear()
    tools_refresh_buffer.clear()
    pending_requests.clear()
    tools_request_in_progress = false

    _send_initialized_notification()
    _request_tools_list("")

func _on_disconnected():
    tools.clear()
    tools_refresh_buffer.clear()
    pending_requests.clear()
    tools_request_in_progress = false

func _on_error(error_message: String):
    print("MCP interface error: ", error_message)

func call_tool(tool_name: String, arguments: Dictionary = {}):
    if not tools.has(tool_name):
        emit_signal("tool_error", tool_name, "Tool not found: " + tool_name)
        return

    var request_id = _new_request_id()
    pending_requests[request_id] = {
        "kind": "tool_call",
        "tool_name": tool_name
    }

    var tool_call_request = {
        "jsonrpc": "2.0",
        "id": request_id,
        "method": "tools/call",
        "params": {
            "name": tool_name,
            "arguments": arguments
        }
    }

    if not mcp_client.send_message(tool_call_request):
        pending_requests.erase(request_id)
        emit_signal("tool_error", tool_name, "Failed to send tools/call request")
        return
    emit_signal("tool_called", tool_name, arguments)

func handle_message(message: Dictionary):
    if message.get("jsonrpc", "") != "2.0":
        return

    if message.has("method"):
        _handle_server_notification(message)
        return

    if not message.has("id"):
        return

    var request_id = str(message.get("id", ""))
    if request_id == "":
        return

    var pending = pending_requests.get(request_id, {})
    pending_requests.erase(request_id)

    if message.has("error"):
        _handle_error_response(pending, message.get("error", {}))
        return

    if not message.has("result"):
        return

    _handle_result_response(pending, message.get("result"))

func _handle_server_notification(message: Dictionary):
    var method = str(message.get("method", ""))
    if method == "notifications/tools/list_changed":
        if not tools_request_in_progress:
            _request_tools_list("")

func _handle_error_response(pending: Dictionary, error_obj: Variant):
    var error_message = _extract_jsonrpc_error_message(error_obj)
    var kind = str(pending.get("kind", ""))

    if kind == "tool_call":
        var tool_name = str(pending.get("tool_name", ""))
        if tool_name != "":
            emit_signal("tool_error", tool_name, error_message)
    elif kind == "tools_list":
        tools_request_in_progress = false
        tools_refresh_buffer.clear()

    handle_error({"message": error_message})

func _handle_result_response(pending: Dictionary, result: Variant):
    var kind = str(pending.get("kind", ""))
    if kind == "tools_list":
        _handle_tools_list_result(result)
        return

    if kind == "tool_call":
        _handle_tool_call_result(result, pending)
        return

func _handle_tools_list_result(result: Variant):
    if not (result is Dictionary):
        tools_request_in_progress = false
        tools_refresh_buffer.clear()
        handle_error({"message": "Invalid tools/list result payload"})
        return

    var result_dict: Dictionary = result
    var listed_tools = result_dict.get("tools", [])
    if listed_tools is Array:
        for tool in listed_tools:
            if tool is Dictionary and tool.has("name"):
                tools_refresh_buffer[str(tool["name"])] = tool

    var next_cursor_value = result_dict.get("nextCursor", "")
    var next_cursor = str(next_cursor_value).strip_edges()
    if next_cursor != "":
        _request_tools_list(next_cursor)
        return

    tools = tools_refresh_buffer.duplicate(true)
    tools_refresh_buffer.clear()
    tools_request_in_progress = false

func _handle_tool_call_result(result: Variant, pending: Dictionary):
    var tool_name = str(pending.get("tool_name", ""))
    if not (result is Dictionary):
        emit_signal("tool_error", tool_name, "Invalid tool result payload")
        return

    var result_dict: Dictionary = result
    var response_tool_name = str(result_dict.get("tool", "")).strip_edges()
    if response_tool_name != "":
        tool_name = response_tool_name

    var is_error = bool(result_dict.get("isError", false))
    if is_error:
        var err_msg = _extract_tool_error_message(result_dict)
        emit_signal("tool_error", tool_name, err_msg)
        return

    var payload: Variant = result_dict.get("result", null)
    if payload == null:
        payload = result_dict.get("structuredContent", {})

    if payload is Dictionary:
        emit_signal("tool_result", tool_name, payload)
        return

    emit_signal("tool_result", tool_name, {"value": payload})

func handle_error(payload: Dictionary):
    var message = payload.get("message", "Unknown error")
    print("MCP error: ", message)

func _send_initialized_notification():
    var initialized_notification = {
        "jsonrpc": "2.0",
        "method": "notifications/initialized",
        "params": {
            "clientId": client_id
        }
    }
    if not mcp_client.send_message(initialized_notification):
        handle_error({"message": "Failed to send initialized notification"})

func _request_tools_list(cursor: String):
    if cursor == "":
        tools_refresh_buffer.clear()
    tools_request_in_progress = true

    var request_id = _new_request_id()
    pending_requests[request_id] = {
        "kind": "tools_list"
    }

    var params = {}
    if cursor != "":
        params["cursor"] = cursor

    var request = {
        "jsonrpc": "2.0",
        "id": request_id,
        "method": "tools/list",
        "params": params
    }
    if not mcp_client.send_message(request):
        pending_requests.erase(request_id)
        tools_request_in_progress = false
        tools_refresh_buffer.clear()
        handle_error({"message": "Failed to send tools/list request"})

func _new_request_id() -> String:
    request_counter += 1
    return "godot-%s-%d" % [client_id, request_counter]

func _extract_tool_error_message(result: Dictionary) -> String:
    var content = result.get("content", [])
    if content is Array and content.size() > 0 and content[0] is Dictionary:
        var first = content[0]
        if first.get("type", "") == "text" and first.has("text"):
            return str(first["text"])
    return str(result.get("error", "Tool execution failed"))

func _extract_jsonrpc_error_message(error_obj: Variant) -> String:
    if error_obj is Dictionary:
        return str(error_obj.get("message", "Unknown JSON-RPC error"))
    return "Unknown JSON-RPC error"
