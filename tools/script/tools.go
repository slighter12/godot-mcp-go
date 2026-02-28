package script

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/tools/types"
)

var supportedScriptExtensions = []string{".gd", ".rs"}

const scriptCommandTimeout = 8 * time.Second

type ListProjectScriptsTool struct{}

func (t *ListProjectScriptsTool) Name() string        { return "godot-script-list" }
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

func (t *ReadScriptTool) Name() string        { return "godot-script-read" }
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

func (t *ModifyScriptTool) Name() string        { return "godot-script-modify" }
func (t *ModifyScriptTool) Description() string { return "Modifies a script" }
func (t *ModifyScriptTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"path": map[string]any{"type": "string", "description": "Script path"}, "content": map[string]any{"type": "string", "description": "New script content"}}, Required: []string{"path", "content"}, Title: "Modify Script"}
}
func (t *ModifyScriptTool) Execute(args json.RawMessage) ([]byte, error) {
	return dispatchScriptRuntimeCommand(args, t.Name(), validateModifyScriptArguments)
}

type CreateScriptTool struct{}

func (t *CreateScriptTool) Name() string        { return "godot-script-create" }
func (t *CreateScriptTool) Description() string { return "Creates a new script" }
func (t *CreateScriptTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"path":    map[string]any{"type": "string", "description": "Script path"},
			"content": map[string]any{"type": "string", "description": "Script content"},
			"replace": map[string]any{"type": "boolean", "description": "Allow overwriting existing script when true"},
		},
		Required: []string{"path", "content"},
		Title:    "Create Script",
	}
}
func (t *CreateScriptTool) Execute(args json.RawMessage) ([]byte, error) {
	return dispatchScriptRuntimeCommand(args, t.Name(), validateCreateScriptArguments)
}

type AnalyzeScriptTool struct{}

func (t *AnalyzeScriptTool) Name() string        { return "godot-script-analyze" }
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

func dispatchScriptRuntimeCommand(rawArgs json.RawMessage, commandName string, validate func(map[string]any, string) (map[string]any, error)) ([]byte, error) {
	return types.DispatchRuntimeCommand(types.RuntimeCommandDispatchOptions{
		RawArgs:                  rawArgs,
		CommandName:              commandName,
		Timeout:                  scriptCommandTimeout,
		SessionRequiredMessage:   "Script commands require an initialized MCP HTTP session",
		BridgeUnavailableMessage: "Script runtime bridge is unavailable",
		InvalidJSONError: func(err error) error {
			return newScriptInvalidParamsError("Invalid JSON arguments", commandName, "invalid_json", map[string]any{"error": err.Error()})
		},
		Validate: validate,
	})
}

func validateModifyScriptArguments(arguments map[string]any, toolName string) (map[string]any, error) {
	path, err := requiredScriptString(arguments, "path", toolName, "missing_path")
	if err != nil {
		return nil, err
	}
	contentValue, exists := arguments["content"]
	if !exists {
		return nil, newScriptInvalidParamsError("content is required", toolName, "missing_content", nil)
	}
	content, ok := contentValue.(string)
	if !ok {
		return nil, newScriptInvalidParamsError("content must be a string", toolName, "invalid_content_type", nil)
	}
	return map[string]any{
		"path":    path,
		"content": content,
	}, nil
}

func validateCreateScriptArguments(arguments map[string]any, toolName string) (map[string]any, error) {
	out, err := validateModifyScriptArguments(arguments, toolName)
	if err != nil {
		return nil, err
	}
	replace := false
	if rawReplace, exists := arguments["replace"]; exists {
		asBool, ok := rawReplace.(bool)
		if !ok {
			return nil, newScriptInvalidParamsError("replace must be a boolean", toolName, "invalid_replace_type", nil)
		}
		replace = asBool
	}
	out["replace"] = replace
	return out, nil
}

func requiredScriptString(arguments map[string]any, key, toolName, reason string) (string, error) {
	value, exists := arguments[key]
	if !exists {
		return "", newScriptInvalidParamsError(key+" is required", toolName, reason, nil)
	}
	asString, ok := value.(string)
	if !ok {
		return "", newScriptInvalidParamsError(key+" must be a string", toolName, "invalid_"+key+"_type", nil)
	}
	asString = strings.TrimSpace(asString)
	if asString == "" {
		return "", newScriptInvalidParamsError(key+" must not be empty", toolName, reason, nil)
	}
	return asString, nil
}

func newScriptInvalidParamsError(message, toolName, reason string, extra map[string]any) error {
	data := map[string]any{
		"feature": "runtime_bridge",
		"tool":    toolName,
	}
	if reason != "" {
		data["reason"] = reason
	}
	for key, value := range extra {
		data[key] = value
	}
	return types.NewSemanticError(types.SemanticKindInvalidParams, message, data)
}
