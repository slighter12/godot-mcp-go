package project

import (
	"encoding/json"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/tools/types"
)

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
	result := map[string]any{"active_scene": "", "active_script": ""}
	return json.Marshal(result)
}

type RunProjectTool struct{}

func (t *RunProjectTool) Name() string        { return "run-project" }
func (t *RunProjectTool) Description() string { return "Runs the project" }
func (t *RunProjectTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "Run Project"}
}
func (t *RunProjectTool) Execute(args json.RawMessage) ([]byte, error) {
	result := map[string]any{"success": true}
	return json.Marshal(result)
}

type StopProjectTool struct{}

func (t *StopProjectTool) Name() string        { return "stop-project" }
func (t *StopProjectTool) Description() string { return "Stops the running project" }
func (t *StopProjectTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "Stop Project"}
}
func (t *StopProjectTool) Execute(args json.RawMessage) ([]byte, error) {
	result := map[string]any{"success": true}
	return json.Marshal(result)
}

func GetAllTools() []types.Tool {
	return []types.Tool{
		&GetProjectSettingsTool{},
		&ListProjectResourcesTool{},
		&GetEditorStateTool{},
		&RunProjectTool{},
		&StopProjectTool{},
	}
}
