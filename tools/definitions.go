package tools

import (
	"github.com/slighter12/godot-mcp-go/tools/node"
	"github.com/slighter12/godot-mcp-go/tools/project"
	"github.com/slighter12/godot-mcp-go/tools/runtime"
	"github.com/slighter12/godot-mcp-go/tools/scene"
	"github.com/slighter12/godot-mcp-go/tools/script"
	"github.com/slighter12/godot-mcp-go/tools/types"
	"github.com/slighter12/godot-mcp-go/tools/utility"
)

// GetAllTools returns all available tools from all categories
func GetAllTools() []types.Tool {
	var all []types.Tool
	all = append(all,
		&node.CreateNodeTool{},
		&node.DeleteNodeTool{},
		&node.ModifyNodeTool{},
	)
	all = append(all, script.GetAllTools()...)
	all = append(all,
		&scene.ListProjectScenesTool{},
		&scene.ReadSceneTool{},
		&scene.CreateSceneTool{},
		&scene.SaveSceneTool{},
		&scene.ApplySceneTool{},
	)
	all = append(all, project.GetAllTools()...)
	all = append(all, runtime.GetAllTools()...)
	all = append(all, &utility.ListOfferingsTool{}, utility.NewRuntimeHealthTool(), utility.NewRuntimeDiagnoseTool())
	return all
}

// GetStdioTools returns tools that are transport-compatible for stdio mode.
// Runtime bridge and mutating command tools are intentionally excluded.
func GetStdioTools() []types.Tool {
	return []types.Tool{
		&scene.ListProjectScenesTool{},
		&scene.ReadSceneTool{},
		&script.ListProjectScriptsTool{},
		&script.ReadScriptTool{},
		&script.AnalyzeScriptTool{},
		&project.GetProjectSettingsTool{},
		&project.ListProjectResourcesTool{},
		&utility.ListOfferingsTool{},
		utility.NewRuntimeHealthTool(),
	}
}
