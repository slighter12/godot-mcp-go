class_name RuntimeCommandDispatcher
extends RefCounted

const VARIANT_UTILS := preload("res://addons/godot_mcp/variant_utils.gd")

func dispatch(command_id: String, command_name: String, arguments: Dictionary, editor_interface: EditorInterface, protocol_adapter: MCPProtocolAdapter, mutating_handlers: Dictionary, sync_callback: Callable) -> bool:
	if protocol_adapter == null:
		return false
	if editor_interface == null:
		protocol_adapter.ack_runtime_command(command_id, false, {}, "Editor interface unavailable")
		return true

	if mutating_handlers.has(command_name):
		if command_name == "godot.project.run":
			var game_session_id := str(arguments.get("session_id", arguments.get("game_session_id", ""))).strip_edges()
			var editor_session_id := ""
			if protocol_adapter != null and protocol_adapter.mcp_client != null:
				var raw_session_id = protocol_adapter.mcp_client.get("session_id")
				if raw_session_id is String:
					editor_session_id = str(raw_session_id).strip_edges()
			var launch_token := str(arguments.get("launch_token", "")).strip_edges()
			print("Godot MCP Plugin: received godot.project.run - command_id=", command_id, " editor_session_id=", editor_session_id, " game_session_id=", game_session_id, " launch_token=", launch_token)
		var handler_callable: Callable = mutating_handlers[command_name]
		var payload: Dictionary = handler_callable.call(arguments, editor_interface)
		if command_name == "godot.project.run":
			print("Godot MCP Plugin: godot.project.run handler completed - command_id=", command_id, " success=", VARIANT_UTILS.to_bool(payload.get("success", false), false))
		var success := VARIANT_UTILS.to_bool(payload.get("success", false), false)
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

	# Compatibility fallback when project run/stop handlers are not provided.
	if command_name == "godot.project.run":
		if not ClassDB.class_has_method("EditorInterface", "play_main_scene"):
			protocol_adapter.ack_runtime_command(command_id, false, {}, "play_main_scene is not available")
			return true
		print("Godot MCP Plugin: received godot.project.run (compat fallback) - command_id=", command_id, " calling play_main_scene")
		EditorInterface.play_main_scene()
		sync_callback.call(true)
		print("Godot MCP Plugin: play_main_scene returned (compat fallback) - command_id=", command_id)
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

	return false
