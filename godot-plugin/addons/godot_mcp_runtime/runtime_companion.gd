extends Node

const CONFIG_PATH := "res://addons/godot_mcp_runtime/config.cfg"

const TOOL_RUNTIME_REGISTER := "godot.bridge.runtime.register"
const TOOL_RUNTIME_SNAPSHOT_PUSH := "godot.bridge.runtime.snapshot.push"
const TOOL_RUNTIME_LOG_PUSH := "godot.bridge.runtime.log.push"
const TOOL_COMMAND_ACK := "godot.bridge.command.ack"
const RUNTIME_SNAPSHOT_COLLECTOR := preload("res://addons/godot_mcp_runtime/runtime_snapshot_collector.gd")

const PROPERTY_WHITELIST := {
	"position": true,
	"global_position": true,
	"velocity": true,
	"visible": true,
	"modulate": true,
	"text": true,
	"frame": true,
	"animation": true,
	"enabled": true,
	"zoom": true
}

var mcp_client: RuntimeStreamableHTTPClient
var mcp_interface: RuntimeMCPProtocolAdapter
var snapshot_collector: RefCounted = RUNTIME_SNAPSHOT_COLLECTOR.new()

var bootstrap_timer: Timer
var snapshot_timer: Timer
var log_flush_timer: Timer

var streamable_http_url := "http://localhost:9080/mcp"
var handshake_path := "user://godot_mcp/runtime/active_handshake.json"
var handshake_scan_dir := "/tmp/godot-mcp"
var bootstrap_poll_seconds := 0.5
var snapshot_interval_seconds := 0.1
var log_flush_seconds := 1.0

var handshake_loaded := false
var game_session_id := ""
var editor_session_id := ""
var launch_token := ""
var game_scene_path := ""
var game_started_at := ""
var active_handshake_path := ""

var is_registered := false
var last_register_attempt_msec := 0
var snapshot_sequence := 0
var log_sequence := 0
var pending_log_entries: Array[Dictionary] = []
var log_push_in_flight := false
var pending_log_flush_batch: Array[Dictionary] = []

var _last_bootstrap_state := ""
var _last_handshake_scan_report := ""

func _enter_tree() -> void:
	print("Godot MCP Runtime: _enter_tree called - editor_hint=", Engine.is_editor_hint(), " pid=", OS.get_process_id())
	if Engine.is_editor_hint():
		return
	var current_scene_path := ""
	var current_tree := get_tree()
	if current_tree != null and current_tree.current_scene != null:
		current_scene_path = str(current_tree.current_scene.scene_file_path).strip_edges()
	print("Godot MCP Runtime: runtime addon started - scene_path=", current_scene_path)

func _ready() -> void:
	print("Godot MCP Runtime: _ready called - editor_hint=", Engine.is_editor_hint())
	if Engine.is_editor_hint():
		return

	_load_config()
	_setup_bridge_nodes()
	_setup_timers()
	print("Godot MCP Runtime: companion ready - handshake_path=", handshake_path, " scan_dir=", handshake_scan_dir)
	_append_log("info", "runtime companion initialized")
	_on_bootstrap_timeout()

func _exit_tree() -> void:
	if bootstrap_timer != null:
		bootstrap_timer.stop()
	if snapshot_timer != null:
		snapshot_timer.stop()
	if log_flush_timer != null:
		log_flush_timer.stop()

func _load_config() -> void:
	var cfg = ConfigFile.new()
	if cfg.load(CONFIG_PATH) != OK:
		return

	streamable_http_url = str(cfg.get_value("mcp_runtime", "streamable_http_url", streamable_http_url)).strip_edges()
	handshake_path = str(cfg.get_value("mcp_runtime", "handshake_path", handshake_path)).strip_edges()
	handshake_scan_dir = str(cfg.get_value("mcp_runtime", "handshake_scan_dir", handshake_scan_dir)).strip_edges()

	bootstrap_poll_seconds = max(0.1, float(cfg.get_value("mcp_runtime", "bootstrap_poll_seconds", bootstrap_poll_seconds)))
	var snapshot_hz = max(1.0, float(cfg.get_value("mcp_runtime", "snapshot_hz", 10.0)))
	snapshot_interval_seconds = 1.0 / snapshot_hz
	log_flush_seconds = max(0.2, float(cfg.get_value("mcp_runtime", "log_flush_seconds", log_flush_seconds)))

