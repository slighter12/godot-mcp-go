@tool
extends EditorPlugin

const DEFAULT_RUNTIME_HEARTBEAT_SECONDS := 5.0
const DEFAULT_RUNTIME_CHANGE_POLL_SECONDS := 0.5
const MIN_RUNTIME_HEARTBEAT_SECONDS := 1.0
const MIN_RUNTIME_CHANGE_POLL_SECONDS := 0.1
const MAX_RUNTIME_TREE_DEPTH := 12
const MAX_RUNTIME_NODE_COUNT := 2000

var mcp_client: Node
var mcp_interface: Node
var settings_dialog: AcceptDialog
var runtime_heartbeat_timer: Timer
var runtime_change_timer: Timer
var current_streamable_http_url: String = "http://localhost:9080/mcp"
var runtime_heartbeat_seconds: float = DEFAULT_RUNTIME_HEARTBEAT_SECONDS
var runtime_change_poll_seconds: float = DEFAULT_RUNTIME_CHANGE_POLL_SECONDS
var last_snapshot_fingerprint: String = ""

func _enter_tree():
    print("Godot MCP Plugin: Entering tree...")

    # Create MCP client node.
    mcp_client = preload("res://addons/godot_mcp/mcp_server.gd").new()
    mcp_client.name = "mcp_client"
    add_child(mcp_client)

    # Create MCP interface node.
    mcp_interface = preload("res://addons/godot_mcp/mcp_interface.gd").new()
    mcp_interface.name = "mcp_interface"
    add_child(mcp_interface)
    mcp_interface.set_mcp_client(mcp_client)

    # Connect signals.
    mcp_client.connected.connect(Callable(self, "_on_mcp_connected"))
    mcp_client.disconnected.connect(Callable(self, "_on_mcp_disconnected"))
    mcp_client.error.connect(Callable(self, "_on_mcp_error"))
    mcp_client.message_received.connect(Callable(self, "_on_mcp_message_received"))
    mcp_interface.runtime_sync_failed.connect(Callable(self, "_on_runtime_sync_failed"))
    mcp_interface.runtime_command_received.connect(Callable(self, "_on_runtime_command_received"))
    mcp_interface.tool_result.connect(Callable(self, "_on_mcp_tool_result"))

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
        runtime_heartbeat_seconds = _resolve_interval_setting(
            config.get_value("mcp", "runtime_heartbeat_seconds", DEFAULT_RUNTIME_HEARTBEAT_SECONDS),
            MIN_RUNTIME_HEARTBEAT_SECONDS,
            DEFAULT_RUNTIME_HEARTBEAT_SECONDS
        )
        runtime_change_poll_seconds = _resolve_interval_setting(
            config.get_value("mcp", "runtime_change_poll_seconds", DEFAULT_RUNTIME_CHANGE_POLL_SECONDS),
            MIN_RUNTIME_CHANGE_POLL_SECONDS,
            DEFAULT_RUNTIME_CHANGE_POLL_SECONDS
        )
    else:
        print("Godot MCP Plugin: Failed to load config, using default connection")
        runtime_heartbeat_seconds = DEFAULT_RUNTIME_HEARTBEAT_SECONDS
        runtime_change_poll_seconds = DEFAULT_RUNTIME_CHANGE_POLL_SECONDS

    _setup_runtime_sync_timers()
    print("Godot MCP Plugin: Connecting with streamable_http")
    # Defer until mcp_client._ready() initializes its HTTPRequest node.
    mcp_client.call_deferred("connect_streamable_http", current_streamable_http_url)
    print("Godot MCP Plugin: Initialized successfully")

