package utility

import (
	"encoding/json"

	"github.com/slighter12/godot-mcp-go/mcp"
)

// PromptCatalogReloader executes a prompt catalog reload and returns structured metadata.
type PromptCatalogReloader func() map[string]any

type ReloadPromptCatalogTool struct {
	reload PromptCatalogReloader
}

func NewReloadPromptCatalogTool(reload PromptCatalogReloader) *ReloadPromptCatalogTool {
	return &ReloadPromptCatalogTool{reload: reload}
}

func (t *ReloadPromptCatalogTool) Name() string { return "reload-prompt-catalog" }

func (t *ReloadPromptCatalogTool) Description() string {
	return "Reloads prompt catalog entries from configured SKILL.md paths"
}

func (t *ReloadPromptCatalogTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
		Title:      "Reload Prompt Catalog",
	}
}

func (t *ReloadPromptCatalogTool) Execute(args json.RawMessage) ([]byte, error) {
	result := map[string]any{
		"changed":        false,
		"promptCount":    0,
		"loadErrorCount": 0,
		"status":         "disabled",
	}
	if t.reload != nil {
		result = t.reload()
	}
	return json.Marshal(result)
}