func _setup_bridge_nodes() -> void:
	mcp_client = RuntimeStreamableHTTPClient.new()
	mcp_client.name = "runtime_mcp_client"
	add_child(mcp_client)

	mcp_interface = RuntimeMCPProtocolAdapter.new()
	mcp_interface.name = "runtime_mcp_interface"
	mcp_interface.set_client(mcp_client)
	add_child(mcp_interface)

	mcp_client.connected.connect(_on_mcp_connected)
	mcp_client.disconnected.connect(_on_mcp_disconnected)
	mcp_client.error.connect(_on_mcp_error)
	mcp_client.message_received.connect(_on_mcp_message_received)
	mcp_interface.runtime_command_received.connect(_on_runtime_command_received)
	mcp_interface.tool_result.connect(_on_tool_result)
	mcp_interface.tool_error.connect(_on_tool_error)

func _setup_timers() -> void:
	bootstrap_timer = Timer.new()
	bootstrap_timer.one_shot = false
	bootstrap_timer.wait_time = bootstrap_poll_seconds
	bootstrap_timer.timeout.connect(_on_bootstrap_timeout)
	add_child(bootstrap_timer)
	bootstrap_timer.start()

	snapshot_timer = Timer.new()
	snapshot_timer.one_shot = false
	snapshot_timer.wait_time = snapshot_interval_seconds
	snapshot_timer.timeout.connect(_on_snapshot_timeout)
	add_child(snapshot_timer)
	# Start only after successful register.
	snapshot_timer.stop()

	log_flush_timer = Timer.new()
	log_flush_timer.one_shot = false
	log_flush_timer.wait_time = log_flush_seconds
	log_flush_timer.timeout.connect(_on_log_flush_timeout)
	add_child(log_flush_timer)
	log_flush_timer.start()

func _on_bootstrap_timeout() -> void:
	var has_register_tool := mcp_interface != null and mcp_interface.has_tool(TOOL_RUNTIME_REGISTER)
	var state_str := "handshake=%s connected=%s connecting=%s registered=%s has_register_tool=%s" % [
		str(handshake_loaded),
		str(_is_client_connected()),
		str(_is_client_connecting()),
		str(is_registered),
		str(has_register_tool)
	]
	if state_str != _last_bootstrap_state:
		print("Godot MCP Runtime: bootstrap tick - ", state_str)
		_last_bootstrap_state = state_str

	if not handshake_loaded:
		handshake_loaded = _load_handshake_from_candidates()
		if not handshake_loaded:
			return

	if not _is_client_connected():
		if not _is_client_connecting():
			mcp_client.connect_streamable_http(streamable_http_url)
		return

	if not is_registered:
		_attempt_runtime_register()

func _on_snapshot_timeout() -> void:
	if not is_registered:
		return
	if not _push_runtime_snapshot(false):
		_append_diagnostic_failure("runtime snapshot push skipped", "runtime_companion", "snapshot_push_unavailable")

func _on_log_flush_timeout() -> void:
	_flush_logs()

func _on_mcp_connected() -> void:
	_append_log("info", "connected to MCP server", "runtime_companion", {
		"url": streamable_http_url
	})

func _on_mcp_disconnected() -> void:
	is_registered = false
	if snapshot_timer != null:
		snapshot_timer.stop()
	_append_log("warning", "disconnected from MCP server", "runtime_companion")

func _on_mcp_message_received(message: Dictionary) -> void:
	if mcp_interface != null:
		mcp_interface.handle_message(message)

func _on_mcp_error(error_message: String) -> void:
	print("Godot MCP Runtime: mcp client error - ", error_message)
	_append_log("error", error_message, "runtime_companion")

func _on_tool_result(tool_name: String, result: Dictionary) -> void:
	if tool_name == TOOL_RUNTIME_REGISTER:
		last_register_attempt_msec = 0
		is_registered = true
		if snapshot_timer != null and snapshot_timer.is_stopped():
			snapshot_timer.start()
		print("Godot MCP Runtime: runtime register accepted - session_id=", game_session_id, " editor_session_id=", editor_session_id, " runtime_session_id=", str(mcp_client.get("session_id")))
		_append_log("info", "runtime session registered", "runtime_companion", {
			"game_session_id": game_session_id,
			"editor_session_id": editor_session_id,
			"handshake_path": active_handshake_path
		})
		print("Godot MCP Runtime: sending first runtime snapshot - game_session_id=", game_session_id, " runtime_session_id=", str(mcp_client.get("session_id")))
		if not _push_runtime_snapshot(true):
			_append_diagnostic_failure("runtime snapshot push skipped after register", "runtime_companion", "snapshot_push_unavailable")
		return

	if tool_name == TOOL_RUNTIME_LOG_PUSH:
		log_push_in_flight = false
		pending_log_flush_batch.clear()
		return
	if tool_name == TOOL_RUNTIME_SNAPSHOT_PUSH:
		return
	if tool_name == TOOL_COMMAND_ACK:
		return

