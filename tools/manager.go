package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/tools/types"
)

// ToolFunc represents a tool function (legacy type for backward compatibility)
type ToolFunc func(args map[string]any) (any, error)

var ErrToolNotFound = errors.New("tool not found")

func IsToolNotFound(err error) bool {
	return errors.Is(err, ErrToolNotFound)
}

// Manager implements ToolRegistry interface
type Manager struct {
	tools map[string]types.Tool
	mutex sync.RWMutex
}

// NewManager creates a new tool manager
func NewManager() *Manager {
	return &Manager{
		tools: make(map[string]types.Tool),
	}
}

// RegisterTool registers a new tool
func (m *Manager) RegisterTool(tool types.Tool) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if tool == nil {
		return errors.New("tool cannot be nil")
	}

	name := tool.Name()
	if name == "" {
		return errors.New("tool name cannot be empty")
	}

	m.tools[name] = tool
	logger.Debug("Tool registered", "name", name)
	return nil
}

// GetTool retrieves a tool by name
func (m *Manager) GetTool(name string) (types.Tool, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	tool, exists := m.tools[name]
	return tool, exists
}

// ListTools returns all registered tools
func (m *Manager) ListTools() []types.Tool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	tools := make([]types.Tool, 0, len(m.tools))
	for _, tool := range m.tools {
		tools = append(tools, tool)
	}
	return tools
}

// ExecuteTool executes a tool by name with the given arguments
func (m *Manager) ExecuteTool(name string, args json.RawMessage) ([]byte, error) {
	tool, exists := m.GetTool(name)
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}

	logger.Debug("Executing tool", "name", name, "args", string(args))
	return tool.Execute(args)
}

// RegisterDefaultTools registers all default tools
func (m *Manager) RegisterDefaultTools() {
	allTools := GetAllTools()
	for _, tool := range allTools {
		if err := m.RegisterTool(tool); err != nil {
			logger.Error("Failed to register tool", "name", tool.Name(), "error", err)
		}
	}
	logger.Info("Default tools registered", "count", len(allTools))
}

// GetTools returns a list of registered tools with their descriptions and schemas
// This method is kept for backward compatibility
func (m *Manager) GetTools() []mcp.Tool {
	tools := m.ListTools()
	mcpTools := make([]mcp.Tool, 0, len(tools))

	for _, tool := range tools {
		mcpTool := mcp.Tool{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.InputSchema(),
		}
		mcpTools = append(mcpTools, mcpTool)
	}

	return mcpTools
}

// CallTool calls a registered tool (kept for backward compatibility)
// This method converts map[string]any to json.RawMessage for compatibility
func (m *Manager) CallTool(name string, args map[string]any) (any, error) {
	// Convert map[string]any to json.RawMessage
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}

	// Execute tool with raw JSON
	resultJSON, err := m.ExecuteTool(name, argsJSON)
	if err != nil {
		return nil, err
	}

	// Convert result back to any
	var result any
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// RegisterToolByName registers a tool by name (kept for backward compatibility)
func (m *Manager) RegisterToolByName(name string, fn ToolFunc) {
	// Create a legacy tool from the function
	tool := &LegacyTool{
		name:        name,
		description: getToolDescription(name),
		schema:      getToolSchema(name),
		executor: func(args json.RawMessage) ([]byte, error) {
			// Convert json.RawMessage to map[string]any
			var argsMap map[string]any
			if err := json.Unmarshal(args, &argsMap); err != nil {
				return nil, err
			}

			// Call the legacy function
			result, err := fn(argsMap)
			if err != nil {
				return nil, err
			}

			// Convert result to JSON
			return json.Marshal(result)
		},
	}

	if err := m.RegisterTool(tool); err != nil {
		logger.Error("Failed to register tool by name", "name", name, "error", err)
	}
}

// LegacyTool implements Tool interface for backward compatibility
type LegacyTool struct {
	name        string
	description string
	schema      mcp.InputSchema
	executor    func(args json.RawMessage) ([]byte, error)
}

func (t *LegacyTool) Name() string {
	return t.name
}

func (t *LegacyTool) Description() string {
	return t.description
}

func (t *LegacyTool) InputSchema() mcp.InputSchema {
	return t.schema
}

func (t *LegacyTool) Execute(args json.RawMessage) ([]byte, error) {
	return t.executor(args)
}

// Legacy functions for backward compatibility

// ListScenesTool implements the listScenes tool
func ListScenesTool(args map[string]any) (any, error) {
	projectRoot := resolveProjectRootFromEnvOrCWD()
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
	return map[string]any{"scenes": scenes}, nil
}

func resolveProjectRootFromEnvOrCWD() string {
	envRoot := strings.TrimSpace(os.Getenv("GODOT_PROJECT_ROOT"))
	if envRoot != "" {
		if stat, err := os.Stat(envRoot); err == nil && stat.IsDir() {
			return envRoot
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return findProjectRootFromDir(wd)
}

func findProjectRootFromDir(startDir string) string {
	dir := startDir
	for {
		projectFile := filepath.Join(dir, "project.godot")
		if stat, err := os.Stat(projectFile); err == nil && !stat.IsDir() {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return startDir
}

// getToolDescription returns the description for a given tool
func getToolDescription(name string) string {
	switch name {
	case "listScenes":
		return "Lists all available scenes in the project"
	case "applyScene":
		return "Applies a scene to the current project"
	default:
		return ""
	}
}

// getToolSchema returns the input schema for a given tool
func getToolSchema(name string) mcp.InputSchema {
	switch name {
	case "listScenes":
		return mcp.InputSchema{
			Type:       "object",
			Properties: map[string]any{},
			Required:   []string{},
			Title:      "List Scenes",
		}
	case "applyScene":
		return mcp.InputSchema{
			Type: "object",
			Properties: map[string]any{
				"scene": map[string]any{
					"type":        "string",
					"description": "The name of the scene to apply",
				},
			},
			Required: []string{"scene"},
			Title:    "Apply Scene",
		}
	default:
		return mcp.InputSchema{
			Type:       "object",
			Properties: map[string]any{},
			Required:   []string{},
			Title:      "",
		}
	}
}
