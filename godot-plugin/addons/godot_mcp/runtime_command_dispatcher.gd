class_name RuntimeCommandDispatcher
extends RefCounted

func dispatch(command_id: String, command_name: String, arguments: Dictionary, editor_interface: EditorInterface, protocol_adapter: MCPProtocolAdapter, mutating_handlers: Dictionary, sync_callback: Callable) -> bool:
	if protocol_adapter == null:
		return false
	if editor_interface == null:
		protocol_adapter.ack_runtime_command(command_id, false, {}, "Editor interface unavailable")
		return true

	if command_name == "godot.project.run":
		if not ClassDB.class_has_method("EditorInterface", "play_main_scene"):
			protocol_adapter.ack_runtime_command(command_id, false, {}, "play_main_scene is not available")
			return true
		EditorInterface.play_main_scene()
		sync_callback.call(true)
		protocol_adapter.ack_runtime_command(command_id, true, {"running": true, "command": command_name}, "")
		return true

	if command_name == "godot.project.stop":
		if not ClassDB.class_has_method("EditorInterface", "stop_playing_scene"):
			protocol_adapter.ack_runtime_command(command_id, false, {}, "stop_playing_scene is not available")
			return true
		EditorInterface.stop_playing_scene()
		sync_callback.call(true)
		protocol_adapter.ack_runtime_command(command_id, true, {"running": false, "command": command_name}, "")
		return true

	if mutating_handlers.has(command_name):
		var handler_callable: Callable = mutating_handlers[command_name]
		var payload: Dictionary = handler_callable.call(arguments, editor_interface)
		var success := bool(payload.get("success", false))
		var result: Dictionary = {}
		var raw_result = payload.get("result", {})
		if raw_result is Dictionary:
			result = raw_result
		var error_message := str(payload.get("error", "")).strip_edges()
		var reason := str(result.get("reason", "")).strip_edges()
		var retryable: Variant = null
		if result.has("retryable") and result["retryable"] is bool:
			retryable = result["retryable"]
		var schema_version := str(result.get("schema_version", "v1")).strip_edges()
		protocol_adapter.ack_runtime_command(command_id, success, result, error_message, reason, retryable, schema_version)
		if success:
			sync_callback.call(true)
		return true

	return false