func _on_tool_error(tool_name: String, error_message: String) -> void:
	if tool_name == TOOL_RUNTIME_REGISTER:
		is_registered = false
		last_register_attempt_msec = 0
		# Reset handshake state so bootstrap can reload a newer handshake payload
		# (for example after editor-side run attach/recover).
		handshake_loaded = false
		game_session_id = ""
		editor_session_id = ""
		launch_token = ""
		game_scene_path = ""
		game_started_at = ""
		active_handshake_path = ""
		_append_diagnostic_failure("runtime register failed", "runtime_companion", "register_failed", error_message)
		return

	if tool_name == TOOL_RUNTIME_SNAPSHOT_PUSH:
		_append_diagnostic_failure("runtime snapshot push failed", "runtime_companion", "snapshot_push_failed", error_message)
		return

	if tool_name == TOOL_RUNTIME_LOG_PUSH:
		log_push_in_flight = false
		var failed_batch = pending_log_flush_batch.duplicate(true)
		pending_log_flush_batch.clear()
		_restore_pending_logs(failed_batch)
		_append_diagnostic_failure("runtime log push failed", "runtime_companion", "log_push_failed", error_message)
		return

	if tool_name == TOOL_COMMAND_ACK:
		_append_diagnostic_failure("runtime command ack failed", "runtime_companion", "command_ack_failed", error_message)

func _attempt_runtime_register() -> void:
	if mcp_interface == null:
		print("Godot MCP Runtime: register skipped - reason=interface_null")
		return
	if not mcp_interface.has_tool(TOOL_RUNTIME_REGISTER):
		var known_tools = mcp_interface.get_tools().keys()
		print("Godot MCP Runtime: register skipped - reason=tool_not_listed tool=", TOOL_RUNTIME_REGISTER, " known_tools_count=", known_tools.size(), " first_10=", known_tools.slice(0, 10))
		return

	var now_ms = Time.get_ticks_msec()
	if now_ms - last_register_attempt_msec < 1000:
		return
	last_register_attempt_msec = now_ms

	print("Godot MCP Runtime: sending runtime register - game_session_id=", game_session_id, " editor_session_id=", editor_session_id, " handshake_path=", active_handshake_path)
	mcp_interface.call_tool(TOOL_RUNTIME_REGISTER, {
		"game_session_id": game_session_id,
		"session_id": game_session_id,
		"editor_session_id": editor_session_id,
		"scene_path": game_scene_path,
		"launch_token": launch_token,
		"started_at": game_started_at,
		"handshake_file": active_handshake_path,
		"source": "godot_mcp_runtime",
		"capabilities": {
			"snapshot_push": true,
			"log_push": true,
			"command_ack": true,
			"input": true,
			"screenshot": true,
			"node_properties": true
		},
		"runtime": {
			"engine": Engine.get_version_info(),
			"platform": OS.get_name()
		}
	})

func _push_runtime_snapshot(force: bool) -> bool:
	if mcp_interface == null:
		print("Godot MCP Runtime: snapshot skipped - reason=interface_null force=", force)
		return false
	if not mcp_interface.has_tool(TOOL_RUNTIME_SNAPSHOT_PUSH):
		print("Godot MCP Runtime: snapshot skipped - reason=tool_not_listed force=", force)
		return false

	snapshot_sequence += 1
	var snapshot_id = "snap_%08d" % snapshot_sequence
	var snapshot = snapshot_collector.collect_snapshot(game_session_id, snapshot_id)
	if force:
		snapshot["force"] = true
	if force or snapshot_sequence == 1:
		print("Godot MCP Runtime: sending runtime snapshot - game_session_id=", game_session_id, " snapshot_id=", snapshot_id, " frame=", int(snapshot.get("frame", 0)))

	mcp_interface.call_tool(TOOL_RUNTIME_SNAPSHOT_PUSH, {
		"game_session_id": game_session_id,
		"session_id": game_session_id,
		"launch_token": launch_token,
		"snapshot": snapshot
	})
	return true

