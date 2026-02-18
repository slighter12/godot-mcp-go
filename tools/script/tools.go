package script

import (
	"encoding/json"
	"strings"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/tools/types"
)

var supportedScriptExtensions = []string{".gd", ".rs"}

type ListProjectScriptsTool struct{}

func (t *ListProjectScriptsTool) Name() string        { return "list-project-scripts" }
func (t *ListProjectScriptsTool) Description() string { return "Lists all scripts in the project" }
func (t *ListProjectScriptsTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "List Project Scripts"}
}
func (t *ListProjectScriptsTool) Execute(args json.RawMessage) ([]byte, error) {
	scripts, scriptPaths, err := listProjectScripts()
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"scripts":      scripts,
		"script_paths": scriptPaths,
	}
	return json.Marshal(result)
}

type ReadScriptTool struct{}

func (t *ReadScriptTool) Name() string        { return "read-script" }
func (t *ReadScriptTool) Description() string { return "Reads a specific script" }
func (t *ReadScriptTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"path": map[string]any{"type": "string", "description": "Script path"}}, Required: []string{"path"}, Title: "Read Script"}
}
func (t *ReadScriptTool) Execute(args json.RawMessage) ([]byte, error) {
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, err
	}
	data, resPath, err := types.ReadProjectFile(payload.Path, supportedScriptExtensions)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"path":    resPath,
		"content": string(data),
		"metadata": map[string]any{
			"size_bytes": len(data),
			"line_count": countLines(data),
			"language":   strings.TrimPrefix(strings.ToLower(filepathExt(resPath)), "."),
		},
	}
	return json.Marshal(result)
}

type ModifyScriptTool struct{}

func (t *ModifyScriptTool) Name() string        { return "modify-script" }
func (t *ModifyScriptTool) Description() string { return "Modifies a script" }
func (t *ModifyScriptTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"path": map[string]any{"type": "string", "description": "Script path"}, "content": map[string]any{"type": "string", "description": "New script content"}}, Required: []string{"path", "content"}, Title: "Modify Script"}
}
func (t *ModifyScriptTool) Execute(args json.RawMessage) ([]byte, error) {
	return nil, types.NewNotAvailableError("Script writes are not available yet", map[string]any{
		"feature": "godot_runtime_write",
		"tool":    t.Name(),
	})
}

type CreateScriptTool struct{}

func (t *CreateScriptTool) Name() string        { return "create-script" }
func (t *CreateScriptTool) Description() string { return "Creates a new script" }
func (t *CreateScriptTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"path": map[string]any{"type": "string", "description": "Script path"}, "content": map[string]any{"type": "string", "description": "Script content"}}, Required: []string{"path", "content"}, Title: "Create Script"}
}
func (t *CreateScriptTool) Execute(args json.RawMessage) ([]byte, error) {
	return nil, types.NewNotAvailableError("Script writes are not available yet", map[string]any{
		"feature": "godot_runtime_write",
		"tool":    t.Name(),
	})
}

type AnalyzeScriptTool struct{}

func (t *AnalyzeScriptTool) Name() string        { return "analyze-script" }
func (t *AnalyzeScriptTool) Description() string { return "Analyzes a script" }
func (t *AnalyzeScriptTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"path": map[string]any{"type": "string", "description": "Script path"}}, Required: []string{"path"}, Title: "Analyze Script"}
}
func (t *AnalyzeScriptTool) Execute(args json.RawMessage) ([]byte, error) {
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, err
	}
	data, resPath, err := types.ReadProjectFile(payload.Path, supportedScriptExtensions)
	if err != nil {
		return nil, err
	}

	content := string(data)
	result := map[string]any{
		"path": resPath,
		"analysis": map[string]any{
			"line_count":      countLines(data),
			"non_empty_lines": countNonEmptyLines(content),
			"function_count":  countFunctionSignatures(content, filepathExt(resPath)),
		},
	}
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
