package node

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

const nodeCommandTimeout = 8 * time.Second

// GetSceneTreeTool returns the scene tree structure.
type GetSceneTreeTool struct{}

func (t *GetSceneTreeTool) Name() string        { return "godot-node-get-tree" }
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

func (t *GetNodePropertiesTool) Name() string        { return "godot-node-get-properties" }
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

func (t *CreateNodeTool) Name() string        { return "godot-node-create" }
func (t *CreateNodeTool) Description() string { return "Creates a new node" }
func (t *CreateNodeTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"type":   map[string]any{"type": "string", "description": "Node type"},
			"parent": map[string]any{"type": "string", "description": "Parent node path"},
			"name":   map[string]any{"type": "string", "description": "Node name"},
		},
		Required: []string{"type", "parent", "name"},
		Title:    "Create Node",
	}
}
func (t *CreateNodeTool) Execute(args json.RawMessage) ([]byte, error) {
	return dispatchNodeRuntimeCommand(args, t.Name(), validateCreateNodeArguments)
}

type DeleteNodeTool struct{}

func (t *DeleteNodeTool) Name() string        { return "godot-node-delete" }
func (t *DeleteNodeTool) Description() string { return "Deletes a node" }
func (t *DeleteNodeTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"node": map[string]any{"type": "string", "description": "Node path"}}, Required: []string{"node"}, Title: "Delete Node"}
}
func (t *DeleteNodeTool) Execute(args json.RawMessage) ([]byte, error) {
	return dispatchNodeRuntimeCommand(args, t.Name(), validateDeleteNodeArguments)
}

type ModifyNodeTool struct{}

func (t *ModifyNodeTool) Name() string        { return "godot-node-modify" }
func (t *ModifyNodeTool) Description() string { return "Updates node properties" }
func (t *ModifyNodeTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"node": map[string]any{"type": "string", "description": "Node path"}, "properties": map[string]any{"type": "object", "description": "Properties to update"}}, Required: []string{"node", "properties"}, Title: "Modify Node"}
}
func (t *ModifyNodeTool) Execute(args json.RawMessage) ([]byte, error) {
	return dispatchNodeRuntimeCommand(args, t.Name(), validateModifyNodeArguments)
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

	keys := make([]string, 0, len(details))
	for key := range details {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		detail := details[key]
		if detail.Name == nodePath {
			return detail, true
		}
	}
	return runtimebridge.NodeDetail{}, false
}

func dispatchNodeRuntimeCommand(rawArgs json.RawMessage, commandName string, validate func(map[string]any, string) (map[string]any, error)) ([]byte, error) {
	var arguments map[string]any
	if err := json.Unmarshal(rawArgs, &arguments); err != nil {
		return nil, newNodeInvalidParamsError("Invalid JSON arguments", commandName, "invalid_json", map[string]any{"error": err.Error()})
	}

	ctx := tooltypes.ExtractMCPContext(arguments)
	if strings.TrimSpace(ctx.SessionID) == "" || !ctx.SessionInitialized {
		return nil, tooltypes.NewNotAvailableError("Node commands require an initialized MCP HTTP session", map[string]any{
			"feature": "runtime_bridge",
			"reason":  "session_not_initialized",
			"tool":    commandName,
		})
	}

	commandArgs := tooltypes.StripMCPContext(arguments)
	var err error
	if validate != nil {
		commandArgs, err = validate(commandArgs, commandName)
		if err != nil {
			return nil, err
		}
	}

	ack, ok, reason := runtimebridge.DefaultCommandBroker().DispatchAndWait(ctx.SessionID, commandName, commandArgs, nodeCommandTimeout)
	if !ok {
		return nil, tooltypes.NewNotAvailableError("Node runtime bridge is unavailable", map[string]any{
			"feature": "runtime_bridge",
			"reason":  reason,
			"tool":    commandName,
		})
	}

	result := map[string]any{
		"success":         ack.Success,
		"command_id":      ack.CommandID,
		"result":          ack.Result,
		"error":           ack.Error,
		"acknowledged_at": ack.AckedAt.UTC().Format(time.RFC3339Nano),
	}
	if schemaVersion, ok := ack.SchemaVersion(); ok {
		result["schema_version"] = schemaVersion
	}
	if reason, ok := ack.Reason(); ok {
		result["reason"] = reason
	}
	if retryable, ok := ack.Retryable(); ok {
		result["retryable"] = retryable
	}
	return json.Marshal(result)
}

func validateCreateNodeArguments(arguments map[string]any, toolName string) (map[string]any, error) {
	nodeType, err := requiredNodeString(arguments, "type", toolName, "missing_node_type")
	if err != nil {
		return nil, err
	}
	parent, err := requiredNodeString(arguments, "parent", toolName, "missing_parent_path")
	if err != nil {
		return nil, err
	}
	name, err := requiredNodeString(arguments, "name", toolName, "missing_node_name")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"type":   nodeType,
		"parent": parent,
		"name":   name,
	}, nil
}

func validateDeleteNodeArguments(arguments map[string]any, toolName string) (map[string]any, error) {
	nodePath, err := requiredNodeString(arguments, "node", toolName, "missing_node_path")
	if err != nil {
		return nil, err
	}
	return map[string]any{"node": nodePath}, nil
}

func validateModifyNodeArguments(arguments map[string]any, toolName string) (map[string]any, error) {
	nodePath, err := requiredNodeString(arguments, "node", toolName, "missing_node_path")
	if err != nil {
		return nil, err
	}

	rawProperties, exists := arguments["properties"]
	if !exists {
		return nil, newNodeInvalidParamsError("properties is required", toolName, "missing_properties", nil)
	}
	properties, ok := rawProperties.(map[string]any)
	if !ok {
		return nil, newNodeInvalidParamsError("properties must be an object", toolName, "invalid_properties_type", nil)
	}
	return map[string]any{
		"node":       nodePath,
		"properties": properties,
	}, nil
}

func requiredNodeString(arguments map[string]any, key, toolName, reason string) (string, error) {
	value, exists := arguments[key]
	if !exists {
		return "", newNodeInvalidParamsError(key+" is required", toolName, reason, nil)
	}
	asString, ok := value.(string)
	if !ok {
		return "", newNodeInvalidParamsError(key+" must be a string", toolName, "invalid_"+key+"_type", nil)
	}
	asString = strings.TrimSpace(asString)
	if asString == "" {
		return "", newNodeInvalidParamsError(key+" must not be empty", toolName, reason, nil)
	}
	return asString, nil
}

func newNodeInvalidParamsError(message, toolName, reason string, extra map[string]any) error {
	data := map[string]any{
		"feature": "runtime_bridge",
		"tool":    toolName,
	}
	if reason != "" {
		data["reason"] = reason
	}
	for key, value := range extra {
		data[key] = value
	}
	return tooltypes.NewSemanticError(tooltypes.SemanticKindInvalidParams, message, data)
}