func _on_runtime_command_received(command_id: String, command_name: String, arguments: Dictionary) -> void:
	var payload = _dispatch_runtime_command(command_name, arguments)
	if not (payload is Dictionary):
		payload = _runtime_command_failure(command_name, "command_failed", "runtime command handler returned invalid payload")

	_ack_runtime_command(command_id, payload)
	if bool(payload.get("success", false)) and _should_refresh_snapshot_after_command(command_name):
		_push_runtime_snapshot(true)

func _dispatch_runtime_command(command_name: String, arguments: Dictionary) -> Dictionary:
	var normalized = command_name.strip_edges().to_lower()
	match normalized:
		"godot.runtime.sync_now":
			if not _push_runtime_snapshot(true):
				return _runtime_command_failure("godot.runtime.sync_now", "command_failed", "runtime snapshot push is unavailable")
			return _runtime_success_result({
				"synced": true,
				"timestamp": _now_rfc3339(),
				"frame": int(Engine.get_process_frames())
			})
		"godot.runtime.node_properties.get":
			return _handle_node_properties_get(arguments)
		"godot.runtime.input.tap":
			return _handle_input_tap(arguments)
		"godot.runtime.input.press":
			return _handle_input_press(arguments)
		"godot.runtime.input.release":
			return _handle_input_release(arguments)
		"godot.runtime.log.clear":
			return _handle_log_clear()
		"godot.runtime.screenshot.get":
			return _handle_screenshot_get(arguments)
		_:
			return _runtime_command_failure(command_name, "command_not_supported", "unsupported runtime command: %s" % command_name)

func _should_refresh_snapshot_after_command(command_name: String) -> bool:
	var normalized = command_name.strip_edges().to_lower()
	return normalized in [
		"godot.runtime.input.tap",
		"godot.runtime.input.press",
		"godot.runtime.input.release"
	]

func _handle_node_properties_get(arguments: Dictionary) -> Dictionary:
	var node_query = str(arguments.get("node", "")).strip_edges()
	if node_query == "":
		return _runtime_command_failure("godot.runtime.node_properties.get", "node_not_found", "node path is required")

	var raw_properties = arguments.get("properties", [])
	if not (raw_properties is Array):
		return _runtime_command_failure("godot.runtime.node_properties.get", "property_not_supported", "properties must be an array")

	var target = snapshot_collector.resolve_node(node_query)
	if target == null:
		return _runtime_command_failure("godot.runtime.node_properties.get", "node_not_found", "node not found: %s" % node_query)

	var properties: Dictionary = {}
	for property_name_any in raw_properties:
		if not (property_name_any is String):
			return _runtime_command_failure("godot.runtime.node_properties.get", "property_not_supported", "property names must be strings")
		var property_name = str(property_name_any).strip_edges()
		if property_name == "":
			continue
		if not PROPERTY_WHITELIST.has(property_name):
			return _runtime_command_failure("godot.runtime.node_properties.get", "property_not_supported", "property not in whitelist: %s" % property_name)
		if not _node_has_property(target, property_name):
			return _runtime_command_failure("godot.runtime.node_properties.get", "property_not_supported", "property unavailable on node: %s" % property_name)
		properties[property_name] = _normalize_variant(target.get(property_name))

	return _runtime_success_result({
		"session_id": game_session_id,
		"snapshot_id": "snap_%08d" % snapshot_sequence,
		"node": str(target.get_path()),
		"type": str(target.get_class()),
		"properties": properties,
		"frame": int(Engine.get_process_frames()),
		"updated_at": _now_rfc3339()
	})

func _handle_input_tap(arguments: Dictionary) -> Dictionary:
	var parsed = _parse_input_descriptor(str(arguments.get("input", "")))
	if not bool(parsed.get("ok", false)):
		return _runtime_command_failure("godot.runtime.input.tap", "input_not_supported", str(parsed.get("error", "input not supported")))

	var duration_ms = int(arguments.get("duration_ms", 120))
	duration_ms = clampi(duration_ms, 0, 5000)

	_send_parsed_input(parsed, true)
	if duration_ms > 0:
		var release_timer = get_tree().create_timer(float(duration_ms) / 1000.0)
		release_timer.timeout.connect(
			func() -> void:
				_send_parsed_input(parsed, false),
			CONNECT_ONE_SHOT
		)
	else:
		_send_parsed_input(parsed, false)

	return _runtime_success_result({
		"input": parsed.get("input", ""),
		"duration_ms": duration_ms,
		"frame": int(Engine.get_process_frames()),
		"timestamp": _now_rfc3339()
	})

