@tool
extends AcceptDialog

@onready var connection_type = $VBoxContainer/ConnectionType
@onready var stdio_settings = $VBoxContainer/StdioSettings
@onready var streamable_http_settings = $VBoxContainer/StreamableHTTPSettings
@onready var server_command = $VBoxContainer/StdioSettings/ServerCommand
@onready var streamable_http_url = $VBoxContainer/StreamableHTTPSettings/StreamableHTTPURL

func _ready():
    # 連接信號
    connection_type.item_selected.connect(_on_connection_type_changed)
    
    # 加載設置
    load_settings()

func load_settings():
    var config = ConfigFile.new()
    var err = config.load("res://addons/godot_mcp/config.cfg")
    if err == OK:
        var type = config.get_value("mcp", "connection_type", "streamable_http")
        connection_type.selected = 0 if type == "stdio" else 1
        
        server_command.text = config.get_value("mcp", "server_command", "./godot-mcp-go")
        streamable_http_url.text = config.get_value("mcp", "streamable_http_url", "http://localhost:9080/mcp")
        
        _on_connection_type_changed(connection_type.selected)

func _on_connection_type_changed(index: int):
    var conn_type = "stdio"
    if index == 1:
        conn_type = "streamable_http"
    
    stdio_settings.visible = index == 0
    streamable_http_settings.visible = index == 1

func _on_ok_pressed():
    var config = ConfigFile.new()
    
    # 保存連接類型
    var conn_type = "stdio"
    if connection_type.selected == 1:
        conn_type = "streamable_http"
    
    config.set_value("mcp", "connection_type", conn_type)
    config.set_value("mcp", "server_command", server_command.text)
    config.set_value("mcp", "streamable_http_url", streamable_http_url.text)
    
    # 保存配置
    var err = config.save("res://addons/godot_mcp/config.cfg")
    if err == OK:
        print("MCP Settings: Configuration saved successfully")
    else:
        print("MCP Settings: Failed to save configuration") 