func _exit_tree():
    print("Godot MCP Plugin: Exiting tree...")
    remove_tool_menu_item("MCP Settings")

    _cleanup_timer(runtime_heartbeat_timer, "_on_runtime_heartbeat_timeout")
    _cleanup_timer(runtime_change_timer, "_on_runtime_change_timeout")
    runtime_heartbeat_timer = null
    runtime_change_timer = null

    _disconnect_signal_if_connected(mcp_client, "connected", "_on_mcp_connected")
    _disconnect_signal_if_connected(mcp_client, "disconnected", "_on_mcp_disconnected")
    _disconnect_signal_if_connected(mcp_client, "error", "_on_mcp_error")
    _disconnect_signal_if_connected(mcp_client, "message_received", "_on_mcp_message_received")
    _disconnect_signal_if_connected(mcp_interface, "runtime_sync_failed", "_on_runtime_sync_failed")
    _disconnect_signal_if_connected(mcp_interface, "runtime_command_received", "_on_runtime_command_received")
    _disconnect_signal_if_connected(mcp_interface, "tool_result", "_on_mcp_tool_result")

    if mcp_client:
        mcp_client.disconnect_from_server()
        mcp_client.queue_free()

    if mcp_interface:
        mcp_interface.queue_free()

    if settings_dialog:
        settings_dialog.queue_free()
    print("Godot MCP Plugin: Cleanup complete")

func _cleanup_timer(timer: Timer, timeout_handler: String) -> void:
    if timer == null:
        return
    timer.stop()
    var timeout_callable := Callable(self, timeout_handler)
    if timer.timeout.is_connected(timeout_callable):
        timer.timeout.disconnect(timeout_callable)
    timer.queue_free()

func _disconnect_signal_if_connected(source: Object, signal_name: StringName, handler_name: String) -> void:
    if source == null:
        return
    var handler := Callable(self, handler_name)
    if source.is_connected(signal_name, handler):
        source.disconnect(signal_name, handler)

func _on_mcp_connected():
    print("Godot MCP Plugin: Connected to MCP server")
    last_snapshot_fingerprint = ""
    _sync_runtime_snapshot(true)
    if mcp_interface != null and mcp_interface.tools.has("godot-runtime-get-health"):
        mcp_interface.call_tool("godot-runtime-get-health", {})

func _on_mcp_disconnected():
    print("Godot MCP Plugin: Disconnected from MCP server")

func _on_mcp_error(error: String):
    print("Godot MCP Plugin: Error: ", error)

func _on_mcp_message_received(message: Dictionary):
    mcp_interface.handle_message(message)

func _on_settings_pressed():
    print("MCP Plugin: Opening settings dialog")
    settings_dialog.popup_centered()

func _on_settings_saved(streamable_http_url: String):
    current_streamable_http_url = streamable_http_url
    print("Godot MCP Plugin: Reconnecting with updated streamable_http URL")
    mcp_client.connect_streamable_http(current_streamable_http_url)

func _on_runtime_sync_failed(error_message: String):
    print("Godot MCP Plugin: Runtime sync failed: ", error_message)

