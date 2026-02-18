package node

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

// GetSceneTreeTool returns the scene tree structure.
type GetSceneTreeTool struct{}

func (t *GetSceneTreeTool) Name() string        { return "get-scene-tree" }
func (t *GetSceneTreeTool) Description() string { return "Returns the scene tree structure" }
func (t *GetSceneTreeTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "Get Scene Tree"}
}
func (t *GetSceneTreeTool) Execute(args json.RawMessage) ([]byte, error) {
	var arguments map[string]any
	if err := json.Unmarshal(args, &arguments); err != nil {
		return nil, err
	}

	ctx := tooltypes.ExtractMCPContext(arguments)
	if strings.TrimSpace(ctx.SessionID) == "" || !ctx.SessionInitialized {
		return nil, tooltypes.NewNotAvailableError("Scene tree requires an initialized MCP HTTP session", map[string]any{
			"feature": "runtime_bridge",
			"reason":  "session_not_initialized",
			"tool":    t.Name(),
		})
	}

	stored, ok, reason := runtimebridge.DefaultStore().FreshForSession(ctx.SessionID, time.Now().UTC())
	if !ok {
		return nil, tooltypes.NewNotAvailableError("Scene tree is unavailable until runtime sync is healthy", map[string]any{
			"feature": "runtime_bridge",
			"reason":  reason,
			"tool":    t.Name(),
		})
	}

	result := map[string]any{
		"root":       stored.Snapshot.SceneTree,
		"session_id": stored.SessionID,
		"updated_at": stored.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	return json.Marshal(result)
}

type GetNodePropertiesTool struct{}

func (t *GetNodePropertiesTool) Name() string        { return "get-node-properties" }
func (t *GetNodePropertiesTool) Description() string { return "Gets properties of a specific node" }
func (t *GetNodePropertiesTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"node": map[string]any{"type": "string", "description": "Node path"}}, Required: []string{"node"}, Title: "Get Node Properties"}
}
func (t *GetNodePropertiesTool) Execute(args json.RawMessage) ([]byte, error) {
	var arguments map[string]any
	if err := json.Unmarshal(args, &arguments); err != nil {
		return nil, err
	}

	ctx := tooltypes.ExtractMCPContext(arguments)
	if strings.TrimSpace(ctx.SessionID) == "" || !ctx.SessionInitialized {
		return nil, tooltypes.NewNotAvailableError("Node properties require an initialized MCP HTTP session", map[string]any{
			"feature": "runtime_bridge",
			"reason":  "session_not_initialized",
			"tool":    t.Name(),
		})
	}

	payload := struct {
		Node string `json:"node"`
	}{}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, err
	}

	query := strings.TrimSpace(payload.Node)
	if query == "" {
		return nil, tooltypes.NewNotAvailableError("Node path is required", map[string]any{
			"feature": "runtime_bridge",
			"reason":  "missing_node_path",
			"tool":    t.Name(),
		})
	}

	stored, ok, reason := runtimebridge.DefaultStore().FreshForSession(ctx.SessionID, time.Now().UTC())
	if !ok {
		return nil, tooltypes.NewNotAvailableError("Node properties are unavailable until runtime sync is healthy", map[string]any{
			"feature": "runtime_bridge",
			"reason":  reason,
			"tool":    t.Name(),
		})
	}

	detail, found := resolveNodeDetail(stored.Snapshot.NodeDetails, query)
	if !found {
		return nil, tooltypes.NewNotAvailableError("Requested node is unavailable in the latest runtime snapshot", map[string]any{
			"feature": "runtime_bridge",
			"reason":  "node_not_found",
			"node":    query,
			"tool":    t.Name(),
		})
	}

	result := map[string]any{
		"path":        detail.Path,
		"name":        detail.Name,
		"type":        detail.Type,
		"owner":       detail.Owner,
		"script":      detail.Script,
		"groups":      detail.Groups,
		"child_count": detail.ChildCount,
		"properties": map[string]any{
			"path":        detail.Path,
			"name":        detail.Name,
			"type":        detail.Type,
			"owner":       detail.Owner,
			"script":      detail.Script,
			"groups":      detail.Groups,
			"child_count": detail.ChildCount,
		},
		"session_id": stored.SessionID,
		"updated_at": stored.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	return json.Marshal(result)
}

type CreateNodeTool struct{}

func (t *CreateNodeTool) Name() string        { return "create-node" }
func (t *CreateNodeTool) Description() string { return "Creates a new node" }
func (t *CreateNodeTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"type": map[string]any{"type": "string", "description": "Node type"}, "parent": map[string]any{"type": "string", "description": "Parent node path"}}, Required: []string{"type", "parent"}, Title: "Create Node"}
}
func (t *CreateNodeTool) Execute(args json.RawMessage) ([]byte, error) {
	return nil, tooltypes.NewNotAvailableError("Node writes are not available yet", map[string]any{
		"feature": "godot_runtime_write",
		"tool":    t.Name(),
	})
}

type DeleteNodeTool struct{}

func (t *DeleteNodeTool) Name() string        { return "delete-node" }
func (t *DeleteNodeTool) Description() string { return "Deletes a node" }
func (t *DeleteNodeTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"node": map[string]any{"type": "string", "description": "Node path"}}, Required: []string{"node"}, Title: "Delete Node"}
}
func (t *DeleteNodeTool) Execute(args json.RawMessage) ([]byte, error) {
	return nil, tooltypes.NewNotAvailableError("Node writes are not available yet", map[string]any{
		"feature": "godot_runtime_write",
		"tool":    t.Name(),
	})
}

type ModifyNodeTool struct{}

func (t *ModifyNodeTool) Name() string        { return "modify-node" }
func (t *ModifyNodeTool) Description() string { return "Updates node properties" }
func (t *ModifyNodeTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"node": map[string]any{"type": "string", "description": "Node path"}, "properties": map[string]any{"type": "object", "description": "Properties to update"}}, Required: []string{"node", "properties"}, Title: "Modify Node"}
}
func (t *ModifyNodeTool) Execute(args json.RawMessage) ([]byte, error) {
	return nil, tooltypes.NewNotAvailableError("Node writes are not available yet", map[string]any{
		"feature": "godot_runtime_write",
		"tool":    t.Name(),
	})
}

func GetAllTools() []tooltypes.Tool {
	return []tooltypes.Tool{
		&GetSceneTreeTool{},
		&GetNodePropertiesTool{},
		&CreateNodeTool{},
		&DeleteNodeTool{},
		&ModifyNodeTool{},
	}
}

func resolveNodeDetail(details map[string]runtimebridge.NodeDetail, nodePath string) (runtimebridge.NodeDetail, bool) {
	if len(details) == 0 {
		return runtimebridge.NodeDetail{}, false
	}
	if detail, ok := details[nodePath]; ok {
		return detail, true
	}
	for _, detail := range details {
		if detail.Path == nodePath || detail.Name == nodePath {
			return detail, true
		}
	}
	return runtimebridge.NodeDetail{}, false
}
