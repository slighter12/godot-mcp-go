package types

import (
	"encoding/json"

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
