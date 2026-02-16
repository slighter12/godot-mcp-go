package shared

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
	"github.com/slighter12/godot-mcp-go/tools"
)

const pageSize = 50
const maxRenderedPromptBytes = 128 * 1024

var promptPlaceholderPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

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

func BuildPromptsListResponse(msg jsonrpc.Request, catalog *promptcatalog.Registry) *jsonrpc.Response {
	if catalog == nil || !catalog.Enabled() {
		return semanticError(msg.ID, jsonrpc.ErrMethodNotFound, "Feature not supported", "not_supported", map[string]any{
			"feature": "prompt_catalog",
		})
	}

	if catalog.PromptCount() == 0 {
		loadErrors := catalog.LoadErrors()
		if len(loadErrors) > 0 {
			return semanticError(msg.ID, jsonrpc.ErrServerError, "Resource temporarily unavailable", "not_available", map[string]any{
				"feature": "prompt_catalog",
				"details": strings.Join(loadErrors, "; "),
			})
		}
	}

	prompts := catalog.ListPrompts()
	start, err := ParseCursor(msg.Params, len(prompts))
	if err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
	}
	end := min(start+pageSize, len(prompts))

	list := make([]map[string]any, 0, end-start)
	for _, prompt := range prompts[start:end] {
		list = append(list, map[string]any{
			"name":        prompt.Name,
			"description": prompt.Description,
		})
	}

	result := map[string]any{
		"prompts": list,
	}
	if end < len(prompts) {
		result["nextCursor"] = strconv.Itoa(end)
	}
	return jsonrpc.NewResponse(msg.ID, result)
}

func BuildPromptsGetResponse(msg jsonrpc.Request, catalog *promptcatalog.Registry) *jsonrpc.Response {
	if catalog == nil || !catalog.Enabled() {
		return semanticError(msg.ID, jsonrpc.ErrMethodNotFound, "Feature not supported", "not_supported", map[string]any{
			"feature": "prompt_catalog",
		})
	}

	if catalog.PromptCount() == 0 {
		loadErrors := catalog.LoadErrors()
		if len(loadErrors) > 0 {
			return semanticError(msg.ID, jsonrpc.ErrServerError, "Resource temporarily unavailable", "not_available", map[string]any{
				"feature": "prompt_catalog",
				"details": strings.Join(loadErrors, "; "),
			})
		}
	}

	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments,omitempty"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return semanticError(msg.ID, jsonrpc.ErrInvalidParams, "Invalid prompts/get payload", "invalid_params", map[string]any{
			"field":   "params",
			"problem": "malformed_payload",
		})
	}
	params.Name = strings.TrimSpace(params.Name)
	if params.Name == "" {
		return semanticError(msg.ID, jsonrpc.ErrInvalidParams, "Prompt name is required", "invalid_params", map[string]any{
			"field":   "name",
			"problem": "missing",
		})
	}

	prompt, found := catalog.GetPrompt(params.Name)
	if !found {
		return semanticError(msg.ID, jsonrpc.ErrInvalidParams, "Unknown prompt name", "invalid_params", map[string]any{
			"field":   "name",
			"problem": "unknown_prompt",
			"value":   params.Name,
		})
	}

	renderedPrompt, renderErr := renderPromptTemplate(prompt.Template, params.Arguments)
	if renderErr != nil {
		return semanticError(msg.ID, jsonrpc.ErrInvalidParams, "Prompt arguments produced oversized output", "invalid_params", map[string]any{
			"field":    "arguments",
			"problem":  "rendered_prompt_too_large",
			"maxBytes": maxRenderedPromptBytes,
		})
	}
	return jsonrpc.NewResponse(msg.ID, map[string]any{
		"name":        prompt.Name,
		"description": prompt.Description,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": map[string]any{
					"type": "text",
					"text": renderedPrompt,
				},
			},
		},
	})
}

func BuildPingResponse(msg jsonrpc.Request) *jsonrpc.Response {
	return jsonrpc.NewResponse(msg.ID, map[string]any{})
}

