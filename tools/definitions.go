package tools

import (
	"github.com/slighter12/godot-mcp-go/tools/node"
	"github.com/slighter12/godot-mcp-go/tools/project"
	"github.com/slighter12/godot-mcp-go/tools/scene"
	"github.com/slighter12/godot-mcp-go/tools/script"
	"github.com/slighter12/godot-mcp-go/tools/types"
	"github.com/slighter12/godot-mcp-go/tools/utility"
)

// GetAllTools returns all available tools from all categories
func GetAllTools() []types.Tool {
	var all []types.Tool
	all = append(all, node.GetAllTools()...)
	all = append(all, script.GetAllTools()...)
	all = append(all, scene.GetAllTools()...)
	all = append(all, project.GetAllTools()...)
	all = append(all, utility.GetAllTools()...)
	return all
}
