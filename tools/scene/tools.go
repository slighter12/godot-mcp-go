package scene

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/tools/types"
)

var sceneNodePattern = regexp.MustCompile(`(name|type|parent)="([^"]*)"`)

const sceneCommandTimeout = 8 * time.Second

type ListProjectScenesTool struct{}

func (t *ListProjectScenesTool) Name() string        { return "godot.scene.list" }
func (t *ListProjectScenesTool) Description() string { return "Lists all scenes in the project" }
func (t *ListProjectScenesTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "List Project Scenes"}
}
func (t *ListProjectScenesTool) Execute(args json.RawMessage) ([]byte, error) {
	projectRoot := types.ResolveProjectRootFromEnvOrCWD()
	var names []string
	var resPaths []string

	err := filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || strings.ToLower(filepath.Ext(path)) != ".tscn" {
			return nil
		}
		relPath, relErr := filepath.Rel(projectRoot, path)
		if relErr != nil {
			return relErr
		}
		names = append(names, strings.TrimSuffix(filepath.Base(path), ".tscn"))
		resPaths = append(resPaths, "res://"+filepath.ToSlash(relPath))
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(names)
	sort.Strings(resPaths)
	result := map[string]any{
		"scenes":      names,
		"scene_paths": resPaths,
	}
	return json.Marshal(result)
}

type ReadSceneTool struct{}

func (t *ReadSceneTool) Name() string        { return "godot.scene.read" }
func (t *ReadSceneTool) Description() string { return "Reads a specific scene" }
func (t *ReadSceneTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"path": map[string]any{"type": "string", "description": "Scene path"}}, Required: []string{"path"}, Title: "Read Scene"}
}
func (t *ReadSceneTool) Execute(args json.RawMessage) ([]byte, error) {
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, err
	}

	data, resPath, err := types.ReadProjectFile(payload.Path, []string{".tscn"})
	if err != nil {
		return nil, err
	}

	nodes := parseSceneNodes(string(data))
	result := map[string]any{
		"path":    resPath,
		"nodes":   nodes,
		"content": string(data),
		"metadata": map[string]any{
			"size_bytes": len(data),
			"line_count": countLines(data),
			"node_count": len(nodes),
		},
	}
	return json.Marshal(result)
}

type CreateSceneTool struct{}

func (t *CreateSceneTool) Name() string        { return "godot.scene.create" }
func (t *CreateSceneTool) Description() string { return "Creates a new scene" }
func (t *CreateSceneTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"path":     map[string]any{"type": "string", "description": "Scene path (res://*.tscn)"},
			"content":  map[string]any{"type": "string", "description": "Optional scene content to write"},
			"template": map[string]any{"type": "string", "description": "Optional template hint when content is omitted"},
		},
		Required: []string{"path"},
		Title:    "Create Scene",
	}
}
func (t *CreateSceneTool) Execute(args json.RawMessage) ([]byte, error) {
	return dispatchSceneRuntimeCommand(args, t.Name(), validateCreateSceneArguments)
}

type SaveSceneTool struct{}

func (t *SaveSceneTool) Name() string        { return "godot.scene.save" }
func (t *SaveSceneTool) Description() string { return "Saves the current scene" }
func (t *SaveSceneTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "Save Scene"}
}
func (t *SaveSceneTool) Execute(args json.RawMessage) ([]byte, error) {
	return dispatchSceneRuntimeCommand(args, t.Name(), nil)
}

type ApplySceneTool struct{}

func (t *ApplySceneTool) Name() string        { return "godot.scene.apply" }
func (t *ApplySceneTool) Description() string { return "Applies a scene to the current project" }
func (t *ApplySceneTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"path": map[string]any{"type": "string", "description": "Scene path to open"},
		},
		Required: []string{"path"},
		Title:    "Apply Scene",
	}
}
func (t *ApplySceneTool) Execute(args json.RawMessage) ([]byte, error) {
	return dispatchSceneRuntimeCommand(args, t.Name(), validateApplySceneArguments)
}

