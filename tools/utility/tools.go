package utility

import (
	"encoding/json"
	"time"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
	"github.com/slighter12/godot-mcp-go/tools/types"
)

type ListOfferingsTool struct{}

func (t *ListOfferingsTool) Name() string        { return "godot-offerings-list" }
func (t *ListOfferingsTool) Description() string { return "Lists available offerings" }
func (t *ListOfferingsTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "List Offerings"}
}
func (t *ListOfferingsTool) Execute(args json.RawMessage) ([]byte, error) {
	result := map[string]any{"offerings": []map[string]any{{"name": "godot-mcp", "version": "0.1.0", "capabilities": map[string]any{"tools": map[string]any{}, "resources": map[string]any{}, "prompts": map[string]any{}}, "serverInfo": map[string]any{"name": "godot-mcp-go", "version": "0.1.0"}}}}
	return json.Marshal(result)
}

// RuntimeHealthTool returns runtime bridge freshness and command broker metrics.
type RuntimeHealthTool struct{}

func NewRuntimeHealthTool() *RuntimeHealthTool {
	return &RuntimeHealthTool{}
}

func (t *RuntimeHealthTool) Name() string { return "godot-runtime-get-health" }

func (t *RuntimeHealthTool) Description() string {
	return "Returns runtime bridge freshness and command broker health metrics"
}

func (t *RuntimeHealthTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
		Title:      "Get Runtime Health",
	}
}

func (t *RuntimeHealthTool) Execute(args json.RawMessage) ([]byte, error) {
	return json.Marshal(runtimebridge.HealthSnapshot(time.Now().UTC()))
}

func GetAllTools() []types.Tool {
	return []types.Tool{
		&ListOfferingsTool{},
		NewRuntimeHealthTool(),
		NewSyncEditorRuntimeTool(),
		NewPingEditorRuntimeTool(),
		NewAckEditorCommandTool(),
	}
}