func _on_runtime_command_received(command_id: String, command_name: String, arguments: Dictionary) -> void:
    var editor_interface = get_editor_interface()
    if editor_interface == null:
        mcp_interface.ack_runtime_command(command_id, false, {}, "Editor interface unavailable")
        return

    if command_name == "godot-project-run":
        if not editor_interface.has_method("play_main_scene"):
            mcp_interface.ack_runtime_command(command_id, false, {}, "play_main_scene is not available")
            return
        editor_interface.play_main_scene()
        _sync_runtime_snapshot(true)
        mcp_interface.ack_runtime_command(command_id, true, {"running": true, "command": command_name}, "")
        return

    if command_name == "godot-project-stop":
        if not editor_interface.has_method("stop_playing_scene"):
            mcp_interface.ack_runtime_command(command_id, false, {}, "stop_playing_scene is not available")
            return
        editor_interface.stop_playing_scene()
        _sync_runtime_snapshot(true)
        mcp_interface.ack_runtime_command(command_id, true, {"running": false, "command": command_name}, "")
        return

    if command_name == "godot-scene-create":
        var scene_create_payload = _handle_scene_create(arguments)
        _ack_runtime_command_with_payload(command_id, scene_create_payload)
        _sync_runtime_snapshot_if_success(scene_create_payload)
        return

    if command_name == "godot-scene-save":
        var scene_save_payload = _handle_scene_save(editor_interface)
        _ack_runtime_command_with_payload(command_id, scene_save_payload)
        _sync_runtime_snapshot_if_success(scene_save_payload)
        return

    if command_name == "godot-scene-apply":
        var scene_apply_payload = _handle_scene_apply(editor_interface, arguments)
        _ack_runtime_command_with_payload(command_id, scene_apply_payload)
        _sync_runtime_snapshot_if_success(scene_apply_payload)
        return

    if command_name == "godot-node-create":
        var node_create_payload = _handle_node_create(editor_interface, arguments)
        _ack_runtime_command_with_payload(command_id, node_create_payload)
        _sync_runtime_snapshot_if_success(node_create_payload)
        return

    if command_name == "godot-node-delete":
        var node_delete_payload = _handle_node_delete(editor_interface, arguments)
        _ack_runtime_command_with_payload(command_id, node_delete_payload)
        _sync_runtime_snapshot_if_success(node_delete_payload)
        return

    if command_name == "godot-node-modify":
        var node_modify_payload = _handle_node_modify(editor_interface, arguments)
        _ack_runtime_command_with_payload(command_id, node_modify_payload)
        _sync_runtime_snapshot_if_success(node_modify_payload)
        return

    if command_name == "godot-script-create":
        var script_create_payload = _handle_script_create(arguments)
        _ack_runtime_command_with_payload(command_id, script_create_payload)
        _sync_runtime_snapshot_if_success(script_create_payload)
        return

    if command_name == "godot-script-modify":
        var script_modify_payload = _handle_script_modify(arguments)
        _ack_runtime_command_with_payload(command_id, script_modify_payload)
        _sync_runtime_snapshot_if_success(script_modify_payload)
        return

    mcp_interface.ack_runtime_command(command_id, false, {}, "Unsupported runtime command: " + command_name)

func _on_mcp_tool_result(tool_name: String, result: Dictionary) -> void:
    if tool_name != "godot-runtime-get-health":
        return
    _apply_runtime_bridge_health(result)

func _ack_runtime_command_with_payload(command_id: String, payload: Dictionary) -> void:
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
    mcp_interface.ack_runtime_command(command_id, success, result, error_message, reason, retryable, schema_version)

func _sync_runtime_snapshot_if_success(payload: Dictionary) -> void:
    if bool(payload.get("success", false)):
        _sync_runtime_snapshot(true)

func _runtime_success_result(data: Dictionary = {}) -> Dictionary:
    var result = {
        "schema_version": "v1"
    }
    for key in data.keys():
        result[key] = data[key]
    return {
        "success": true,
        "result": result
    }

func _runtime_failure_result(reason: String, error_message: String) -> Dictionary:
    return {
        "success": false,
        "result": {
            "reason": reason,
            "retryable": false,
            "schema_version": "v1"
        },
        "error": error_message
    }

func _handle_scene_create(arguments: Dictionary) -> Dictionary:
    var scene_path = str(arguments.get("path", "")).strip_edges()
    if not _is_safe_res_path(scene_path, [".tscn"]):
        return _runtime_failure_result("invalid_path", "scene create requires a safe res://*.tscn path")
    if FileAccess.file_exists(scene_path):
        return _runtime_failure_result("scene_already_exists", "scene file already exists: " + scene_path)

    var content := ""
    if arguments.has("content"):
        if not (arguments["content"] is String):
            return _runtime_failure_result("invalid_content_type", "content must be a string")
        content = str(arguments["content"])
    elif arguments.has("template"):
        if not (arguments["template"] is String):
            return _runtime_failure_result("invalid_template_type", "template must be a string")
        content = _build_scene_template(str(arguments["template"]))
    else:
        content = _build_scene_template("")

    var mkdir_err = DirAccess.make_dir_recursive_absolute(ProjectSettings.globalize_path(scene_path.get_base_dir()))
    if mkdir_err != OK:
        return _runtime_failure_result("directory_create_failed", "failed to create scene directory (code=%d)" % mkdir_err)

    var file = FileAccess.open(scene_path, FileAccess.WRITE)
    if file == null:
        return _runtime_failure_result("scene_write_failed", "failed to open scene file for writing: " + scene_path)
    file.store_string(content)
    file.flush()
    file.close()

    return _runtime_success_result({
        "path": scene_path,
        "bytes_written": content.length()
    })

