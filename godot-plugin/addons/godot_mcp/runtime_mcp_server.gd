@tool
extends "res://addons/godot_mcp/mcp_server.gd"
# Runtime companion uses the same modern protocol client and its own config
# section. The transport and wire contract are intentionally identical.

func load_settings() -> void:
	var config := ConfigFile.new()
	var err := config.load("res://addons/godot_mcp/config.cfg")
	if err == OK:
		streamable_http_url = str(config.get_value("mcp_runtime", "streamable_http_url", streamable_http_url)).strip_edges()
