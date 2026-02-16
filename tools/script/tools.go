package script

import (
	"encoding/json"
	"errors"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/tools/types"
)

type ListProjectScriptsTool struct{}

func (t *ListProjectScriptsTool) Name() string        { return "list-project-scripts" }
func (t *ListProjectScriptsTool) Description() string { return "Lists all scripts in the project" }
func (t *ListProjectScriptsTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "List Project Scripts"}
}
func (t *ListProjectScriptsTool) Execute(args json.RawMessage) ([]byte, error) {
	result := []any{}
	return json.Marshal(result)
}

type ReadScriptTool struct{}

func (t *ReadScriptTool) Name() string        { return "read-script" }
func (t *ReadScriptTool) Description() string { return "Reads a specific script" }
func (t *ReadScriptTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"path": map[string]any{"type": "string", "description": "Script path"}}, Required: []string{"path"}, Title: "Read Script"}
}
func (t *ReadScriptTool) Execute(args json.RawMessage) ([]byte, error) {
	var argsMap map[string]any
	if err := json.Unmarshal(args, &argsMap); err != nil {
		return nil, err
	}
	path, ok := argsMap["path"].(string)
	if !ok {
		return nil, errors.New("invalid script path")
	}
	result := map[string]any{"path": path, "content": ""}
	return json.Marshal(result)
}

type ModifyScriptTool struct{}

func (t *ModifyScriptTool) Name() string        { return "modify-script" }
func (t *ModifyScriptTool) Description() string { return "Modifies a script" }
func (t *ModifyScriptTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"path": map[string]any{"type": "string", "description": "Script path"}, "content": map[string]any{"type": "string", "description": "New script content"}}, Required: []string{"path", "content"}, Title: "Modify Script"}
}
func (t *ModifyScriptTool) Execute(args json.RawMessage) ([]byte, error) {
	var argsMap map[string]any
	if err := json.Unmarshal(args, &argsMap); err != nil {
		return nil, err
	}
	path, ok1 := argsMap["path"].(string)
	_, ok2 := argsMap["content"].(string)
	if !ok1 || !ok2 {
		return nil, errors.New("invalid script modification parameters")
	}
	result := map[string]any{"success": true, "path": path}
	return json.Marshal(result)
}

type CreateScriptTool struct{}

func (t *CreateScriptTool) Name() string        { return "create-script" }
func (t *CreateScriptTool) Description() string { return "Creates a new script" }
func (t *CreateScriptTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"path": map[string]any{"type": "string", "description": "Script path"}, "content": map[string]any{"type": "string", "description": "Script content"}}, Required: []string{"path", "content"}, Title: "Create Script"}
}
func (t *CreateScriptTool) Execute(args json.RawMessage) ([]byte, error) {
	var argsMap map[string]any
	if err := json.Unmarshal(args, &argsMap); err != nil {
		return nil, err
	}
	path, ok1 := argsMap["path"].(string)
	_, ok2 := argsMap["content"].(string)
	if !ok1 || !ok2 {
		return nil, errors.New("invalid script creation parameters")
	}
	result := map[string]any{"success": true, "path": path}
	return json.Marshal(result)
}

type AnalyzeScriptTool struct{}

func (t *AnalyzeScriptTool) Name() string        { return "analyze-script" }
func (t *AnalyzeScriptTool) Description() string { return "Analyzes a script" }
func (t *AnalyzeScriptTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"path": map[string]any{"type": "string", "description": "Script path"}}, Required: []string{"path"}, Title: "Analyze Script"}
}
func (t *AnalyzeScriptTool) Execute(args json.RawMessage) ([]byte, error) {
	var argsMap map[string]any
	if err := json.Unmarshal(args, &argsMap); err != nil {
		return nil, err
	}
	path, ok := argsMap["path"].(string)
	if !ok {
		return nil, errors.New("invalid script path")
	}
	result := map[string]any{"path": path, "analysis": map[string]any{}}
	return json.Marshal(result)
}

func GetAllTools() []types.Tool {
	return []types.Tool{
		&ListProjectScriptsTool{},
		&ReadScriptTool{},
		&ModifyScriptTool{},
		&CreateScriptTool{},
		&AnalyzeScriptTool{},
	}
}