func _handle_scene_save(editor_interface: EditorInterface) -> Dictionary:
    var edited_root = editor_interface.get_edited_scene_root()
    if edited_root == null:
        return _runtime_failure_result("no_edited_scene", "no edited scene is available to save")
    if not editor_interface.has_method("save_scene"):
        return _runtime_failure_result("save_scene_unavailable", "save_scene is not available")

    var save_result = editor_interface.call("save_scene")
    if save_result is int and int(save_result) != OK:
        return _runtime_failure_result("save_failed", "save_scene failed (code=%d)" % int(save_result))

    return _runtime_success_result({
        "scene_path": _resolve_active_scene_path(edited_root)
    })

func _handle_scene_apply(editor_interface: EditorInterface, arguments: Dictionary) -> Dictionary:
    var scene_path = str(arguments.get("path", "")).strip_edges()
    if not _is_safe_res_path(scene_path, [".tscn"]):
        return _runtime_failure_result("invalid_path", "scene apply requires a safe res://*.tscn path")
    if not FileAccess.file_exists(scene_path):
        return _runtime_failure_result("scene_not_found", "scene file does not exist: " + scene_path)
    if not editor_interface.has_method("open_scene_from_path"):
        return _runtime_failure_result("open_scene_unavailable", "open_scene_from_path is not available")

    var open_result = editor_interface.call("open_scene_from_path", scene_path)
    if open_result is int and int(open_result) != OK:
        return _runtime_failure_result("scene_open_failed", "failed to open scene (code=%d)" % int(open_result))

    return _runtime_success_result({
        "scene_path": scene_path
    })

func _handle_node_create(editor_interface: EditorInterface, arguments: Dictionary) -> Dictionary:
    var edited_root = editor_interface.get_edited_scene_root()
    if edited_root == null:
        return _runtime_failure_result("no_edited_scene", "node create requires an edited scene")
    if not (arguments.get("parent", null) is String):
        return _runtime_failure_result("invalid_parent_type", "parent must be a string")
    if not (arguments.get("type", null) is String):
        return _runtime_failure_result("invalid_type_type", "type must be a string")
    if not (arguments.get("name", null) is String):
        return _runtime_failure_result("invalid_name_type", "name must be a string")

    var parent_path = str(arguments.get("parent", "")).strip_edges()
    var node_type = str(arguments.get("type", "")).strip_edges()
    var node_name = str(arguments.get("name", "")).strip_edges()
    if parent_path == "" or node_type == "" or node_name == "":
        return _runtime_failure_result("missing_required_field", "parent, type, and name are required")

    var parent_node = _resolve_scene_node(edited_root, parent_path)
    if parent_node == null:
        return _runtime_failure_result("parent_not_found", "parent node not found: " + parent_path)
    if not ClassDB.class_exists(node_type):
        return _runtime_failure_result("node_type_not_found", "unknown node type: " + node_type)

    var instance = ClassDB.instantiate(node_type)
    if instance == null or not (instance is Node):
        return _runtime_failure_result("node_type_not_instantiable", "failed to instantiate node type: " + node_type)

    var created_node: Node = instance
    created_node.name = node_name
    parent_node.add_child(created_node)
    created_node.owner = edited_root

    return _runtime_success_result({
        "path": str(created_node.get_path()),
        "parent": str(parent_node.get_path()),
        "name": str(created_node.name),
        "type": node_type
    })

