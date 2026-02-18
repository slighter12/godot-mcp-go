package scene

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/tools/types"
)

var sceneNodePattern = regexp.MustCompile(`(name|type|parent)="([^"]*)"`)

type ListProjectScenesTool struct{}

func (t *ListProjectScenesTool) Name() string        { return "list-project-scenes" }
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

func (t *ReadSceneTool) Name() string        { return "read-scene" }
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

func (t *CreateSceneTool) Name() string        { return "create-scene" }
func (t *CreateSceneTool) Description() string { return "Creates a new scene" }
func (t *CreateSceneTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"path": map[string]any{"type": "string", "description": "Scene path"}}, Required: []string{"path"}, Title: "Create Scene"}
}
func (t *CreateSceneTool) Execute(args json.RawMessage) ([]byte, error) {
	return nil, types.NewNotAvailableError("Scene writes are not available yet", map[string]any{
		"feature": "godot_runtime_write",
		"tool":    t.Name(),
	})
}

type SaveSceneTool struct{}

func (t *SaveSceneTool) Name() string        { return "save-scene" }
func (t *SaveSceneTool) Description() string { return "Saves the current scene" }
func (t *SaveSceneTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "Save Scene"}
}
func (t *SaveSceneTool) Execute(args json.RawMessage) ([]byte, error) {
	return nil, types.NewNotAvailableError("Scene writes are not available yet", map[string]any{
		"feature": "godot_runtime_write",
		"tool":    t.Name(),
	})
}

type ApplySceneTool struct{}

func (t *ApplySceneTool) Name() string        { return "apply-scene" }
func (t *ApplySceneTool) Description() string { return "Applies a scene to the current project" }
func (t *ApplySceneTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"scene": map[string]any{"type": "string", "description": "The name of the scene to apply"}}, Required: []string{"scene"}, Title: "Apply Scene"}
}
func (t *ApplySceneTool) Execute(args json.RawMessage) ([]byte, error) {
	return nil, types.NewNotAvailableError("Scene writes are not available yet", map[string]any{
		"feature": "godot_runtime_write",
		"tool":    t.Name(),
	})
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
