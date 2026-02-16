package scene

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/tools/types"
)

type ListProjectScenesTool struct{}

func (t *ListProjectScenesTool) Name() string        { return "list-project-scenes" }
func (t *ListProjectScenesTool) Description() string { return "Lists all scenes in the project" }
func (t *ListProjectScenesTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "List Project Scenes"}
}
func (t *ListProjectScenesTool) Execute(args json.RawMessage) ([]byte, error) {
	projectRoot := types.ResolveProjectRootFromEnvOrCWD()
	var scenes []string
	err := filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".tscn") {
			scenes = append(scenes, strings.TrimSuffix(filepath.Base(path), ".tscn"))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := map[string]any{"scenes": scenes}
	return json.Marshal(result)
}

type ReadSceneTool struct{}

func (t *ReadSceneTool) Name() string        { return "read-scene" }
func (t *ReadSceneTool) Description() string { return "Reads a specific scene" }
func (t *ReadSceneTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"path": map[string]any{"type": "string", "description": "Scene path"}}, Required: []string{"path"}, Title: "Read Scene"}
}
func (t *ReadSceneTool) Execute(args json.RawMessage) ([]byte, error) {
	var argsMap map[string]any
	if err := json.Unmarshal(args, &argsMap); err != nil {
		return nil, err
	}
	path, ok := argsMap["path"].(string)
	if !ok {
		return nil, errors.New("invalid scene path")
	}
	result := map[string]any{"path": path, "nodes": []any{}}
	return json.Marshal(result)
}

type CreateSceneTool struct{}

func (t *CreateSceneTool) Name() string        { return "create-scene" }
func (t *CreateSceneTool) Description() string { return "Creates a new scene" }
func (t *CreateSceneTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"path": map[string]any{"type": "string", "description": "Scene path"}}, Required: []string{"path"}, Title: "Create Scene"}
}
func (t *CreateSceneTool) Execute(args json.RawMessage) ([]byte, error) {
	var argsMap map[string]any
	if err := json.Unmarshal(args, &argsMap); err != nil {
		return nil, err
	}
	path, ok := argsMap["path"].(string)
	if !ok {
		return nil, errors.New("invalid scene path")
	}
	result := map[string]any{"success": true, "path": path}
	return json.Marshal(result)
}

type SaveSceneTool struct{}

func (t *SaveSceneTool) Name() string        { return "save-scene" }
func (t *SaveSceneTool) Description() string { return "Saves the current scene" }
func (t *SaveSceneTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "Save Scene"}
}
func (t *SaveSceneTool) Execute(args json.RawMessage) ([]byte, error) {
	result := map[string]any{"success": true}
	return json.Marshal(result)
}

type ApplySceneTool struct{}

func (t *ApplySceneTool) Name() string        { return "apply-scene" }
func (t *ApplySceneTool) Description() string { return "Applies a scene to the current project" }
func (t *ApplySceneTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"scene": map[string]any{"type": "string", "description": "The name of the scene to apply"}}, Required: []string{"scene"}, Title: "Apply Scene"}
}
func (t *ApplySceneTool) Execute(args json.RawMessage) ([]byte, error) {
	var argsMap map[string]any
	if err := json.Unmarshal(args, &argsMap); err != nil {
		return nil, err
	}
	scene, ok := argsMap["scene"].(string)
	if !ok {
		return nil, errors.New("scene argument is required")
	}
	logger.Info("Applying scene", "scene", scene)
	result := map[string]bool{"success": true}
	return json.Marshal(result)
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
