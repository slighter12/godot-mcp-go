package project

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

const projectCommandTimeout = 8 * time.Second

type GetProjectSettingsTool struct{}

func (t *GetProjectSettingsTool) Name() string        { return "get-project-settings" }
func (t *GetProjectSettingsTool) Description() string { return "Gets project settings" }
func (t *GetProjectSettingsTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "Get Project Settings"}
}
func (t *GetProjectSettingsTool) Execute(args json.RawMessage) ([]byte, error) {
	result := map[string]any{}
	return json.Marshal(result)
}

type ListProjectResourcesTool struct{}

func (t *ListProjectResourcesTool) Name() string        { return "list-project-resources" }
func (t *ListProjectResourcesTool) Description() string { return "Lists all resources in the project" }
func (t *ListProjectResourcesTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "List Project Resources"}
}
func (t *ListProjectResourcesTool) Execute(args json.RawMessage) ([]byte, error) {
	result := []any{}
	return json.Marshal(result)
}

type GetEditorStateTool struct{}

func (t *GetEditorStateTool) Name() string        { return "get-editor-state" }
func (t *GetEditorStateTool) Description() string { return "Gets the current editor state" }
func (t *GetEditorStateTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "Get Editor State"}
}
func (t *GetEditorStateTool) Execute(args json.RawMessage) ([]byte, error) {
	var arguments map[string]any
	if err := json.Unmarshal(args, &arguments); err != nil {
		return nil, err
	}

	ctx := tooltypes.ExtractMCPContext(arguments)
	if strings.TrimSpace(ctx.SessionID) == "" || !ctx.SessionInitialized {
		return nil, tooltypes.NewNotAvailableError("Editor state requires an initialized MCP HTTP session", map[string]any{
			"feature": "runtime_bridge",
			"reason":  "session_not_initialized",
			"tool":    t.Name(),
		})
	}

	stored, ok, reason := runtimebridge.DefaultStore().FreshForSession(ctx.SessionID, time.Now().UTC())
	if !ok {
		return nil, tooltypes.NewNotAvailableError("Editor state is unavailable until runtime sync is healthy", map[string]any{
			"feature": "runtime_bridge",
			"reason":  reason,
			"tool":    t.Name(),
		})
	}
	result := map[string]any{
		"active_scene":  stored.Snapshot.RootSummary.ActiveScene,
		"active_script": stored.Snapshot.RootSummary.ActiveScript,
		"root_summary":  stored.Snapshot.RootSummary,
		"session_id":    stored.SessionID,
		"updated_at":    stored.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	return json.Marshal(result)
}

type RunProjectTool struct{}

func (t *RunProjectTool) Name() string        { return "run-project" }
func (t *RunProjectTool) Description() string { return "Runs the project" }
func (t *RunProjectTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "Run Project"}
}
func (t *RunProjectTool) Execute(args json.RawMessage) ([]byte, error) {
	return dispatchProjectRuntimeCommand(args, t.Name())
}

type StopProjectTool struct{}

func (t *StopProjectTool) Name() string        { return "stop-project" }
func (t *StopProjectTool) Description() string { return "Stops the running project" }
func (t *StopProjectTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "Stop Project"}
}
func (t *StopProjectTool) Execute(args json.RawMessage) ([]byte, error) {
	return dispatchProjectRuntimeCommand(args, t.Name())
}

func GetAllTools() []tooltypes.Tool {
	return []tooltypes.Tool{
		&GetProjectSettingsTool{},
		&ListProjectResourcesTool{},
		&GetEditorStateTool{},
		&RunProjectTool{},
		&StopProjectTool{},
	}
}

func dispatchProjectRuntimeCommand(rawArgs json.RawMessage, commandName string) ([]byte, error) {
	var arguments map[string]any
	if err := json.Unmarshal(rawArgs, &arguments); err != nil {
		return nil, err
	}

	ctx := tooltypes.ExtractMCPContext(arguments)
	if strings.TrimSpace(ctx.SessionID) == "" || !ctx.SessionInitialized {
		return nil, tooltypes.NewNotAvailableError("Project execution requires an initialized MCP HTTP session", map[string]any{
			"feature": "runtime_bridge",
			"reason":  "session_not_initialized",
			"tool":    commandName,
		})
	}

	commandArgs := tooltypes.StripMCPContext(arguments)
	ack, ok, reason := runtimebridge.DefaultCommandBroker().DispatchAndWait(ctx.SessionID, commandName, commandArgs, projectCommandTimeout)
	if !ok {
		return nil, tooltypes.NewNotAvailableError("Project execution bridge is unavailable", map[string]any{
			"feature": "runtime_bridge",
			"reason":  reason,
			"tool":    commandName,
		})
	}

	result := map[string]any{
		"success":         ack.Success,
		"command_id":      ack.CommandID,
		"result":          ack.Result,
		"error":           ack.Error,
		"acknowledged_at": ack.AckedAt.UTC().Format(time.RFC3339Nano),
	}
	return json.Marshal(result)
}
