class_name RuntimeSnapshotCollector
extends RefCounted

const MAX_EDITOR_TREE_DEPTH := 12
const MAX_EDITOR_NODE_COUNT := 2000
const DEFAULT_MAX_DEPTH := 6
const DEFAULT_MAX_NODES := 2000

func build_editor_snapshot(editor_interface: EditorInterface) -> Dictionary:
	if editor_interface == null:
		return {
			"source": "editor",
			"snapshot_kind": "editor_state",
			"root_summary": {
				"project_path": ProjectSettings.globalize_path("res://")
			},
			"scene_tree": {},
			"node_details": {}
		}

	var edited_root = EditorInterface.get_edited_scene_root()
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
		scene_tree = _build_editor_tree(edited_root, 0, node_counter)

		var details_counter := [0]
		_collect_node_details(edited_root, node_details, 0, details_counter)

	return {
		"source": "editor",
		"snapshot_kind": "editor_state",
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
	if not ClassDB.class_has_method("EditorInterface", "get_script_editor"):
		return ""

	var script_editor = EditorInterface.get_script_editor()
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

func _build_editor_tree(node: Node, depth: int, counter: Array) -> Dictionary:
	if node == null:
		return {}
	if depth > MAX_EDITOR_TREE_DEPTH:
		return {}
	if counter[0] >= MAX_EDITOR_NODE_COUNT:
		return {}

	counter[0] += 1
	var tree = {
		"path": str(node.get_path()),
		"name": str(node.name),
		"type": str(node.get_class()),
		"child_count": int(node.get_child_count()),
		"children": []
	}

	if depth == MAX_EDITOR_TREE_DEPTH:
		return tree

	var children: Array = []
	for child in node.get_children():
		if counter[0] >= MAX_EDITOR_NODE_COUNT:
			break
		if child is Node:
			var compact_child = _build_editor_tree(child, depth + 1, counter)
			if not compact_child.is_empty():
				children.append(compact_child)
	tree["children"] = children
	return tree

func _collect_node_details(node: Node, details: Dictionary, depth: int, counter: Array) -> void:
	if node == null:
		return
	if depth > MAX_EDITOR_TREE_DEPTH:
		return
	if counter[0] >= MAX_EDITOR_NODE_COUNT:
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

	if depth == MAX_EDITOR_TREE_DEPTH:
		return

	for child in node.get_children():
		if counter[0] >= MAX_EDITOR_NODE_COUNT:
			return
		if child is Node:
			_collect_node_details(child, details, depth + 1, counter)

func collect_runtime_snapshot(game_session_id: String, snapshot_id: String, max_depth: int = DEFAULT_MAX_DEPTH, max_nodes: int = DEFAULT_MAX_NODES) -> Dictionary:
	var root = _resolve_root_node()
	var now = _now_rfc3339()
	var frame = int(Engine.get_process_frames())

	if root == null:
		return {
			"source": "runtime",
			"session_id": game_session_id,
			"snapshot_id": snapshot_id,
			"frame": frame,
			"updated_at": now,
			"root_scene_path": "",
			"root_node_name": "",
			"node_count": 0,
			"running": true,
			"paused": get_tree_paused(),
			"scene_tree": {}
		}

	var node_counter := [0]
	var compact_tree = _build_runtime_tree(root, 0, max_depth, max_nodes, node_counter)
	var root_scene_path = _resolve_scene_path(root)

	return {
		"source": "runtime",
		"session_id": game_session_id,
		"snapshot_id": snapshot_id,
		"frame": frame,
		"updated_at": now,
		"root_scene_path": root_scene_path,
		"root_node_name": str(root.name),
		"node_count": int(node_counter[0]),
		"running": true,
		"paused": get_tree_paused(),
		"scene_tree": compact_tree
	}

func resolve_node(query: String) -> Node:
	var trimmed = query.strip_edges()
	if trimmed == "":
		return null

	var tree := Engine.get_main_loop() as SceneTree
	if tree == null:
		return null
	if trimmed.begins_with("/root"):
		return tree.root.get_node_or_null(trimmed)

	var current = tree.current_scene
	if current != null:
		if str(current.get_path()) == trimmed:
			return current
		var in_scene = current.get_node_or_null(trimmed)
		if in_scene != null:
			return in_scene

	return tree.root.get_node_or_null(trimmed)

func _resolve_root_node() -> Node:
	var tree := Engine.get_main_loop() as SceneTree
	if tree == null:
		return null
	if tree.current_scene != null:
		return tree.current_scene
	if tree.root != null and tree.root.get_child_count() > 0:
		for child in tree.root.get_children():
			if child is Node and not (child as Node).name.begins_with("@"):
				return child
	return tree.root

func _resolve_scene_path(root: Node) -> String:
	if root == null:
		return ""
	if root.has_method("get_scene_file_path"):
		var scene_path = str(root.call("get_scene_file_path"))
		if scene_path != "":
			return scene_path
	var fallback_scene_path = root.get("scene_file_path")
	if fallback_scene_path != null:
		var as_text = str(fallback_scene_path)
		if as_text != "":
			return as_text
	return ""

func _build_runtime_tree(node: Node, depth: int, max_depth: int, max_nodes: int, counter: Array) -> Dictionary:
	if node == null:
		return {}
	if depth > max_depth:
		return {}
	if counter[0] >= max_nodes:
		return {}

	counter[0] += 1

	var script_path := ""
	var script_ref = node.get_script()
	if script_ref != null and script_ref is Resource:
		script_path = str(script_ref.resource_path)

	var item: Dictionary = {
		"path": str(node.get_path()),
		"name": str(node.name),
		"type": str(node.get_class()),
		"script_path": script_path,
		"child_count": int(node.get_child_count())
	}

	if node is CanvasItem:
		item["visible"] = (node as CanvasItem).visible
	item["process_mode"] = int(node.process_mode)
	item["paused"] = not node.can_process()

	if depth >= max_depth:
		item["children"] = []
		return item

	var children: Array = []
	for child in node.get_children():
		if counter[0] >= max_nodes:
			break
		if child is Node:
			var child_item = _build_runtime_tree(child, depth + 1, max_depth, max_nodes, counter)
			if not child_item.is_empty():
				children.append(child_item)
	item["children"] = children
	return item

func get_tree_paused() -> bool:
	var tree := Engine.get_main_loop() as SceneTree
	if tree == null:
		return false
	return tree.paused

func _now_rfc3339() -> String:
	return "%sZ" % Time.get_datetime_string_from_system(true)