func _handle_input_press(arguments: Dictionary) -> Dictionary:
	var parsed = _parse_input_descriptor(str(arguments.get("input", "")))
	if not bool(parsed.get("ok", false)):
		return _runtime_command_failure("godot.runtime.input.press", "input_not_supported", str(parsed.get("error", "input not supported")))

	_send_parsed_input(parsed, true)
	return _runtime_success_result({
		"input": parsed.get("input", ""),
		"frame": int(Engine.get_process_frames()),
		"timestamp": _now_rfc3339()
	})

func _handle_input_release(arguments: Dictionary) -> Dictionary:
	var parsed = _parse_input_descriptor(str(arguments.get("input", "")))
	if not bool(parsed.get("ok", false)):
		return _runtime_command_failure("godot.runtime.input.release", "input_not_supported", str(parsed.get("error", "input not supported")))

	_send_parsed_input(parsed, false)
	return _runtime_success_result({
		"input": parsed.get("input", ""),
		"frame": int(Engine.get_process_frames()),
		"timestamp": _now_rfc3339()
	})

func _handle_log_clear() -> Dictionary:
	var cleared = pending_log_entries.size()
	pending_log_entries.clear()
	log_push_in_flight = false
	pending_log_flush_batch.clear()
	return _runtime_success_result({
		"session_id": game_session_id,
		"cleared": cleared,
		"timestamp": _now_rfc3339()
	})

func _handle_screenshot_get(arguments: Dictionary) -> Dictionary:
	var mode = str(arguments.get("mode", "viewport")).strip_edges().to_lower()
	if mode == "":
		mode = "viewport"
	if mode != "viewport":
		return _runtime_command_failure("godot.runtime.screenshot.get", "input_not_supported", "only viewport screenshot mode is supported")

	if get_viewport() == null or get_viewport().get_texture() == null:
		return _runtime_command_failure("godot.runtime.screenshot.get", "command_failed", "viewport texture unavailable")

	RenderingServer.force_draw(false)
	var image = get_viewport().get_texture().get_image()
	if image == null:
		return _runtime_command_failure("godot.runtime.screenshot.get", "command_failed", "failed to read viewport image")

	var base_dir = "user://godot-mcp-runtime/%s" % game_session_id
	var absolute_dir = ProjectSettings.globalize_path(base_dir)
	var mk_err = DirAccess.make_dir_recursive_absolute(absolute_dir)
	if mk_err != OK:
		return _runtime_command_failure("godot.runtime.screenshot.get", "command_failed", "failed to prepare screenshot directory")

	var frame = int(Engine.get_process_frames())
	var relative_path = "%s/frame_%08d.png" % [base_dir, frame]
	var save_err = image.save_png(relative_path)
	if save_err != OK:
		return _runtime_command_failure("godot.runtime.screenshot.get", "command_failed", "failed to save screenshot")

	return _runtime_success_result({
		"session_id": game_session_id,
		"path": ProjectSettings.globalize_path(relative_path),
		"width": image.get_width(),
		"height": image.get_height(),
		"frame": frame,
		"timestamp": _now_rfc3339()
	})

func _parse_input_descriptor(raw_input: String) -> Dictionary:
	var input_name = raw_input.strip_edges()
	if input_name == "":
		return {
			"ok": false,
			"error": "input is required"
		}

	if InputMap.has_action(input_name):
		return {
			"ok": true,
			"kind": "action",
			"action": input_name,
			"input": input_name
		}

	var keycode = _resolve_keycode(input_name)
	if keycode != KEY_NONE:
		return {
			"ok": true,
			"kind": "key",
			"keycode": keycode,
			"input": input_name.to_upper()
		}

	return {
		"ok": false,
		"error": "unsupported input: %s" % input_name
	}

func _resolve_keycode(raw_input: String) -> Key:
	var upper = raw_input.strip_edges().to_upper()
	if upper.begins_with("KEY_"):
		upper = upper.substr(4)

	match upper:
		"SPACE":
			return KEY_SPACE
		"E":
			return KEY_E
		"1":
			return KEY_1
		"2":
			return KEY_2
		"3":
			return KEY_3
		"LEFT":
			return KEY_LEFT
		"RIGHT":
			return KEY_RIGHT

	var lookup = upper.replace("_", " ")
	var found = OS.find_keycode_from_string(lookup)
	if found != KEY_NONE:
		return found

	return KEY_NONE

