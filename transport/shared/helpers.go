package shared

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/tools"
)

const pageSize = 50

func BuildToolsListResponse(msg jsonrpc.Request, tools []mcp.Tool) *jsonrpc.Response {
	sortedTools := append([]mcp.Tool(nil), tools...)
	sort.Slice(sortedTools, func(i, j int) bool {
		return sortedTools[i].Name < sortedTools[j].Name
	})

	start, err := ParseCursor(msg.Params, len(sortedTools))
	if err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
	}
	end := min(start+pageSize, len(sortedTools))

	result := map[string]any{
		"tools": sortedTools[start:end],
	}
	if end < len(sortedTools) {
		result["nextCursor"] = strconv.Itoa(end)
	}
	return jsonrpc.NewResponse(msg.ID, result)
}

func BuildResourcesListResponse(msg jsonrpc.Request) *jsonrpc.Response {
	resources := defaultResources()
	start, err := ParseCursor(msg.Params, len(resources))
	if err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
	}
	end := min(start+pageSize, len(resources))

	result := map[string]any{
		"resources": resources[start:end],
	}
	if end < len(resources) {
		result["nextCursor"] = strconv.Itoa(end)
	}
	return jsonrpc.NewResponse(msg.ID, result)
}

func BuildResourcesReadResponse(msg jsonrpc.Request, readResource func(string) (any, error)) *jsonrpc.Response {
	var params struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Invalid resources/read payload", nil)
	}
	if params.URI == "" {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Resource URI is required", nil)
	}

	result, err := readResource(params.URI)
	if err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInternalError), "Failed to encode resource result", nil)
	}

	return jsonrpc.NewResponse(msg.ID, map[string]any{
		"contents": []map[string]any{
			{
				"uri":      params.URI,
				"mimeType": "application/json",
				"text":     string(resultJSON),
			},
		},
	})
}

func BuildPromptsListResponse(msg jsonrpc.Request) *jsonrpc.Response {
	prompts := []map[string]any{}
	start, err := ParseCursor(msg.Params, len(prompts))
	if err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
	}
	if start != 0 {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Invalid cursor value", nil)
	}
	return jsonrpc.NewResponse(msg.ID, map[string]any{
		"prompts": prompts,
	})
}

func BuildPromptsGetResponse(msg jsonrpc.Request) *jsonrpc.Response {
	var params struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Invalid prompts/get payload", nil)
	}
	if params.Name == "" {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Prompt name is required", nil)
	}
	return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrMethodNotFound), "Prompt not found", map[string]any{
		"name": params.Name,
	})
}

func BuildPingResponse(msg jsonrpc.Request) *jsonrpc.Response {
	return jsonrpc.NewResponse(msg.ID, &mcp.PongMessage{
		Type: string(mcp.TypePong),
	})
}

// DispatchStandardMethod handles shared non-initialize JSON-RPC methods for all transports.
func DispatchStandardMethod(msg jsonrpc.Request, toolManager *tools.Manager, readResource func(string) (any, error)) any {
	switch msg.Method {
	case "tools/list":
		return BuildToolsListResponse(msg, toolManager.GetTools())
	case "resources/list":
		return BuildResourcesListResponse(msg)
	case "resources/read":
		return BuildResourcesReadResponse(msg, readResource)
	case "prompts/list":
		return BuildPromptsListResponse(msg)
	case "prompts/get":
		return BuildPromptsGetResponse(msg)
	case "tools/call":
		return BuildToolCallResponse(msg, toolManager, readResource)
	case "ping":
		return BuildPingResponse(msg)
	case "tools/progress":
		if msg.ID != nil {
			return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil)
		}
		return nil
	default:
		if msg.ID != nil {
			return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrMethodNotFound), "Method not found", map[string]any{
				"method": msg.Method,
			})
		}
		return nil
	}
}