func GetAllTools() []types.Tool {
	return []types.Tool{
		&ListProjectScenesTool{},
		&ReadSceneTool{},
		&CreateSceneTool{},
		&SaveSceneTool{},
		&ApplySceneTool{},
	}
}

func parseSceneNodes(content string) []map[string]any {
	scanner := bufio.NewScanner(strings.NewReader(content))
	nodes := make([]map[string]any, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "[node ") || !strings.HasSuffix(line, "]") {
			continue
		}
		matches := sceneNodePattern.FindAllStringSubmatch(line, -1)
		if len(matches) == 0 {
			continue
		}
		node := map[string]any{}
		for _, match := range matches {
			if len(match) < 3 {
				continue
			}
			node[match[1]] = match[2]
		}
		if _, ok := node["name"]; !ok {
			node["name"] = ""
		}
		if _, ok := node["type"]; !ok {
			node["type"] = ""
		}
		if _, ok := node["parent"]; !ok {
			node["parent"] = ""
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	lines := 1
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	return lines
}

func dispatchSceneRuntimeCommand(rawArgs json.RawMessage, commandName string, validate func(map[string]any, string) (map[string]any, error)) ([]byte, error) {
	return types.DispatchRuntimeCommand(types.RuntimeCommandDispatchOptions{
		RawArgs:                  rawArgs,
		CommandName:              commandName,
		Timeout:                  sceneCommandTimeout,
		SessionRequiredMessage:   "Scene commands require an initialized MCP HTTP session",
		BridgeUnavailableMessage: "Scene runtime bridge is unavailable",
		InvalidJSONError: func(err error) error {
			return newSceneInvalidParamsError("Invalid JSON arguments", commandName, "invalid_json", map[string]any{"error": err.Error()})
		},
		Validate: validate,
	})
}

func validateCreateSceneArguments(arguments map[string]any, toolName string) (map[string]any, error) {
	path, err := requiredSceneString(arguments, "path", toolName, "missing_path")
	if err != nil {
		return nil, err
	}
	out := map[string]any{"path": path}
	if raw, exists := arguments["content"]; exists {
		content, ok := raw.(string)
		if !ok {
			return nil, newSceneInvalidParamsError("content must be a string", toolName, "invalid_content_type", nil)
		}
		out["content"] = content
	}
	if raw, exists := arguments["template"]; exists {
		template, ok := raw.(string)
		if !ok {
			return nil, newSceneInvalidParamsError("template must be a string", toolName, "invalid_template_type", nil)
		}
		out["template"] = strings.TrimSpace(template)
	}
	return out, nil
}

func validateApplySceneArguments(arguments map[string]any, toolName string) (map[string]any, error) {
	path := ""
	if raw, exists := arguments["path"]; exists {
		value, ok := raw.(string)
		if !ok {
			return nil, newSceneInvalidParamsError("path must be a string", toolName, "invalid_path_type", nil)
		}
		path = strings.TrimSpace(value)
	}
	if path == "" {
		return nil, newSceneInvalidParamsError("path is required", toolName, "missing_path", nil)
	}
	return map[string]any{"path": path}, nil
}

func requiredSceneString(arguments map[string]any, key, toolName, reason string) (string, error) {
	value, exists := arguments[key]
	if !exists {
		return "", newSceneInvalidParamsError(key+" is required", toolName, reason, nil)
	}
	asString, ok := value.(string)
	if !ok {
		return "", newSceneInvalidParamsError(key+" must be a string", toolName, "invalid_"+key+"_type", nil)
	}
	asString = strings.TrimSpace(asString)
	if asString == "" {
		return "", newSceneInvalidParamsError(key+" must not be empty", toolName, reason, nil)
	}
	return asString, nil
}

func newSceneInvalidParamsError(message, toolName, reason string, extra map[string]any) error {
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
