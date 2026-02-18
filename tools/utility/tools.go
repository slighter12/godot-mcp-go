package utility

import (
	"encoding/json"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/tools/types"
)

type ListOfferingsTool struct{}

func (t *ListOfferingsTool) Name() string        { return "list-offerings" }
func (t *ListOfferingsTool) Description() string { return "Lists available offerings" }
func (t *ListOfferingsTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "List Offerings"}
}
func (t *ListOfferingsTool) Execute(args json.RawMessage) ([]byte, error) {
	result := map[string]any{"offerings": []map[string]any{{"name": "godot-mcp", "version": "0.1.0", "capabilities": map[string]any{"tools": map[string]any{}, "resources": map[string]any{}, "prompts": map[string]any{}}, "serverInfo": map[string]any{"name": "godot-mcp-go", "version": "0.1.0"}}}}
	return json.Marshal(result)
}

func GetAllTools() []types.Tool {
	return []types.Tool{
		&ListOfferingsTool{},
		NewSyncEditorRuntimeTool(),
		NewPingEditorRuntimeTool(),
		NewAckEditorCommandTool(),
	}
}
