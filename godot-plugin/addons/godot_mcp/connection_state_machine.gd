extends RefCounted

var _connected: bool = false
var _last_snapshot_fingerprint: String = ""

func mark_connected() -> void:
	_connected = true
	_last_snapshot_fingerprint = ""

func mark_disconnected() -> void:
	_connected = false

func is_connection_active() -> bool:
	return _connected

func should_sync(snapshot: Dictionary, force: bool) -> bool:
	if force:
		return true
	if _last_snapshot_fingerprint == "":
		return true
	return JSON.stringify(snapshot).sha256_text() != _last_snapshot_fingerprint

func mark_snapshot_synced(snapshot: Dictionary) -> void:
	_last_snapshot_fingerprint = JSON.stringify(snapshot).sha256_text()

func clear_snapshot_fingerprint() -> void:
	_last_snapshot_fingerprint = ""

func has_snapshot_fingerprint() -> bool:
	return _last_snapshot_fingerprint != ""
