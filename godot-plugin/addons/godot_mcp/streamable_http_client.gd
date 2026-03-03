@tool
class_name StreamableHTTPClient
extends Node

signal connected
signal disconnected
signal error(error: String)
signal message_received(message: Dictionary)

var _legacy_client: Node

func _ready() -> void:
	_legacy_client = preload("res://addons/godot_mcp/mcp_server.gd").new()
	_legacy_client.name = "legacy_mcp_server"
	add_child(_legacy_client)
	_legacy_client.connected.connect(_on_connected)
	_legacy_client.disconnected.connect(_on_disconnected)
	_legacy_client.error.connect(_on_error)
	_legacy_client.message_received.connect(_on_message_received)

func connect_streamable_http(url: String) -> void:
	if _legacy_client != null:
		_legacy_client.connect_streamable_http(url)

func disconnect_from_server() -> void:
	if _legacy_client != null:
		_legacy_client.disconnect_from_server()

func send_message(message: Dictionary) -> bool:
	if _legacy_client == null:
		return false
	return _legacy_client.send_message(message)

func _on_connected() -> void:
	emit_signal("connected")

func _on_disconnected() -> void:
	emit_signal("disconnected")

func _on_error(message: String) -> void:
	emit_signal("error", message)

func _on_message_received(message: Dictionary) -> void:
	emit_signal("message_received", message)