func _send_parsed_input(parsed: Dictionary, pressed: bool) -> void:
	var kind = str(parsed.get("kind", ""))
	if kind == "action":
		var event = InputEventAction.new()
		event.action = str(parsed.get("action", ""))
		event.pressed = pressed
		event.strength = 1.0 if pressed else 0.0
		Input.parse_input_event(event)
		return

	if kind == "key":
		var key_event = InputEventKey.new()
		var keycode: Key = parsed.get("keycode", KEY_NONE)
		key_event.keycode = keycode
		key_event.physical_keycode = keycode
		key_event.pressed = pressed
		Input.parse_input_event(key_event)

func _ack_runtime_command(command_id: String, payload: Dictionary) -> void:
	if mcp_interface == null:
		return
	if not mcp_interface.has_tool(TOOL_COMMAND_ACK):
		_append_log("warning", "bridge command ack tool is unavailable")
		return

	var success = bool(payload.get("success", false))
	var result = payload.get("result", {})
	if not (result is Dictionary):
		result = {}

	var arguments: Dictionary = {
		"command_id": command_id,
		"success": success,
		"result": result,
		"session_id": game_session_id,
		"game_session_id": game_session_id,
		"launch_token": launch_token
	}

	var error_message = str(payload.get("error", "")).strip_edges()
	if error_message != "":
		arguments["error"] = error_message
	if result.has("reason"):
		arguments["reason"] = str(result.get("reason", "")).strip_edges()
	if result.has("retryable"):
		arguments["retryable"] = bool(result.get("retryable", false))
	if result.has("schema_version"):
		arguments["schema_version"] = str(result.get("schema_version", "v1")).strip_edges()

	mcp_interface.call_tool(TOOL_COMMAND_ACK, arguments)

func _runtime_success_result(data: Dictionary = {}) -> Dictionary:
	var result: Dictionary = {
		"schema_version": "v1"
	}
	for key in data.keys():
		result[key] = data[key]
	return {
		"success": true,
		"result": result
	}

func _runtime_failure_result(reason: String, error_message: String) -> Dictionary:
	var trimmed_reason = reason.strip_edges()
	if trimmed_reason == "":
		trimmed_reason = "command_failed"
	var trimmed_error = error_message.strip_edges()
	if trimmed_error == "":
		trimmed_error = "runtime command failed"
	return {
		"success": false,
		"result": {
			"reason": trimmed_reason,
			"retryable": false,
			"schema_version": "v1"
		},
		"error": trimmed_error
	}

func _runtime_command_failure(command_name: String, reason: String, error_message: String) -> Dictionary:
	var payload = _runtime_failure_result(reason, error_message)
	_append_diagnostic_failure(error_message, _runtime_command_source(command_name), reason, "")
	return payload

func _append_log(level: String, message: String, source: String = "runtime_companion", context: Dictionary = {}, stack_trace: String = "") -> void:
	log_sequence += 1
	var entry: Dictionary = {
		"sequence": log_sequence,
		"time": _now_rfc3339(),
		"level": level.strip_edges().to_lower(),
		"message": message,
		"source": source
	}
	if not context.is_empty():
		entry["context"] = context
	var trimmed_stack_trace = stack_trace.strip_edges()
	if trimmed_stack_trace != "":
		entry["stack_trace"] = trimmed_stack_trace

	pending_log_entries.append(entry)
	if pending_log_entries.size() > 500:
		pending_log_entries.pop_front()

func _append_diagnostic_failure(message: String, source: String, reason: String, detail: String = "", stack_trace: String = "") -> void:
	var parts: Array[String] = []
	var trimmed_message = message.strip_edges()
	if trimmed_message != "":
		parts.append(trimmed_message)
	var trimmed_reason = reason.strip_edges()
	if trimmed_reason != "":
		parts.append("reason=%s" % trimmed_reason)
	var trimmed_detail = detail.strip_edges()
	if trimmed_detail != "":
		parts.append("detail=%s" % trimmed_detail)

	var final_message = "runtime diagnostics failure"
	if not parts.is_empty():
		final_message = " | ".join(parts)

	var context := {}
	if trimmed_reason != "":
		context["reason"] = trimmed_reason
	if trimmed_detail != "":
		context["detail"] = trimmed_detail
	_append_log("error", final_message, source, context, stack_trace)

