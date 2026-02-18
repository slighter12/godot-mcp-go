package utility

import (
	"encoding/json"
	"time"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

type syncEditorRuntimeContext struct {
	SessionID          string `json:"session_id"`
	SessionInitialized bool   `json:"session_initialized"`
}

type syncEditorRuntimePayload struct {
	Snapshot runtimebridge.Snapshot   `json:"snapshot"`
	Context  syncEditorRuntimeContext `json:"_mcp"`
}

// SyncEditorRuntimeTool accepts runtime snapshots from the Godot editor plugin.
type SyncEditorRuntimeTool struct{}

func NewSyncEditorRuntimeTool() *SyncEditorRuntimeTool {
	return &SyncEditorRuntimeTool{}
}

func (t *SyncEditorRuntimeTool) Name() string { return "sync-editor-runtime" }

func (t *SyncEditorRuntimeTool) Description() string {
	return "Synchronizes Godot editor runtime snapshot (internal bridge tool)"
}

func (t *SyncEditorRuntimeTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"snapshot": map[string]any{
				"type":        "object",
				"description": "Runtime snapshot payload from Godot plugin",
			},
		},
		Required: []string{"snapshot"},
		Title:    "Sync Editor Runtime",
	}
}

func (t *SyncEditorRuntimeTool) Execute(args json.RawMessage) ([]byte, error) {
	var payload syncEditorRuntimePayload
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, err
	}

	if payload.Context.SessionID == "" || !payload.Context.SessionInitialized {
		return nil, tooltypes.NewNotAvailableError("Runtime sync requires an initialized MCP HTTP session", map[string]any{
			"feature": "runtime_bridge",
			"reason":  "session_not_initialized",
			"tool":    t.Name(),
		})
	}

	now := time.Now().UTC()
	runtimebridge.DefaultStore().Upsert(payload.Context.SessionID, payload.Snapshot, now)
	result := map[string]any{
		"synced":     true,
		"session_id": payload.Context.SessionID,
		"updated_at": now.Format(time.RFC3339Nano),
	}
	return json.Marshal(result)
}
