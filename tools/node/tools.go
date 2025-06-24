package node

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/tools/types"
)

// GetSceneTreeTool returns the scene tree structure
type GetSceneTreeTool struct{}

func (t *GetSceneTreeTool) Name() string        { return "get-scene-tree" }
func (t *GetSceneTreeTool) Description() string { return "Returns the scene tree structure" }
func (t *GetSceneTreeTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "Get Scene Tree"}
}
func (t *GetSceneTreeTool) Execute(args json.RawMessage) ([]byte, error) {
	result := map[string]any{"root": map[string]any{"name": "Root", "type": "Node", "children": []any{}}}
	return json.Marshal(result)
}

type GetNodePropertiesTool struct{}

func (t *GetNodePropertiesTool) Name() string        { return "get-node-properties" }
func (t *GetNodePropertiesTool) Description() string { return "Gets properties of a specific node" }
func (t *GetNodePropertiesTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"node": map[string]any{"type": "string", "description": "Node path"}}, Required: []string{"node"}, Title: "Get Node Properties"}
}
func (t *GetNodePropertiesTool) Execute(args json.RawMessage) ([]byte, error) {
	var argsMap map[string]any
	if err := json.Unmarshal(args, &argsMap); err != nil {
		return nil, err
	}
	node, ok := argsMap["node"].(string)
	if !ok {
		return nil, errors.New("invalid node path")
	}
	result := map[string]any{"name": node, "type": "Node", "properties": map[string]any{}}
	return json.Marshal(result)
}

type CreateNodeTool struct{}

func (t *CreateNodeTool) Name() string        { return "create-node" }
func (t *CreateNodeTool) Description() string { return "Creates a new node" }
func (t *CreateNodeTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"type": map[string]any{"type": "string", "description": "Node type"}, "parent": map[string]any{"type": "string", "description": "Parent node path"}}, Required: []string{"type", "parent"}, Title: "Create Node"}
}
func (t *CreateNodeTool) Execute(args json.RawMessage) ([]byte, error) {
	var argsMap map[string]any
	if err := json.Unmarshal(args, &argsMap); err != nil {
		return nil, err
	}
	nodeType, ok1 := argsMap["type"].(string)
	parent, ok2 := argsMap["parent"].(string)
	if !ok1 || !ok2 {
		return nil, errors.New("invalid node creation parameters")
	}
	result := map[string]any{"name": fmt.Sprintf("New%s", nodeType), "type": nodeType, "parent": parent}
	return json.Marshal(result)
}

type DeleteNodeTool struct{}

func (t *DeleteNodeTool) Name() string        { return "delete-node" }
func (t *DeleteNodeTool) Description() string { return "Deletes a node" }
func (t *DeleteNodeTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"node": map[string]any{"type": "string", "description": "Node path"}}, Required: []string{"node"}, Title: "Delete Node"}
}
func (t *DeleteNodeTool) Execute(args json.RawMessage) ([]byte, error) {
	var argsMap map[string]any
	if err := json.Unmarshal(args, &argsMap); err != nil {
		return nil, err
	}
	node, ok := argsMap["node"].(string)
	if !ok {
		return nil, errors.New("invalid node path")
	}
	result := map[string]any{"success": true, "node": node}
	return json.Marshal(result)
}

type ModifyNodeTool struct{}

func (t *ModifyNodeTool) Name() string        { return "modify-node" }
func (t *ModifyNodeTool) Description() string { return "Updates node properties" }
func (t *ModifyNodeTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"node": map[string]any{"type": "string", "description": "Node path"}, "properties": map[string]any{"type": "object", "description": "Properties to update"}}, Required: []string{"node", "properties"}, Title: "Modify Node"}
}
func (t *ModifyNodeTool) Execute(args json.RawMessage) ([]byte, error) {
	var argsMap map[string]any
	if err := json.Unmarshal(args, &argsMap); err != nil {
		return nil, err
	}
	node, ok1 := argsMap["node"].(string)
	properties, ok2 := argsMap["properties"].(map[string]any)
	if !ok1 || !ok2 {
		return nil, errors.New("invalid node modification parameters")
	}
	result := map[string]any{"success": true, "node": node, "properties": properties}
	return json.Marshal(result)
}

func GetAllTools() []types.Tool {
	return []types.Tool{
		&GetSceneTreeTool{},
		&GetNodePropertiesTool{},
		&CreateNodeTool{},
		&DeleteNodeTool{},
		&ModifyNodeTool{},
	}
}