func BuildToolCallResponse(msg jsonrpc.Request, toolManager *tools.Manager, readResource func(string) (any, error)) *jsonrpc.Response {
	var toolCall struct {
		Name      string         `json:"name"`
		Tool      string         `json:"tool"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(msg.Params, &toolCall); err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Invalid tool call payload", nil)
	}

	toolName := strings.TrimSpace(toolCall.Name)
	if toolName == "" {
		toolName = strings.TrimSpace(toolCall.Tool)
	}
	if toolName == "" {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Tool name is required", nil)
	}

	arguments := toolCall.Arguments
	if arguments == nil {
		arguments = map[string]any{}
	}

	if strings.HasPrefix(toolName, "godot://") {
		if readResource == nil {
			return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Resource handler is not configured", nil)
		}
		result, err := readResource(toolName)
		if err != nil {
			return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
		}
		return jsonrpc.NewResponse(msg.ID, BuildToolSuccessResult(toolName, result))
	}

	result, err := toolManager.CallTool(toolName, arguments)
	if err != nil {
		if tools.IsToolNotFound(err) {
			return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
		}
		return jsonrpc.NewResponse(msg.ID, map[string]any{
			"type":    string(mcp.TypeResult),
			"tool":    toolName,
			"content": []map[string]any{{"type": "text", "text": err.Error()}},
			"isError": true,
		})
	}

	return jsonrpc.NewResponse(msg.ID, BuildToolSuccessResult(toolName, result))
}

func BuildToolSuccessResult(toolName string, result any) map[string]any {
	return map[string]any{
		"type":              string(mcp.TypeResult),
		"tool":              toolName,
		"result":            result,
		"content":           ToolContentFromResult(result),
		"structuredContent": result,
		"isError":           false,
	}
}

func ToolContentFromResult(result any) []map[string]any {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return []map[string]any{{"type": "text", "text": "tool call completed"}}
	}
	return []map[string]any{{"type": "text", "text": string(resultJSON)}}
}

func ServerCapabilities() map[string]any {
	return map[string]any{
		"tools":     map[string]any{},
		"resources": map[string]any{},
		"prompts":   map[string]any{},
	}
}

func ParseCursor(paramsRaw json.RawMessage, total int) (int, error) {
	if len(paramsRaw) == 0 {
		return 0, nil
	}

	var params struct {
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return 0, fmt.Errorf("invalid params payload")
	}
	if strings.TrimSpace(params.Cursor) == "" {
		return 0, nil
	}

	offset, err := strconv.Atoi(params.Cursor)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor value")
	}
	if offset < 0 || offset > total {
		return 0, fmt.Errorf("invalid cursor value")
	}
	return offset, nil
}

func defaultResources() []map[string]any {
	return []map[string]any{
		{
			"uri":      "godot://project/info",
			"name":     "Project Info",
			"mimeType": "application/json",
		},
		{
			"uri":      "godot://scene/current",
			"name":     "Current Scene",
			"mimeType": "application/json",
		},
		{
			"uri":      "godot://script/current",
			"name":     "Current Script",
			"mimeType": "application/json",
		},
	}
}

// ParseJSONRPCFrame validates and parses one JSON-RPC message frame.
// Both stdio and streamable HTTP currently require a single message per frame.
func ParseJSONRPCFrame(frame []byte) ([]jsonrpc.Request, []any, bool, error) {
	trimmed := bytes.TrimSpace(frame)
	if len(trimmed) == 0 {
		return nil, nil, false, fmt.Errorf("empty message")
	}

	if trimmed[0] == '[' {
		return nil, []any{jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil)}, false, nil
	}

	rawMessages := []json.RawMessage{json.RawMessage(trimmed)}
	requests := make([]jsonrpc.Request, 0, len(rawMessages))
	prebuiltResponses := make([]any, 0)
	acceptedOneWay := false

	for _, rawMsg := range rawMessages {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(rawMsg, &envelope); err != nil {
			prebuiltResponses = append(prebuiltResponses, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		requestID, hasID, validID := parseIDFromEnvelope(envelope)
		if !validID {
			prebuiltResponses = append(prebuiltResponses, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		var msg jsonrpc.Request
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			prebuiltResponses = append(prebuiltResponses, jsonrpc.NewErrorResponse(requestID, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		if msg.Method == "" {
			_, hasResult := envelope["result"]
			_, hasErr := envelope["error"]
			if hasResult || hasErr {
				if msg.JSONRPC != jsonrpc.Version || !hasID || (hasResult && hasErr) {
					prebuiltResponses = append(prebuiltResponses, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
				} else {
					acceptedOneWay = true
				}
				continue
			}
			prebuiltResponses = append(prebuiltResponses, jsonrpc.NewErrorResponse(requestID, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		if msg.JSONRPC != jsonrpc.Version {
			prebuiltResponses = append(prebuiltResponses, jsonrpc.NewErrorResponse(requestID, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		if rawParams, ok := envelope["params"]; ok && !isValidParamsValue(rawParams) {
			prebuiltResponses = append(prebuiltResponses, jsonrpc.NewErrorResponse(requestID, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		if msg.Method == "initialize" && msg.ID == nil {
			prebuiltResponses = append(prebuiltResponses, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		requests = append(requests, msg)
	}

	return requests, prebuiltResponses, acceptedOneWay, nil
}

func parseIDFromEnvelope(envelope map[string]json.RawMessage) (any, bool, bool) {
	rawID, exists := envelope["id"]
	if !exists {
		return nil, false, true
	}
	trimmed := bytes.TrimSpace(rawID)
	if len(trimmed) == 0 {
		return nil, true, false
	}

	var id any
	if err := json.Unmarshal(trimmed, &id); err != nil {
		return nil, true, false
	}
	if !isValidJSONRPCID(id) {
		return nil, true, false
	}
	return id, true, true
}

func isValidJSONRPCID(id any) bool {
	switch id.(type) {
	case nil, string, float64, int, int64, int32, uint, uint64, uint32:
		return true
	default:
		return false
	}
}

func isValidParamsValue(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}
	return trimmed[0] == '{' || trimmed[0] == '['
}