func _handle_node_delete(editor_interface: EditorInterface, arguments: Dictionary) -> Dictionary:
    var edited_root = editor_interface.get_edited_scene_root()
    if edited_root == null:
        return _runtime_failure_result("no_edited_scene", "node delete requires an edited scene")
    if not (arguments.get("node", null) is String):
        return _runtime_failure_result("invalid_node_type", "node must be a string")

    var node_path = str(arguments.get("node", "")).strip_edges()
    if node_path == "":
        return _runtime_failure_result("missing_node_path", "node path is required")

    var target = _resolve_scene_node(edited_root, node_path)
    if target == null:
        return _runtime_failure_result("node_not_found", "node not found: " + node_path)
    if target == edited_root:
        return _runtime_failure_result("cannot_delete_root", "cannot delete the edited scene root node")

    var parent = target.get_parent()
    if parent == null:
        return _runtime_failure_result("node_parent_missing", "node parent is unavailable")

    parent.remove_child(target)
    target.queue_free()

    return _runtime_success_result({
        "deleted_path": node_path
    })

func _handle_node_modify(editor_interface: EditorInterface, arguments: Dictionary) -> Dictionary:
    var edited_root = editor_interface.get_edited_scene_root()
    if edited_root == null:
        return _runtime_failure_result("no_edited_scene", "node modify requires an edited scene")
    if not (arguments.get("node", null) is String):
        return _runtime_failure_result("invalid_node_type", "node must be a string")
    if not (arguments.get("properties", null) is Dictionary):
        return _runtime_failure_result("invalid_properties_type", "properties must be an object")

    var node_path = str(arguments.get("node", "")).strip_edges()
    if node_path == "":
        return _runtime_failure_result("missing_node_path", "node path is required")

    var target = _resolve_scene_node(edited_root, node_path)
    if target == null:
        return _runtime_failure_result("node_not_found", "node not found: " + node_path)

    var updates: Dictionary = arguments.get("properties", {})
    var updated_keys: Array[String] = []
    for key in updates.keys():
        if not (key is String):
            return _runtime_failure_result("invalid_property_name", "property names must be strings")
        var property_name = str(key).strip_edges()
        if property_name == "":
            return _runtime_failure_result("invalid_property_name", "property name must not be empty")
        if not _node_has_property(target, property_name):
            return _runtime_failure_result("property_not_found", "property not found: " + property_name)
        target.set(property_name, updates[key])
        var after_value = target.get(property_name)
        if after_value != updates[key]:
            return _runtime_failure_result("property_update_failed", "failed to update property: " + property_name)
        updated_keys.append(property_name)

    return _runtime_success_result({
        "path": str(target.get_path()),
        "updated_properties": updated_keys
    })

func _handle_script_create(arguments: Dictionary) -> Dictionary:
    if not (arguments.get("path", null) is String):
        return _runtime_failure_result("invalid_path_type", "path must be a string")
    if not (arguments.get("content", null) is String):
        return _runtime_failure_result("invalid_content_type", "content must be a string")

    var script_path = str(arguments.get("path", "")).strip_edges()
    var content = str(arguments.get("content", ""))
    var replace_existing := false
    if arguments.has("replace"):
        if not (arguments.get("replace", null) is bool):
            return _runtime_failure_result("invalid_replace_type", "replace must be a boolean")
        replace_existing = bool(arguments.get("replace", false))
    if not _is_safe_res_path(script_path, [".gd", ".rs"]):
        return _runtime_failure_result("invalid_path", "script create requires a safe res:// path with .gd or .rs extension")
    if FileAccess.file_exists(script_path) and not replace_existing:
        return _runtime_failure_result("script_exists_requires_replace", "script file already exists and replace=false: " + script_path)

    var mkdir_err = DirAccess.make_dir_recursive_absolute(ProjectSettings.globalize_path(script_path.get_base_dir()))
    if mkdir_err != OK:
        return _runtime_failure_result("directory_create_failed", "failed to create script directory (code=%d)" % mkdir_err)

    var file = FileAccess.open(script_path, FileAccess.WRITE)
    if file == null:
        return _runtime_failure_result("script_write_failed", "failed to open script file for writing: " + script_path)
    file.store_string(content)
    file.flush()
    file.close()

    return _runtime_success_result({
        "path": script_path,
        "bytes_written": content.length(),
        "replaced": replace_existing
    })