func _runtime_command_source(command_name: String) -> String:
	var normalized = command_name.strip_edges().to_lower()
	if normalized == "":
		normalized = "unknown"
	return "runtime_command:%s" % normalized

func _flush_logs() -> void:
	if not is_registered:
		return
	if mcp_interface == null:
		return
	if log_push_in_flight:
		return
	if pending_log_entries.is_empty():
		return
	if not mcp_interface.has_tool(TOOL_RUNTIME_LOG_PUSH):
		return

	var entries = pending_log_entries.duplicate(true)
	pending_log_entries.clear()
	pending_log_flush_batch = entries.duplicate(true)
	log_push_in_flight = true

	mcp_interface.call_tool(TOOL_RUNTIME_LOG_PUSH, {
		"game_session_id": game_session_id,
		"session_id": game_session_id,
		"launch_token": launch_token,
		"entries": entries
	})

func _restore_pending_logs(entries: Array[Dictionary]) -> void:
	if entries.is_empty():
		return
	var restored: Array[Dictionary] = []
	for entry in entries:
		restored.append(entry)
	for entry in pending_log_entries:
		restored.append(entry)
	pending_log_entries = restored
	while pending_log_entries.size() > 500:
		pending_log_entries.pop_front()

func _load_handshake_from_candidates() -> bool:
	var candidates: Array[String] = []

	var env_path = OS.get_environment("GODOT_MCP_RUNTIME_HANDSHAKE_PATH").strip_edges()
	if env_path != "":
		candidates.append(env_path)
	if handshake_path != "":
		candidates.append(handshake_path)
	var editor_active_handshake = "user://godot_mcp/runtime/active_handshake.json"
	if not candidates.has(editor_active_handshake):
		candidates.append(editor_active_handshake)

	var discovered = _discover_latest_handshake_path()
	if discovered != "" and not candidates.has(discovered):
		candidates.append(discovered)

	for candidate in candidates:
		var payload = _read_json_file(candidate)
		if payload.is_empty():
			continue
		if _apply_handshake_payload(payload, candidate):
			return true

	var report = _debug_candidate_report(candidates)
	if report != _last_handshake_scan_report:
		print("Godot MCP Runtime: handshake scan failed - candidates=", candidates, " detail=[", report, "]")
		_last_handshake_scan_report = report
	return false

func _debug_candidate_report(candidates: Array[String]) -> String:
	var parts: Array[String] = []
	for candidate in candidates:
		if candidate == "":
			continue
		var exists := FileAccess.file_exists(candidate)
		var size := 0
		var state := ""
		if exists:
			var f = FileAccess.open(candidate, FileAccess.READ)
			if f != null:
				size = int(f.get_length())
				f.close()
			var payload = _read_json_file(candidate)
			if payload is Dictionary:
				state = str(payload.get("state", "")).strip_edges()
		parts.append("%s(exists=%s,size=%d,state=%s)" % [candidate, str(exists), size, state])
	return " | ".join(parts)

func _discover_latest_handshake_path() -> String:
	var scan_dirs: Array[String] = []
	var env_dir = OS.get_environment("GODOT_MCP_RUNTIME_HANDSHAKE_DIR").strip_edges()
	if env_dir != "":
		scan_dirs.append(env_dir)
	if handshake_scan_dir != "":
		scan_dirs.append(handshake_scan_dir)

	var latest_path := ""
	var latest_modified := 0

	for scan_dir in scan_dirs:
		if scan_dir == "":
			continue
		if not DirAccess.dir_exists_absolute(scan_dir):
			continue
		var dir = DirAccess.open(scan_dir)
		if dir == null:
			continue
		dir.list_dir_begin()
		var file_name = dir.get_next()
		while file_name != "":
			if not dir.current_is_dir() and file_name.to_lower().ends_with(".json"):
				var full_path = scan_dir.path_join(file_name)
				var modified = FileAccess.get_modified_time(full_path)
				if modified > latest_modified:
					latest_modified = modified
					latest_path = full_path
			file_name = dir.get_next()
		dir.list_dir_end()

	return latest_path

