extends Node

signal tool_called(tool_name: String, arguments: Dictionary)
signal tool_result(tool_name: String, result: Dictionary)
signal tool_error(tool_name: String, error: String)

var mcp_server: Node
var tools: Dictionary = {}
var client_id: String = ""
var server_id: String = "default"

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
    # Send initialize message after the connection is established.
    var init_message = {
        "type": "init",
        "client_id": client_id,
        "server_id": server_id,
        "payload": {}
    }
    mcp_server.send_message(init_message)

func _on_disconnected():
    # Clear tool list on disconnect.
    tools.clear()

func _on_error(error: String):
    print("MCP interface error: ", error)

func call_tool(tool_name: String, arguments: Dictionary = {}):
    if not tools.has(tool_name):
        emit_signal("tool_error", tool_name, "Tool not found: " + tool_name)
        return
    
    var tool_call = {
        "type": "tool_call",
        "client_id": client_id,
        "server_id": server_id,
        "payload": {
            "tool": tool_name,
            "arguments": arguments
        }
    }
    
    mcp_server.send_message(tool_call)
    emit_signal("tool_called", tool_name, arguments)

func handle_message(message: Dictionary):
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
