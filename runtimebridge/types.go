package runtimebridge

import "time"

// RootSummary captures the editor-facing root context used by godot.editor.state.get.
type RootSummary struct {
	ProjectPath  string `json:"project_path,omitempty"`
	ActiveScene  string `json:"active_scene,omitempty"`
	ActiveScript string `json:"active_script,omitempty"`
	RootPath     string `json:"root_path,omitempty"`
	RootName     string `json:"root_name,omitempty"`
	RootType     string `json:"root_type,omitempty"`
	ChildCount   int    `json:"child_count,omitempty"`
}

// CompactNode is a reduced scene tree representation for runtime/editor snapshots.
type CompactNode struct {
	Path       string        `json:"path"`
	Name       string        `json:"name"`
	Type       string        `json:"type"`
	ChildCount int           `json:"child_count"`
	Children   []CompactNode `json:"children,omitempty"`
}

// NodeDetail is the whitelist payload for on-demand runtime node property reads.
type NodeDetail struct {
	Path       string   `json:"path"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Owner      string   `json:"owner,omitempty"`
	Script     string   `json:"script,omitempty"`
	Groups     []string `json:"groups,omitempty"`
	ChildCount int      `json:"child_count"`
}

// Snapshot is the runtime payload synced from the Godot plugin.
type Snapshot struct {
	RootSummary RootSummary           `json:"root_summary"`
	SceneTree   CompactNode           `json:"scene_tree"`
	NodeDetails map[string]NodeDetail `json:"node_details"`
}

// StoredSnapshot keeps snapshot metadata needed by stale checks.
type StoredSnapshot struct {
	SessionID string
	Snapshot  Snapshot
	UpdatedAt time.Time
}

// EditorSnapshot is the editor-facing snapshot payload synced from the editor plugin.
// It intentionally keeps the historical shape used by existing editor tools.
type EditorSnapshot = Snapshot

// StoredEditorSnapshot keeps editor snapshot metadata needed by stale checks.
type StoredEditorSnapshot = StoredSnapshot

// RuntimeSnapshot is the running game snapshot payload synced from runtime companion.
type RuntimeSnapshot struct {
	SessionID     string                `json:"session_id"`
	SnapshotID    string                `json:"snapshot_id"`
	Frame         int64                 `json:"frame"`
	UpdatedAt     string                `json:"updated_at"`
	RootScenePath string                `json:"root_scene_path"`
	RootNodeName  string                `json:"root_node_name"`
	NodeCount     int                   `json:"node_count"`
	Running       bool                  `json:"running"`
	Paused        bool                  `json:"paused"`
	SceneTree     CompactNode           `json:"scene_tree,omitempty"`
	NodeDetails   map[string]NodeDetail `json:"node_details,omitempty"`
}

// StoredRuntimeSnapshot keeps runtime snapshot metadata needed by stale checks.
type StoredRuntimeSnapshot struct {
	SessionID string
	Snapshot  RuntimeSnapshot
	UpdatedAt time.Time
}

// RuntimeLogEntry is one machine-readable runtime log line.
type RuntimeLogEntry struct {
	Sequence   int64  `json:"sequence"`
	Time       string `json:"time"`
	Level      string `json:"level"`
	Message    string `json:"message"`
	Source     string `json:"source,omitempty"`
	StackTrace string `json:"stack_trace,omitempty"`
}

// RuntimeLogAppendEntry is the append payload for runtime log ingestion.
type RuntimeLogAppendEntry struct {
	Time       string `json:"time,omitempty"`
	Level      string `json:"level,omitempty"`
	Message    string `json:"message"`
	Source     string `json:"source,omitempty"`
	StackTrace string `json:"stack_trace,omitempty"`
}

// GameSession keeps one running game session lifecycle state.
type GameSession struct {
	SessionID        string `json:"session_id"`
	EditorSessionID  string `json:"editor_session_id,omitempty"`
	RuntimeSessionID string `json:"runtime_session_id,omitempty"`
	LaunchToken      string `json:"launch_token,omitempty"`
	ScenePath        string `json:"scene_path,omitempty"`
	StartedAt        string `json:"started_at,omitempty"`
	StoppedAt        string `json:"stopped_at,omitempty"`
	Running          bool   `json:"running"`
	HasSnapshot      bool   `json:"has_snapshot"`
	LastSnapshotAt   string `json:"last_snapshot_at,omitempty"`
}