func _handle_script_modify(arguments: Dictionary) -> Dictionary:
    if not (arguments.get("path", null) is String):
        return _runtime_failure_result("invalid_path_type", "path must be a string")
    if not (arguments.get("content", null) is String):
        return _runtime_failure_result("invalid_content_type", "content must be a string")

    var script_path = str(arguments.get("path", "")).strip_edges()
    var content = str(arguments.get("content", ""))
    if not _is_safe_res_path(script_path, [".gd", ".rs"]):
        return _runtime_failure_result("invalid_path", "script modify requires a safe res:// path with .gd or .rs extension")
    if not FileAccess.file_exists(script_path):
        return _runtime_failure_result("script_not_found", "script file does not exist: " + script_path)

    var file = FileAccess.open(script_path, FileAccess.WRITE)
    if file == null:
        return _runtime_failure_result("script_write_failed", "failed to open script file for writing: " + script_path)
    file.store_string(content)
    file.flush()
    file.close()

    return _runtime_success_result({
        "path": script_path,
        "bytes_written": content.length()
    })

func _is_safe_res_path(path: String, allowed_extensions: Array[String]) -> bool:
    var normalized = _normalize_res_path(path)
    if normalized == "":
        return false

    var lowered = normalized.to_lower()
    if lowered == "res://addons" or lowered.begins_with("res://addons/"):
        return false
    if lowered == "res://.godot" or lowered.begins_with("res://.godot/"):
        return false
    if lowered == "res://.git" or lowered.begins_with("res://.git/"):
        return false
    for ext in allowed_extensions:
        if lowered.ends_with(ext):
            return true
    return false

func _normalize_res_path(path: String) -> String:
    var trimmed = path.strip_edges().replace("\\", "/")
    if trimmed == "":
        return ""
    if not trimmed.begins_with("res://"):
        return ""

    var relative = trimmed.substr(6)
    var parts = relative.split("/", false)
    var normalized_parts: Array[String] = []
    for part in parts:
        var segment = str(part).strip_edges()
        if segment == "" or segment == ".":
            continue
        if segment == "..":
            return ""
        normalized_parts.append(segment)

    if normalized_parts.is_empty():
        return "res://"
    return "res://" + "/".join(normalized_parts)

func _build_scene_template(template_name: String) -> String:
    var root_type = "Node"
    var template = template_name.to_lower().strip_edges()
    if template == "2d" or template == "node2d" or template == "empty_2d":
        root_type = "Node2D"
    elif template == "3d" or template == "node3d" or template == "empty_3d":
        root_type = "Node3D"
    elif template == "ui" or template == "control":
        root_type = "Control"
    return "[gd_scene format=3]\n\n[node name=\"Root\" type=\"%s\"]\n" % root_type

func _resolve_scene_node(edited_root: Node, query: String) -> Node:
    if edited_root == null:
        return null

    var needle = query.strip_edges()
    if needle == "" or needle == ".":
        return edited_root
    if needle.find("..") != -1:
        return null
    if needle == str(edited_root.name):
        return edited_root

    var root_path = str(edited_root.get_path())
    if needle == root_path:
        return edited_root
    if needle.begins_with("/"):
        if not needle.begins_with(root_path + "/"):
            return null

    var resolved = edited_root.get_node_or_null(NodePath(needle))
    if resolved != null and _node_within_edited_scene(edited_root, resolved):
        return resolved

    if needle.begins_with(root_path + "/"):
        var relative = needle.substr(root_path.length())
        if relative.begins_with("/"):
            relative = relative.substr(1)
        if relative == "":
            return edited_root
        resolved = edited_root.get_node_or_null(NodePath(relative))
        if resolved != null and _node_within_edited_scene(edited_root, resolved):
            return resolved

    return null

