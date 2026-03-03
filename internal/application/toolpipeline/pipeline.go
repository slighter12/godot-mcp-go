package toolpipeline

import (
	"encoding/json"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/slighter12/godot-mcp-go/internal/domain/toolspec"
	"github.com/slighter12/godot-mcp-go/internal/infra/notifications"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/tools"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

const toolExecutionErrorMessage = "Tool execution failed"

type ToolCallContext struct {
	SessionID          string
	SessionInitialized bool
	MutatingAllowed    bool
}

type ToolCallOptions struct {
	SchemaValidationEnabled   bool
	RejectUnknownArguments    bool
	PermissionMode            string
	AllowedTools              []string
	EmitProgressNotifications bool
}

type ExecuteInput struct {
	Message      jsonrpc.Request
	ToolManager  *tools.Manager
	ReadResource func(string) (any, error)
	Context      ToolCallContext
	Options      ToolCallOptions
}

func Execute(input ExecuteInput) *jsonrpc.Response {
	var toolCall struct {
		Name      string         `json:"name"`
		Tool      string         `json:"tool"`
		Arguments map[string]any `json:"arguments"`
		Meta      map[string]any `json:"_meta"`
	}
	if err := json.Unmarshal(input.Message.Params, &toolCall); err != nil {
		return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInvalidParams), "Invalid tool call payload", nil)
	}

	toolName := strings.TrimSpace(toolCall.Name)
	if toolName == "" {
		toolName = strings.TrimSpace(toolCall.Tool)
	}
	if toolName == "" {
		return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInvalidParams), "Tool name is required", nil)
	}
	if !strings.HasPrefix(toolName, "godot://") && !toolspec.ValidateToolName(toolName) {
		return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInvalidParams), "Invalid tool name", nil)
	}

	startedAt := time.Now()
	defer func() {
		logger.Info(
			"tools.call.end",
			"tool", toolName,
			"session", strings.TrimSpace(input.Context.SessionID),
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
	}()

	arguments := toolCall.Arguments
	if arguments == nil {
		arguments = map[string]any{}
	}

	progressToken, hasProgressToken, progressTokenErr := extractProgressToken(toolCall.Meta)
	if progressTokenErr != nil {
		return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInvalidParams), progressTokenErr.Error(), nil)
	}

	if strings.HasPrefix(toolName, "godot://") {
		if !toolspec.IsToolAllowed(toolName, input.Options.PermissionMode, input.Options.AllowedTools) {
			return jsonrpc.NewResponse(input.Message.ID, buildToolSemanticErrorResult(toolName, tooltypes.NewSemanticError(
				tooltypes.SemanticKindNotSupported,
				"Tool call is blocked by permission policy",
				map[string]any{"reason": "permission_denied", "permission_mode": input.Options.PermissionMode},
			)))
		}
		if input.ReadResource == nil {
			return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInvalidParams), "Resource handler is not configured", nil)
		}
		result, err := input.ReadResource(toolName)
		if err != nil {
			return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
		}
		return jsonrpc.NewResponse(input.Message.ID, BuildToolSuccessResult(toolName, result))
	}

	canonicalToolName := toolName
	tool, found := input.ToolManager.GetTool(toolName)
	if found && tool != nil {
		canonicalToolName = tool.Name()
	}
	if !toolspec.ValidateToolName(canonicalToolName) {
		return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInvalidParams), "Invalid tool name", nil)
	}
	isInternalBridgeTool := toolspec.IsInternalBridgeTool(canonicalToolName)

	if found && tool != nil {
		if !isInternalBridgeTool && !toolspec.IsToolAllowed(canonicalToolName, input.Options.PermissionMode, input.Options.AllowedTools) {
			return jsonrpc.NewResponse(input.Message.ID, buildToolSemanticErrorResult(canonicalToolName, tooltypes.NewSemanticError(
				tooltypes.SemanticKindNotSupported,
				"Tool call is blocked by permission policy",
				map[string]any{"reason": "permission_denied", "permission_mode": input.Options.PermissionMode},
			)))
		}
		if toolspec.IsMutatingTool(canonicalToolName) && !input.Context.MutatingAllowed {
			return jsonrpc.NewResponse(input.Message.ID, buildToolSemanticErrorResult(canonicalToolName, tooltypes.NewSemanticError(
				tooltypes.SemanticKindNotSupported,
				"Mutating tools require initialize.params.capabilities.godot.mutating=true",
				map[string]any{"reason": "mutating_capability_required"},
			)))
		}
		if input.Options.SchemaValidationEnabled {
			if err := validateToolArguments(tool.InputSchema(), arguments, input.Options.RejectUnknownArguments); err != nil {
				return jsonrpc.NewResponse(input.Message.ID, buildToolSemanticErrorResult(canonicalToolName, err))
			}
		}
	}

	arguments = enrichToolCallArguments(arguments, input.Context, input.Options, progressToken, hasProgressToken)
	result, err := input.ToolManager.CallTool(canonicalToolName, arguments)
	if err != nil {
		if semanticErr, ok := tooltypes.AsSemanticError(err); ok {
			return jsonrpc.NewResponse(input.Message.ID, buildToolSemanticErrorResult(canonicalToolName, semanticErr))
		}
		if tools.IsToolNotFound(err) {
			return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
		}
		return jsonrpc.NewResponse(input.Message.ID, buildToolExecutionErrorResult(canonicalToolName))
	}

	return jsonrpc.NewResponse(input.Message.ID, BuildToolSuccessResult(canonicalToolName, result))
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

