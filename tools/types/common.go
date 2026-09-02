package types

import (
	"context"
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

// AnnotatedTool extends Tool with MCP annotations for discoverability.
type AnnotatedTool interface {
	Tool
	Annotations() *mcp.ToolAnnotations
}

// OutputSchemaTool extends Tool with an opaque JSON Schema 2020-12 output schema.
// The schema is transported unchanged; the server does not dereference it.
type OutputSchemaTool interface {
	Tool
	OutputSchema() map[string]any
}

// ContentResultTool extends Tool with a protocol-aware result path that
// preserves standard MCP content blocks and structured content.
type ContentResultTool interface {
	Tool
	ExecuteContent(args json.RawMessage) (mcp.CompleteResult, error)
}

// MultiRoundTripTool opts a tool into the MCP 2026-07-28 input-required flow.
// Modern dispatch requires request-state configuration before entering the
// handler. Execute remains the legacy and non-modern compatibility path.
type MultiRoundTripTool interface {
	Tool
	ExecuteRoundTrip(context.Context, mcp.RoundTripRequest) (mcp.RoundTripOutcome, error)
}

// BoolPtr returns a pointer to a bool value.
func BoolPtr(b bool) *bool { return &b }

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
