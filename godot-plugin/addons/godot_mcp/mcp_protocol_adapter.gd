class_name MCPProtocolAdapter
extends Node

signal tool_called(tool_name: String, arguments: Dictionary)
signal tool_result(tool_name: String, result: Dictionary)
signal tool_error(tool_name: String, error: String)
signal tool_progress(tool_name: String, progress: float, total: float, message: String, progress_token: Variant)
signal runtime_sync_failed(error: String)
signal runtime_command_received(command_id: String, command_name: String, arguments: Dictionary)

var _legacy_interface: Node
var _pending_client: Node

func _ready() -> void:
	_legacy_interface = preload("res://addons/godot_mcp/mcp_interface.gd").new()
	_legacy_interface.name = "legacy_mcp_interface"
	add_child(_legacy_interface)
	_legacy_interface.tool_called.connect(_on_tool_called)
	_legacy_interface.tool_result.connect(_on_tool_result)
	_legacy_interface.tool_error.connect(_on_tool_error)
	_legacy_interface.tool_progress.connect(_on_tool_progress)
	_legacy_interface.runtime_sync_failed.connect(_on_runtime_sync_failed)
	_legacy_interface.runtime_command_received.connect(_on_runtime_command_received)
	if _pending_client != null:
		_legacy_interface.set_mcp_client(_pending_client)
		_pending_client = null

func set_client(client: Node) -> void:
	if client == null:
		return
	if _legacy_interface == null:
		_pending_client = client
		return
	_legacy_interface.set_mcp_client(client)

func handle_message(message: Dictionary) -> void:
	if _legacy_interface != null:
		_legacy_interface.handle_message(message)

func call_tool(tool_name: String, arguments: Dictionary = {}) -> void:
	if _legacy_interface != null:
		_legacy_interface.call_tool(tool_name, arguments)

func sync_runtime_snapshot(snapshot: Dictionary) -> bool:
	if _legacy_interface == null:
		return false
	return _legacy_interface.sync_runtime_snapshot(snapshot)

func can_ping_runtime_bridge() -> bool:
	if _legacy_interface == null:
		return false
	return _legacy_interface.can_ping_runtime_bridge()

func ping_runtime_bridge() -> void:
	if _legacy_interface != null:
		_legacy_interface.ping_runtime_bridge()

func ack_runtime_command(command_id: String, success: bool, result: Dictionary = {}, error_message: String = "", reason: String = "", retryable: Variant = null, schema_version: String = "v1") -> void:
	if _legacy_interface != null:
		_legacy_interface.ack_runtime_command(command_id, success, result, error_message, reason, retryable, schema_version)

func has_tool(tool_name: String) -> bool:
	if _legacy_interface == null:
		return false
	return _legacy_interface.tools.has(tool_name)

func get_tools() -> Dictionary:
	if _legacy_interface == null:
		return {}
	return _legacy_interface.tools.duplicate(true)

func _on_tool_called(tool_name: String, arguments: Dictionary) -> void:
	emit_signal("tool_called", tool_name, arguments)

func _on_tool_result(tool_name: String, result: Dictionary) -> void:
	emit_signal("tool_result", tool_name, result)

func _on_tool_error(tool_name: String, error_message: String) -> void:
	emit_signal("tool_error", tool_name, error_message)

func _on_tool_progress(tool_name: String, progress: float, total: float, message: String, progress_token: Variant) -> void:
	emit_signal("tool_progress", tool_name, progress, total, message, progress_token)

func _on_runtime_sync_failed(error_message: String) -> void:
	emit_signal("runtime_sync_failed", error_message)

func _on_runtime_command_received(command_id: String, command_name: String, arguments: Dictionary) -> void:
	emit_signal("runtime_command_received", command_id, command_name, arguments)