func buildToolExecutionErrorResult(toolName string) map[string]any {
	return map[string]any{
		"type":    string(mcp.TypeResult),
		"tool":    toolName,
		"content": []map[string]any{{"type": "text", "text": toolExecutionErrorMessage}},
		"isError": true,
		"error": map[string]any{
			"kind": tooltypes.SemanticKindExecutionFailed,
		},
	}
}

func buildToolSemanticErrorResult(toolName string, semanticErr *tooltypes.SemanticError) map[string]any {
	message := "Tool is temporarily unavailable"
	if semanticErr != nil && strings.TrimSpace(semanticErr.Message) != "" {
		message = strings.TrimSpace(semanticErr.Message)
	}
	errorPayload := map[string]any{}
	if semanticErr != nil {
		if strings.TrimSpace(semanticErr.Kind) != "" {
			errorPayload["kind"] = strings.TrimSpace(semanticErr.Kind)
		}
		if semanticErr.Data != nil {
			maps.Copy(errorPayload, semanticErr.Data)
		}
	}
	return map[string]any{
		"type":    string(mcp.TypeResult),
		"tool":    toolName,
		"content": []map[string]any{{"type": "text", "text": message}},
		"isError": true,
		"error":   errorPayload,
	}
}

func enrichToolCallArguments(arguments map[string]any, callContext ToolCallContext, options ToolCallOptions, progressToken any, hasProgressToken bool) map[string]any {
	enriched := make(map[string]any, len(arguments)+1)
	maps.Copy(enriched, arguments)
	context := map[string]any{
		"session_id":                  strings.TrimSpace(callContext.SessionID),
		"session_initialized":         callContext.SessionInitialized,
		"emit_progress_notifications": options.EmitProgressNotifications,
	}
	if hasProgressToken {
		context["progress_token"] = progressToken
	}
	enriched["_mcp"] = context
	return enriched
}

func extractProgressToken(meta map[string]any) (any, bool, error) {
	if len(meta) == 0 {
		return nil, false, nil
	}
	rawToken, exists := meta["progressToken"]
	if !exists {
		return nil, false, nil
	}
	if !notifications.IsValidProgressToken(rawToken) {
		return nil, false, tooltypes.NewSemanticError(tooltypes.SemanticKindInvalidParams, "Invalid progressToken in tools/call _meta", nil)
	}
	switch token := rawToken.(type) {
	case string:
		return strings.TrimSpace(token), true, nil
	default:
		return token, true, nil
	}
}

func validateToolArguments(schema mcp.InputSchema, arguments map[string]any, rejectUnknown bool) *tooltypes.SemanticError {
	missingRequired := make([]string, 0)
	for _, required := range schema.Required {
		requiredKey := strings.TrimSpace(required)
		if requiredKey == "" {
			continue
		}
		if _, ok := arguments[requiredKey]; !ok {
			missingRequired = append(missingRequired, requiredKey)
		}
	}
	if len(missingRequired) > 0 {
		sort.Strings(missingRequired)
		return tooltypes.NewSemanticError(tooltypes.SemanticKindInvalidParams, "Missing required tool arguments", map[string]any{
			"field":   "arguments",
			"problem": "missing_required_arguments",
			"missing": missingRequired,
		})
	}

	if rejectUnknown {
		unknown := make([]string, 0)
		for argName := range arguments {
			if _, ok := schema.Properties[argName]; !ok {
				unknown = append(unknown, argName)
			}
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			return tooltypes.NewSemanticError(tooltypes.SemanticKindInvalidParams, "Unknown tool arguments", map[string]any{
				"field":   "arguments",
				"problem": "unknown_arguments",
				"unknown": unknown,
			})
		}
	}

	for argName, argValue := range arguments {
		propertySchemaRaw, exists := schema.Properties[argName]
		if !exists {
			continue
		}
		propertySchema, ok := propertySchemaRaw.(map[string]any)
		if !ok {
			continue
		}
		expectedType, _ := propertySchema["type"].(string)
		expectedType = strings.ToLower(strings.TrimSpace(expectedType))
		if expectedType == "" {
			continue
		}
		if !isJSONTypeMatch(argValue, expectedType) {
			return tooltypes.NewSemanticError(tooltypes.SemanticKindInvalidParams, "Tool argument has invalid type", map[string]any{
				"field":    argName,
				"problem":  "invalid_type",
				"expected": expectedType,
				"actual":   jsonTypeName(argValue),
			})
		}
	}

	return nil
}

func isJSONTypeMatch(value any, expectedType string) bool {
	switch expectedType {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		number, ok := value.(float64)
		if !ok {
			return false
		}
		return number == float64(int64(number))
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "null":
		return value == nil
	default:
		return true
	}
}

func jsonTypeName(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		return "unknown"
	}
}