func _node_has_property(node: Node, property_name: String) -> bool:
    for entry in node.get_property_list():
        if entry is Dictionary and str(entry.get("name", "")) == property_name:
            return true
    return false

func _node_within_edited_scene(edited_root: Node, node: Node) -> bool:
    if edited_root == null or node == null:
        return false
    var root_path = str(edited_root.get_path())
    var node_path = str(node.get_path())
    if node_path == root_path:
        return true
    return node_path.begins_with(root_path + "/")

func _setup_runtime_sync_timers() -> void:
    runtime_heartbeat_timer = Timer.new()
    runtime_heartbeat_timer.one_shot = false
    runtime_heartbeat_timer.wait_time = runtime_heartbeat_seconds
    runtime_heartbeat_timer.timeout.connect(Callable(self, "_on_runtime_heartbeat_timeout"))
    add_child(runtime_heartbeat_timer)
    runtime_heartbeat_timer.start()

    runtime_change_timer = Timer.new()
    runtime_change_timer.one_shot = false
    runtime_change_timer.wait_time = runtime_change_poll_seconds
    runtime_change_timer.timeout.connect(Callable(self, "_on_runtime_change_timeout"))
    add_child(runtime_change_timer)
    runtime_change_timer.start()

func _on_runtime_heartbeat_timeout() -> void:
    if mcp_interface == null:
        return

    if last_snapshot_fingerprint == "":
        _sync_runtime_snapshot(true)
        return

    if mcp_interface.can_ping_runtime_bridge():
        mcp_interface.ping_runtime_bridge()
        return

    # Backward compatibility: fallback to full sync if server does not expose ping tool.
    _sync_runtime_snapshot(true)

func _on_runtime_change_timeout() -> void:
    _sync_runtime_snapshot(false)

func _sync_runtime_snapshot(force: bool) -> void:
    if mcp_interface == null:
        return

    var snapshot = _build_runtime_snapshot()
    var fingerprint = JSON.stringify(snapshot).sha256_text()
    if force or fingerprint != last_snapshot_fingerprint:
        if mcp_interface.sync_runtime_snapshot(snapshot):
            last_snapshot_fingerprint = fingerprint

func _resolve_interval_setting(value: Variant, min_value: float, fallback_value: float) -> float:
    var interval := fallback_value
    if value is float or value is int:
        interval = float(value)
    if interval < min_value:
        interval = min_value
    return interval

func _apply_runtime_bridge_health(health: Dictionary) -> void:
    var freshness = health.get("freshness", {})
    if not (freshness is Dictionary):
        return

    var stale_after_ms = int(freshness.get("stale_after_ms", 0))
    var stale_grace_ms = int(freshness.get("stale_grace_ms", 0))
    if stale_after_ms <= 0:
        return

    var effective_window_seconds = float(stale_after_ms + max(0, stale_grace_ms)) / 1000.0
    var target_heartbeat = clampf(effective_window_seconds / 3.0, MIN_RUNTIME_HEARTBEAT_SECONDS, 10.0)
    var target_poll = clampf(target_heartbeat / 5.0, MIN_RUNTIME_CHANGE_POLL_SECONDS, 2.0)

    runtime_heartbeat_seconds = target_heartbeat
    runtime_change_poll_seconds = target_poll
    if runtime_heartbeat_timer != null:
        runtime_heartbeat_timer.wait_time = runtime_heartbeat_seconds
    if runtime_change_timer != null:
        runtime_change_timer.wait_time = runtime_change_poll_seconds