func _read_json_file(path: String) -> Dictionary:
	if path.strip_edges() == "":
		return {}
	if not FileAccess.file_exists(path):
		return {}
	var file = FileAccess.open(path, FileAccess.READ)
	if file == null:
		return {}
	var raw = file.get_as_text()
	file.close()
	if raw.strip_edges() == "":
		return {}
	var parsed = JSON.parse_string(raw)
	if parsed is Dictionary:
		return parsed
	return {}

func _apply_handshake_payload(payload: Dictionary, source_path: String) -> bool:
	var handshake_state = _pick_first_string(payload, [
		"state"
	]).to_lower()
	if handshake_state == "stopped":
		_append_log("warning", "ignored stopped handshake payload", "runtime_lifecycle", {
			"path": source_path
		})
		return false

	var candidate_session = _pick_first_string(payload, [
		"game_session_id",
		"session_id"
	])
	var candidate_launch_token = _pick_first_string(payload, [
		"launch_token",
		"token"
	])
	var candidate_url = _pick_first_string(payload, [
		"streamable_http_url",
		"server_url",
		"mcp_url",
		"mcp_streamable_http_url"
	])
	if candidate_url == "":
		candidate_url = streamable_http_url

	if candidate_session == "" or candidate_launch_token == "" or candidate_url == "":
		_append_diagnostic_failure("runtime handshake payload incomplete", "runtime_lifecycle", "handshake_payload_invalid", source_path)
		return false

	game_session_id = candidate_session
	editor_session_id = _pick_first_string(payload, [
		"editor_session_id"
	])
	launch_token = candidate_launch_token
	game_scene_path = _pick_first_string(payload, [
		"scene_path"
	])
	game_started_at = _pick_first_string(payload, [
		"started_at"
	])
	streamable_http_url = candidate_url
	active_handshake_path = source_path

	if payload.has("snapshot_hz"):
		var hz = max(1.0, float(payload.get("snapshot_hz", 10.0)))
		snapshot_interval_seconds = 1.0 / hz
		if snapshot_timer != null:
			snapshot_timer.wait_time = snapshot_interval_seconds
	if payload.has("log_flush_seconds"):
		log_flush_seconds = max(0.2, float(payload.get("log_flush_seconds", log_flush_seconds)))
		if log_flush_timer != null:
			log_flush_timer.wait_time = log_flush_seconds

	_append_log("info", "handshake loaded", "runtime_companion", {
		"path": source_path,
		"game_session_id": game_session_id,
		"editor_session_id": editor_session_id,
		"scene_path": game_scene_path,
		"streamable_http_url": streamable_http_url
	})
	print("Godot MCP Runtime: handshake loaded - game_session_id=", game_session_id, " editor_session_id=", editor_session_id, " launch_token=", launch_token, " scene_path=", game_scene_path)

	return true

func _pick_first_string(payload: Dictionary, keys: Array) -> String:
	for key_any in keys:
		if not (key_any is String):
			continue
		var key = str(key_any)
		if payload.has(key):
			var value = str(payload.get(key, "")).strip_edges()
			if value != "":
				return value
	return ""

func _is_client_connected() -> bool:
	if mcp_client == null:
		return false
	return bool(mcp_client.get("is_connected"))

func _is_client_connecting() -> bool:
	if mcp_client == null:
		return false
	return bool(mcp_client.get("is_connecting"))

func _node_has_property(node: Node, property_name: String) -> bool:
	for property_info in node.get_property_list():
		if property_info is Dictionary and str(property_info.get("name", "")) == property_name:
			return true
	return false

func _normalize_variant(value: Variant) -> Variant:
	if value is Vector2:
		var v2: Vector2 = value
		return {"x": v2.x, "y": v2.y}
	if value is Vector3:
		var v3: Vector3 = value
		return {"x": v3.x, "y": v3.y, "z": v3.z}
	if value is Color:
		var c: Color = value
		return {"r": c.r, "g": c.g, "b": c.b, "a": c.a}
	if value is Rect2:
		var r: Rect2 = value
		return {
			"position": {"x": r.position.x, "y": r.position.y},
			"size": {"x": r.size.x, "y": r.size.y}
		}
	if value is Array:
		var out: Array = []
		for item in value:
			out.append(_normalize_variant(item))
		return out
	if value is Dictionary:
		var out_dict: Dictionary = {}
		for key in value.keys():
			out_dict[key] = _normalize_variant(value[key])
		return out_dict
	return value

func _now_rfc3339() -> String:
	return "%sZ" % Time.get_datetime_string_from_system(true)