// DispatchStandardMethod handles shared non-initialize JSON-RPC methods for all transports.
func DispatchStandardMethod(msg jsonrpc.Request, toolManager *tools.Manager, catalog *promptcatalog.Registry, readResource func(string) (any, error)) any {
	switch msg.Method {
	case "tools/list":
		return BuildToolsListResponse(msg, toolManager.GetTools())
	case "resources/list":
		return BuildResourcesListResponse(msg)
	case "resources/read":
		return BuildResourcesReadResponse(msg, readResource)
	case "prompts/list":
		return BuildPromptsListResponse(msg, catalog)
	case "prompts/get":
		return BuildPromptsGetResponse(msg, catalog)
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

func semanticError(id any, code jsonrpc.ErrorCode, message, kind string, extra map[string]any) *jsonrpc.Response {
	data := map[string]any{
		"kind": kind,
	}
	for key, value := range extra {
		data[key] = value
	}
	return jsonrpc.NewErrorResponse(id, int(code), message, data)
}

func renderPromptTemplate(template string, arguments map[string]any) (string, error) {
	if template == "" || len(arguments) == 0 {
		return template, nil
	}

	normalizedArgs := make(map[string]string, len(arguments))
	for key, value := range arguments {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		normalizedArgs[trimmedKey] = normalizePromptArgumentValue(value)
	}
	if len(normalizedArgs) == 0 {
		return template, nil
	}

	matches := promptPlaceholderPattern.FindAllStringSubmatchIndex(template, -1)
	if len(matches) == 0 {
		return template, nil
	}

	var b strings.Builder
	b.Grow(len(template))
	last := 0

	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		start, end := match[0], match[1]
		keyStart, keyEnd := match[2], match[3]

		segment := template[last:start]
		if err := appendBounded(&b, segment); err != nil {
			return "", err
		}

		key := template[keyStart:keyEnd]
		if value, ok := normalizedArgs[key]; ok {
			if err := appendBounded(&b, value); err != nil {
				return "", err
			}
		} else {
			if err := appendBounded(&b, template[start:end]); err != nil {
				return "", err
			}
		}

		last = end
	}

	if err := appendBounded(&b, template[last:]); err != nil {
		return "", err
	}
	return b.String(), nil
}

func normalizePromptArgumentValue(value any) string {
	if text, ok := value.(string); ok {
		return strings.ReplaceAll(text, "\x00", "")
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return strings.ReplaceAll(fmt.Sprint(value), "\x00", "")
	}
	return strings.ReplaceAll(string(raw), "\x00", "")
}

func appendBounded(builder *strings.Builder, segment string) error {
	if builder.Len()+len(segment) > maxRenderedPromptBytes {
		return fmt.Errorf("rendered prompt exceeds %d bytes", maxRenderedPromptBytes)
	}
	builder.WriteString(segment)
	return nil
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

func ServerCapabilities(promptCatalogEnabled bool) map[string]any {
	capabilities := map[string]any{
		"tools":     map[string]any{},
		"resources": map[string]any{},
	}
	if promptCatalogEnabled {
		capabilities["prompts"] = map[string]any{}
	}
	return capabilities
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
		{
			"uri":      "godot://policy/godot-checks",
			"name":     "Godot Policy Checks",
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
			prebuiltResponses = append(prebuiltResponses, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrParseError), "Parse error", nil))
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
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&id); err != nil {
		return nil, true, false
	}
	if !isValidJSONRPCID(id) {
		return nil, true, false
	}
	return id, true, true
}

func isValidJSONRPCID(id any) bool {
	switch v := id.(type) {
	case string:
		return true
	case json.Number:
		return isJSONInteger(v.String())
	default:
		return false
	}
}

func isValidParamsValue(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}
	return trimmed[0] == '{'
}

func isJSONInteger(value string) bool {
	if value == "" || strings.ContainsAny(value, ".eE") {
		return false
	}
	if _, err := strconv.ParseInt(value, 10, 64); err == nil {
		return true
	}
	if strings.HasPrefix(value, "-") {
		return false
	}
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil
}
