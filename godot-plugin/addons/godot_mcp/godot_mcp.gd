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

    # Create MCP client node (script path keeps legacy name for compatibility).
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

    if command_name == "run-project":
        if not editor_interface.has_method("play_main_scene"):
            mcp_interface.ack_runtime_command(command_id, false, {}, "play_main_scene is not available")
            return
        editor_interface.play_main_scene()
        _sync_runtime_snapshot(true)
        mcp_interface.ack_runtime_command(command_id, true, {"running": true, "command": command_name}, "")
        return

    if command_name == "stop-project":
        if not editor_interface.has_method("stop_playing_scene"):
            mcp_interface.ack_runtime_command(command_id, false, {}, "stop_playing_scene is not available")
            return
        editor_interface.stop_playing_scene()
        _sync_runtime_snapshot(true)
        mcp_interface.ack_runtime_command(command_id, true, {"running": false, "command": command_name}, "")
        return

    mcp_interface.ack_runtime_command(command_id, false, {}, "Unsupported runtime command: " + command_name)

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
