class_name VariantUtils
extends RefCounted

static func to_bool(value: Variant, default_value: bool = false) -> bool:
	if value is bool:
		return value
	return default_value
