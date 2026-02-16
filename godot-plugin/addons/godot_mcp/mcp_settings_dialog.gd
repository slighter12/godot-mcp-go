@tool
extends AcceptDialog

signal settings_saved(streamable_http_url: String)

@onready var connection_type = $VBoxContainer/ConnectionType
@onready var streamable_http_settings = $VBoxContainer/StreamableHTTPSettings
@onready var streamable_http_url = $VBoxContainer/StreamableHTTPSettings/StreamableHTTPURL

func _ready():
    confirmed.connect(_on_ok_pressed)

    # Configure transport selector (streamable_http only).
    connection_type.clear()
    connection_type.add_item("streamable_http")
    connection_type.selected = 0
    connection_type.disabled = true

    # Connect signals.
    connection_type.item_selected.connect(_on_connection_type_changed)
    
    # Load settings.
    load_settings()

func load_settings():
    var config = ConfigFile.new()
    var err = config.load("res://addons/godot_mcp/config.cfg")
    if err == OK:
        streamable_http_url.text = config.get_value("mcp", "streamable_http_url", "http://localhost:9080/mcp")
    _on_connection_type_changed(connection_type.selected)

func _on_connection_type_changed(index: int):
    streamable_http_settings.visible = index == 0

func _on_ok_pressed():
    var config = ConfigFile.new()
    var target_url = streamable_http_url.text.strip_edges()
    if target_url == "":
        target_url = "http://localhost:9080/mcp"
        streamable_http_url.text = target_url
    
    # Persist transport and endpoint settings.
    config.set_value("mcp", "connection_type", "streamable_http")
    config.set_value("mcp", "streamable_http_url", target_url)
    
    # Save configuration.
    var err = config.save("res://addons/godot_mcp/config.cfg")
    if err == OK:
        print("MCP Settings: Configuration saved successfully")
        emit_signal("settings_saved", target_url)
    else:
        print("MCP Settings: Failed to save configuration") 
