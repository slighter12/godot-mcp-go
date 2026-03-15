class_name RuntimeSnapshotCollector
extends RefCounted

const MAX_RUNTIME_TREE_DEPTH := 12
const MAX_RUNTIME_NODE_COUNT := 2000

func build_snapshot(editor_interface: EditorInterface) -> Dictionary:
	if editor_interface == null:
		return {
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
