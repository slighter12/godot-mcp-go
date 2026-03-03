class_name ToolCatalog
extends RefCounted

var _tools: Dictionary = {}

func replace_all(tools: Dictionary) -> void:
	_tools = tools.duplicate(true)

func has_tool(name: String) -> bool:
	return _tools.has(name)

func all_tools() -> Dictionary:
	return _tools.duplicate(true)
