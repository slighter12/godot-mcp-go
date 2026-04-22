class_name RuntimeMCPProtocolAdapter
extends "res://addons/godot_mcp/runtime_mcp_interface.gd"

func set_client(client: Node) -> void:
	set_mcp_client(client)
