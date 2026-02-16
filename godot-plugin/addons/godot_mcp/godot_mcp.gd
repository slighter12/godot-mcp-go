@tool
extends EditorPlugin

var mcp_client: Node
var mcp_interface: Node
var settings_dialog: AcceptDialog
var current_streamable_http_url: String = "http://localhost:9080/mcp"

func _enter_tree():
    print("Godot MCP Plugin: Entering tree...")
    
    # Create MCP client node (script path keeps legacy name for compatibility).
    mcp_client = preload("res://addons/godot_mcp/mcp_server.gd").new()
    mcp_client.name = "mcp_client"
    add_child(mcp_client)
    
    # Create MCP interface node.
    mcp_interface = preload("res://addons/godot_mcp/mcp_interface.gd").new()
    mcp_interface.name = "mcp_interface"
    add_child(mcp_interface)
    mcp_interface.set_mcp_client(mcp_client)
    
    # Connect signals.
    mcp_client.connected.connect(_on_mcp_connected)
    mcp_client.disconnected.connect(_on_mcp_disconnected)
    mcp_client.error.connect(_on_mcp_error)
    mcp_client.message_received.connect(_on_mcp_message_received)
    
    # Create settings dialog.
    settings_dialog = preload("res://addons/godot_mcp/mcp_settings_dialog.tscn").instantiate()
    add_child(settings_dialog)
    settings_dialog.connect("settings_saved", Callable(self, "_on_settings_saved"))
    
    # Add toolbar menu item.
    add_tool_menu_item("MCP Settings", _on_settings_pressed)
    
    # Load settings and connect via streamable HTTP.
    var config = ConfigFile.new()
    var err = config.load("res://addons/godot_mcp/config.cfg")
    if err == OK:
        current_streamable_http_url = config.get_value("mcp", "streamable_http_url", current_streamable_http_url)
    else:
        print("Godot MCP Plugin: Failed to load config, using default connection")
    print("Godot MCP Plugin: Connecting with streamable_http")
    mcp_client.connect_streamable_http(current_streamable_http_url)
    print("Godot MCP Plugin: Initialized successfully")

func _exit_tree():
    print("Godot MCP Plugin: Exiting tree...")
    remove_tool_menu_item("MCP Settings")
    
    if mcp_client:
        mcp_client.disconnect_from_server()
        mcp_client.queue_free()
    
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

func _on_settings_saved(streamable_http_url: String):
    current_streamable_http_url = streamable_http_url
    print("Godot MCP Plugin: Reconnecting with updated streamable_http URL")
    mcp_client.connect_streamable_http(current_streamable_http_url)
