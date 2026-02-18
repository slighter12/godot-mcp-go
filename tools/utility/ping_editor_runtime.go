package utility

import (
	"encoding/json"
	"time"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

type pingEditorRuntimeContext struct {
	SessionID          string `json:"session_id"`
	SessionInitialized bool   `json:"session_initialized"`
}

type pingEditorRuntimePayload struct {
	Context pingEditorRuntimeContext `json:"_mcp"`
}

// PingEditorRuntimeTool refreshes runtime snapshot freshness without replacing snapshot content.
type PingEditorRuntimeTool struct{}

func NewPingEditorRuntimeTool() *PingEditorRuntimeTool {
	return &PingEditorRuntimeTool{}
}

func (t *PingEditorRuntimeTool) Name() string { return "ping-editor-runtime" }

func (t *PingEditorRuntimeTool) Description() string {
	return "Refreshes Godot editor runtime snapshot freshness (internal bridge tool)"
}

func (t *PingEditorRuntimeTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
		Title:      "Ping Editor Runtime",
	}
}

func (t *PingEditorRuntimeTool) Execute(args json.RawMessage) ([]byte, error) {
	var payload pingEditorRuntimePayload
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, err
	}

	if payload.Context.SessionID == "" || !payload.Context.SessionInitialized {
		return nil, tooltypes.NewNotAvailableError("Runtime ping requires an initialized MCP HTTP session", map[string]any{
			"feature": "runtime_bridge",
			"reason":  "session_not_initialized",
			"tool":    t.Name(),
		})
	}

	now := time.Now().UTC()
	if touched := runtimebridge.DefaultStore().Touch(payload.Context.SessionID, now); !touched {
		return nil, tooltypes.NewNotAvailableError("Runtime ping requires an existing runtime snapshot", map[string]any{
			"feature": "runtime_bridge",
			"reason":  "runtime_snapshot_missing",
			"tool":    t.Name(),
		})
	}

	result := map[string]any{
		"pong":       true,
		"session_id": payload.Context.SessionID,
		"updated_at": now.Format(time.RFC3339Nano),
	}
	return json.Marshal(result)
}
