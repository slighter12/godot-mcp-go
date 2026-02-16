extends Node

signal tool_called(tool_name: String, arguments: Dictionary)
signal tool_result(tool_name: String, result: Dictionary)
signal tool_error(tool_name: String, error: String)

var mcp_server: Node
var tools: Dictionary = {}
var client_id: String = ""
var request_counter: int = 0
var pending_tool_calls: Dictionary = {}

func _ready():
    if mcp_server == null:
        mcp_server = get_parent().get_node_or_null("mcp_server")
    if mcp_server == null:
        push_error("MCP Interface: mcp_server node not found")
        return

    mcp_server.connected.connect(_on_connected)
    mcp_server.disconnected.connect(_on_disconnected)
    mcp_server.error.connect(_on_error)
    
    # Generate a unique client ID.
    client_id = str(randi())

func set_mcp_server(server: Node):
    mcp_server = server

func _on_connected():
    # Send initialized notification after initialize response is received.
    var initialized_notification = {
        "jsonrpc": "2.0",
        "method": "initialized"
    }
    mcp_server.send_message(initialized_notification)

func _on_disconnected():
    # Clear tool list on disconnect.
    tools.clear()
    pending_tool_calls.clear()

func _on_error(error: String):
    print("MCP interface error: ", error)

func call_tool(tool_name: String, arguments: Dictionary = {}):
    if not tools.has(tool_name):
        emit_signal("tool_error", tool_name, "Tool not found: " + tool_name)
        return
    
    var request_id = _next_request_id()
    var tool_call_request = {
        "jsonrpc": "2.0",
        "id": request_id,
        "method": "tools/call",
        "params": {
            "name": tool_name,
            "arguments": arguments
        }
    }
    
    pending_tool_calls[request_id] = tool_name
    mcp_server.send_message(tool_call_request)
    emit_signal("tool_called", tool_name, arguments)

func handle_message(message: Dictionary):
    if message.get("jsonrpc", "") != "2.0":
        print("Ignoring non JSON-RPC 2.0 payload: ", message)
        return

    if message.has("method"):
        # Server-initiated method calls are out of scope for this plugin sample.
        print("Ignoring server-initiated JSON-RPC method: ", message.get("method", ""))
        return

    if not message.has("id"):
        print("Ignoring JSON-RPC response without id: ", message)
        return

    var response_id = message.get("id", null)
    var request_id = "" if response_id == null else str(response_id)
    var pending_tool_name = ""
    if request_id != "" and pending_tool_calls.has(request_id):
        pending_tool_name = pending_tool_calls[request_id]
        pending_tool_calls.erase(request_id)

    if message.has("error"):
        var err_obj = message.get("error", {})
        if err_obj is Dictionary:
            var err_msg = _extract_jsonrpc_error_message(err_obj)
            if pending_tool_name != "":
                emit_signal("tool_error", pending_tool_name, err_msg)
            handle_error({"message": err_msg})
            return

    if message.has("result"):
        var result = message.get("result", {})
        if result is Dictionary:
            if result.get("type", "") == "init":
                handle_init(result)
                return
            if pending_tool_name != "" and not result.has("tool"):
                result["tool"] = pending_tool_name
            if result.has("tool") or result.has("isError") or pending_tool_name != "":
                if result.get("isError", false):
                    var tool_name = result.get("tool", pending_tool_name)
                    var err_msg = _extract_tool_error_message(result)
                    emit_signal("tool_error", tool_name, err_msg)
                    handle_error({"message": err_msg})
                else:
                    handle_tool_result(result)
                return

    print("Unknown JSON-RPC message shape: ", message)

func handle_init(payload: Dictionary):
    # Refresh tool list.
    tools.clear()
    for tool in payload.get("tools", []):
        tools[tool.name] = tool

func handle_tool_result(payload: Dictionary):
    var tool_name = payload.get("tool", "")
    var result = payload.get("result", {})
    emit_signal("tool_result", tool_name, result)

func handle_error(payload: Dictionary):
    var message = payload.get("message", "Unknown error")
    print("MCP error: ", message)

func _next_request_id() -> String:
    request_counter += 1
    return "godot-%s-%d" % [client_id, request_counter]

func _extract_tool_error_message(result: Dictionary) -> String:
    var content = result.get("content", [])
    if content is Array and content.size() > 0 and content[0] is Dictionary:
        var first = content[0]
        if first.get("type", "") == "text" and first.has("text"):
            return str(first["text"])
    return str(result.get("error", "Tool execution failed"))

func _extract_jsonrpc_error_message(error_obj: Dictionary) -> String:
    return str(error_obj.get("message", "Unknown JSON-RPC error"))
