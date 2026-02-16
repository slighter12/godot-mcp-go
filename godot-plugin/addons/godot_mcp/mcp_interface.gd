extends Node

signal tool_called(tool_name: String, arguments: Dictionary)
signal tool_result(tool_name: String, result: Dictionary)
signal tool_error(tool_name: String, error: String)

var mcp_server: Node
var tools: Dictionary = {}
var client_id: String = ""
var request_counter: int = 0

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
        "method": "initialized",
        "params": {
            "client_id": client_id
        }
    }
    mcp_server.send_message(initialized_notification)

func _on_disconnected():
    # Clear tool list on disconnect.
    tools.clear()

func _on_error(error: String):
    print("MCP interface error: ", error)

func call_tool(tool_name: String, arguments: Dictionary = {}):
    if not tools.has(tool_name):
        emit_signal("tool_error", tool_name, "Tool not found: " + tool_name)
        return
    
    var tool_call_request = {
        "jsonrpc": "2.0",
        "id": _next_request_id(),
        "method": "tools/call",
        "params": {
            "name": tool_name,
            "arguments": arguments
        }
    }
    
    mcp_server.send_message(tool_call_request)
    emit_signal("tool_called", tool_name, arguments)

func handle_message(message: Dictionary):
    if message.has("error"):
        var err_obj = message.get("error", {})
        if err_obj is Dictionary:
            handle_error({
                "message": err_obj.get("message", "Unknown error")
            })
            return

    if message.has("result"):
        var result = message.get("result", {})
        if result is Dictionary:
            if result.get("type", "") == "init":
                handle_init(result)
                return
            if result.has("tool") or result.has("isError"):
                handle_tool_result(result)
                return

    # Legacy message handling fallback.
    var message_type = message.get("type", "")
    var payload = message.get("payload", {})
    
    match message_type:
        "init":
            handle_init(payload)
        "tool_result":
            handle_tool_result(payload)
        "error":
            handle_error(payload)
        _:
            print("Unknown message type: ", message_type)

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