func _build_runtime_snapshot() -> Dictionary:
    var editor_interface = get_editor_interface()
    if editor_interface == null:
        return {
            "root_summary": {
                "project_path": ProjectSettings.globalize_path("res://")
            },
            "scene_tree": {},
            "node_details": {}
        }

    var edited_root = editor_interface.get_edited_scene_root()
    var root_summary = {
        "project_path": ProjectSettings.globalize_path("res://"),
        "active_scene": "",
        "active_script": _resolve_active_script_path(editor_interface),
        "root_path": "",
        "root_name": "",
        "root_type": "",
        "child_count": 0
    }
    var scene_tree: Dictionary = {}
    var node_details: Dictionary = {}

    if edited_root != null:
        root_summary["active_scene"] = _resolve_active_scene_path(edited_root)
        root_summary["root_path"] = str(edited_root.get_path())
        root_summary["root_name"] = str(edited_root.name)
        root_summary["root_type"] = str(edited_root.get_class())
        root_summary["child_count"] = int(edited_root.get_child_count())

        var node_counter := [0]
        scene_tree = _build_compact_tree(edited_root, 0, node_counter)

        var details_counter := [0]
        _collect_node_details(edited_root, node_details, 0, details_counter)

    return {
        "root_summary": root_summary,
        "scene_tree": scene_tree,
        "node_details": node_details
    }

func _resolve_active_scene_path(edited_root: Node) -> String:
    if edited_root == null:
        return ""
    var scene_path = str(edited_root.scene_file_path)
    if scene_path == "":
        return str(edited_root.get_path())
    return scene_path

func _resolve_active_script_path(editor_interface: EditorInterface) -> String:
    if editor_interface == null:
        return ""
    if not editor_interface.has_method("get_script_editor"):
        return ""

    var script_editor = editor_interface.get_script_editor()
    if script_editor == null:
        return ""
    if not script_editor.has_method("get_current_script"):
        return ""

    var current_script = script_editor.get_current_script()
    if current_script == null:
        return ""
    if current_script is Resource:
        return str(current_script.resource_path)
    return ""

func _build_compact_tree(node: Node, depth: int, counter: Array) -> Dictionary:
    if node == null:
        return {}
    if depth > MAX_RUNTIME_TREE_DEPTH:
        return {}
    if counter[0] >= MAX_RUNTIME_NODE_COUNT:
        return {}

    counter[0] += 1
    var tree = {
        "path": str(node.get_path()),
        "name": str(node.name),
        "type": str(node.get_class()),
        "child_count": int(node.get_child_count()),
        "children": []
    }

    if depth == MAX_RUNTIME_TREE_DEPTH:
        return tree

    var children: Array = []
    for child in node.get_children():
        if counter[0] >= MAX_RUNTIME_NODE_COUNT:
            break
        if child is Node:
            var compact_child = _build_compact_tree(child, depth + 1, counter)
            if not compact_child.is_empty():
                children.append(compact_child)
    tree["children"] = children
    return tree

func _collect_node_details(node: Node, details: Dictionary, depth: int, counter: Array) -> void:
    if node == null:
        return
    if depth > MAX_RUNTIME_TREE_DEPTH:
        return
    if counter[0] >= MAX_RUNTIME_NODE_COUNT:
        return

    counter[0] += 1

    var owner_path = ""
    if node.owner != null:
        owner_path = str(node.owner.get_path())

    var script_path = ""
    var script_ref = node.get_script()
    if script_ref != null and script_ref is Resource:
        script_path = str(script_ref.resource_path)

    var groups: Array[String] = []
    for group_name in node.get_groups():
        groups.append(str(group_name))

    var node_path = str(node.get_path())
    details[node_path] = {
        "path": node_path,
        "name": str(node.name),
        "type": str(node.get_class()),
        "owner": owner_path,
        "script": script_path,
        "groups": groups,
        "child_count": int(node.get_child_count())
    }

    if depth == MAX_RUNTIME_TREE_DEPTH:
        return

    for child in node.get_children():
        if counter[0] >= MAX_RUNTIME_NODE_COUNT:
            return
        if child is Node:
            _collect_node_details(child, details, depth + 1, counter)
