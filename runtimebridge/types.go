package runtimebridge

import "time"

// RootSummary captures the editor-facing root context used by get-editor-state.
type RootSummary struct {
	ProjectPath  string `json:"project_path,omitempty"`
	ActiveScene  string `json:"active_scene,omitempty"`
	ActiveScript string `json:"active_script,omitempty"`
	RootPath     string `json:"root_path,omitempty"`
	RootName     string `json:"root_name,omitempty"`
	RootType     string `json:"root_type,omitempty"`
	ChildCount   int    `json:"child_count,omitempty"`
}

// CompactNode is a reduced scene tree representation for get-scene-tree.
type CompactNode struct {
	Path       string        `json:"path"`
	Name       string        `json:"name"`
	Type       string        `json:"type"`
	ChildCount int           `json:"child_count"`
	Children   []CompactNode `json:"children,omitempty"`
}

// NodeDetail is the whitelist payload for get-node-properties.
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
