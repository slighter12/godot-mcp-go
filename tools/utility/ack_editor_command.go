package utility

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

type ackEditorCommandPayload struct {
	CommandID string                   `json:"command_id"`
	Success   *bool                    `json:"success,omitempty"`
	Result    map[string]any           `json:"result,omitempty"`
	Error     string                   `json:"error,omitempty"`
	Context   syncEditorRuntimeContext `json:"_mcp"`
}

// AckEditorCommandTool accepts plugin command execution acknowledgements.
type AckEditorCommandTool struct{}

func NewAckEditorCommandTool() *AckEditorCommandTool {
	return &AckEditorCommandTool{}
}

func (t *AckEditorCommandTool) Name() string { return "ack-editor-command" }

func (t *AckEditorCommandTool) Description() string {
	return "Acknowledges completion of an internal Godot runtime command"
}

func (t *AckEditorCommandTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"command_id": map[string]any{"type": "string"},
			"success":    map[string]any{"type": "boolean"},
			"result":     map[string]any{"type": "object"},
			"error":      map[string]any{"type": "string"},
		},
		Required: []string{"command_id"},
		Title:    "Ack Editor Command",
	}
}

func (t *AckEditorCommandTool) Execute(args json.RawMessage) ([]byte, error) {
	var payload ackEditorCommandPayload
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, err
	}

	commandID := strings.TrimSpace(payload.CommandID)
	if commandID == "" {
		return nil, tooltypes.NewNotAvailableError("Command acknowledgement requires command_id", map[string]any{
			"feature": "runtime_bridge",
			"reason":  "command_id_missing",
			"tool":    t.Name(),
		})
	}
	if payload.Context.SessionID == "" || !payload.Context.SessionInitialized {
		return nil, tooltypes.NewNotAvailableError("Command acknowledgement requires an initialized MCP HTTP session", map[string]any{
			"feature": "runtime_bridge",
			"reason":  "session_not_initialized",
			"tool":    t.Name(),
		})
	}

	success := true
	if payload.Success != nil {
		success = *payload.Success
	}
	ack := runtimebridge.CommandAck{
		CommandID: commandID,
		Success:   success,
		Result:    payload.Result,
		Error:     strings.TrimSpace(payload.Error),
		AckedAt:   time.Now().UTC(),
	}
	acknowledged := runtimebridge.DefaultCommandBroker().Ack(payload.Context.SessionID, ack)
	if !acknowledged {
		return nil, tooltypes.NewNotAvailableError("Command acknowledgement rejected", map[string]any{
			"feature":    "runtime_bridge",
			"reason":     "unknown_or_expired_command",
			"command_id": commandID,
			"tool":       t.Name(),
		})
	}

	result := map[string]any{
		"acknowledged": acknowledged,
		"command_id":   commandID,
	}
	return json.Marshal(result)
}
