package types

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/slighter12/godot-mcp-go/mcp"
)

// Tool interface defines the contract for all tools
type Tool interface {
	Name() string
	Description() string
	InputSchema() mcp.InputSchema
	Execute(args json.RawMessage) ([]byte, error)
}

// ToolRegistry interface defines the contract for tool registries
type ToolRegistry interface {
	RegisterTool(tool Tool) error
	GetTool(name string) (Tool, bool)
	ListTools() []Tool
	ExecuteTool(name string, args json.RawMessage) ([]byte, error)
}

// ResolveProjectRootFromEnvOrCWD resolves the Godot project root by checking
// GODOT_PROJECT_ROOT first, then searching upward from current directory for
// project.godot, and finally falling back to current directory.
func ResolveProjectRootFromEnvOrCWD() string {
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
