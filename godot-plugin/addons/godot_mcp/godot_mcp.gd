@tool
extends EditorPlugin

var mcp_server: Node
var mcp_interface: Node
var settings_dialog: AcceptDialog

func _enter_tree():
    print("Godot MCP Plugin: Entering tree...")
    
    # 創建 MCP 服務器節點
    mcp_server = preload("res://addons/godot_mcp/mcp_server.gd").new()
    add_child(mcp_server)
    
    # 創建 MCP 接口節點
    mcp_interface = preload("res://addons/godot_mcp/mcp_interface.gd").new()
    add_child(mcp_interface)
    
    # 連接信號
    mcp_server.connected.connect(_on_mcp_connected)
    mcp_server.disconnected.connect(_on_mcp_disconnected)
    mcp_server.error.connect(_on_mcp_error)
    mcp_server.message_received.connect(_on_mcp_message_received)
    
    # 創建設置對話框
    settings_dialog = preload("res://addons/godot_mcp/mcp_settings_dialog.tscn").instantiate()
    add_child(settings_dialog)
    
    # 添加工具欄按鈕
    add_tool_menu_item("MCP Settings", _on_settings_pressed)
    
    # 加載設置並連接
    var config = ConfigFile.new()
    var err = config.load("res://addons/godot_mcp/config.cfg")
    if err == OK:
        var type = config.get_value("mcp", "connection_type", "streamable_http")
        var streamable_http_url = config.get_value("mcp", "streamable_http_url", "http://localhost:9080/mcp")
        
        print("Godot MCP Plugin: Connecting with type: ", type)
        if type == "streamable_http":
            mcp_server.connect_streamable_http(streamable_http_url)
        else:
            mcp_server.connect_to_server()
    else:
        print("Godot MCP Plugin: Failed to load config, using default connection")
        mcp_server.connect_to_server()
    print("Godot MCP Plugin: Initialized successfully")

func _exit_tree():
    print("Godot MCP Plugin: Exiting tree...")
    
    if mcp_server:
        mcp_server.queue_free()
    
    if mcp_interface:
        mcp_interface.queue_free()
    
    if settings_dialog:
        settings_dialog.queue_free()
    print("Godot MCP Plugin: Cleanup complete")

func _on_mcp_connected():
    print("Godot MCP Plugin: Connected to MCP server")

func _on_mcp_disconnected():
    print("Godot MCP Plugin: Disconnected from MCP server")

func _on_mcp_error(error: String):
    print("Godot MCP Plugin: Error: ", error)

func _on_mcp_message_received(message: Dictionary):
    print("Godot MCP Plugin: Message received: ", message)
    mcp_interface.handle_message(message)

func _on_settings_pressed():
    print("MCP Plugin: Opening settings dialog")
    settings_dialog.popup_centered() 