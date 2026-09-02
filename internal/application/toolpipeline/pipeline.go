package toolpipeline

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/slighter12/godot-mcp-go/internal/domain/toolspec"
	"github.com/slighter12/godot-mcp-go/internal/infra/notifications"
	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/tools"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

const toolExecutionErrorMessage = "Tool execution failed"

type ToolCallContext struct {
	Context                 context.Context
	RequestID               string
	ProgressRouteKey        string
	SessionID               string
	EditorSessionID         string
	RuntimeSessionID        string
	RuntimeCommandSessionID string
	SessionInitialized      bool
	MutatingAllowed         bool
	Modern                  bool
	ClientInfo              map[string]any
	ClientCapabilities      map[string]any
	RequestStateCodec       *mcpv20260728.RequestStateCodec
	PrincipalID             string
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
	toolCall, err := mcpv20260728.DecodeToolCallParams(input.Message.Params, input.Context.Modern)
	if err != nil {
		return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInvalidParams), "Invalid tool call payload", nil)
	}

	toolName := toolCall.Name
	if toolName == "" {
		return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInvalidParams), "Tool name is required", nil)
	}
	if !strings.HasPrefix(toolName, "godot://") && !input.ToolManager.ValidToolName(toolName) {
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
			return buildToolSemanticErrorResponse(input.Message.ID, toolName, tooltypes.NewSemanticError(
				tooltypes.SemanticKindNotSupported,
				"Tool call is blocked by permission policy",
				map[string]any{"reason": "permission_denied", "permission_mode": input.Options.PermissionMode},
			), input.Context.Modern)
		}
		if input.ReadResource == nil {
			return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInvalidParams), "Resource handler is not configured", nil)
		}
		result, err := input.ReadResource(toolName)
		if err != nil {
			return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
		}
		return buildToolSuccessResponse(input.Message.ID, toolName, result, input.Context.Modern)
	}

	canonicalToolName := toolName
	tool, found := input.ToolManager.GetTool(toolName)
	if found && tool != nil {
		canonicalToolName = tool.Name()
	}
	log.Printf("godot-mcp tools/call dispatch: tool=%q canonical=%q session_id=%q known=%t",
		toolName, canonicalToolName, strings.TrimSpace(input.Context.SessionID), found)
	if !input.ToolManager.ValidToolName(canonicalToolName) {
		return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInvalidParams), "Invalid tool name", nil)
	}
	isInternalBridgeTool := toolspec.IsInternalBridgeTool(canonicalToolName)

	if found && tool != nil {
		if !isInternalBridgeTool && !toolspec.IsToolAllowed(canonicalToolName, input.Options.PermissionMode, input.Options.AllowedTools) {
			return buildToolSemanticErrorResponse(input.Message.ID, canonicalToolName, tooltypes.NewSemanticError(
				tooltypes.SemanticKindNotSupported,
				"Tool call is blocked by permission policy",
				map[string]any{"reason": "permission_denied", "permission_mode": input.Options.PermissionMode},
			), input.Context.Modern)
		}
		if toolspec.IsMutatingTool(canonicalToolName) && !input.Context.MutatingAllowed {
			if input.Context.Modern {
				return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrMissingRequiredClientCapability), "Missing required client capability", map[string]any{
					"requiredCapabilities": []string{"extensions.com.slighter12/godot-mcp.mutating"},
					"tool":                 canonicalToolName,
				})
			}
			return buildToolSemanticErrorResponse(input.Message.ID, canonicalToolName, tooltypes.NewSemanticError(
				tooltypes.SemanticKindNotSupported,
				"Mutating tools require modern Godot mutating capability",
				map[string]any{"reason": "mutating_capability_required"},
			), input.Context.Modern)
		}
		if input.Options.SchemaValidationEnabled {
			if err := validateToolArguments(tool.InputSchema(), arguments, input.Options.RejectUnknownArguments); err != nil {
				return buildToolSemanticErrorResponse(input.Message.ID, canonicalToolName, err, input.Context.Modern)
			}
		}
	}

	arguments = enrichToolCallArguments(arguments, input.Context, input.Options, progressToken, hasProgressToken)
	if input.Context.Modern && found && tool != nil {
		if roundTripTool, ok := tool.(tooltypes.MultiRoundTripTool); ok {
			binding := mcpv20260728.RequestStateBinding{Method: input.Message.Method, Identity: canonicalToolName, Parameters: toolCall.Arguments, PrincipalID: input.Context.PrincipalID}
			resultValue, executeErr := mcpv20260728.ProcessRoundTrip(input.Context.Context, input.Context.RequestStateCodec, binding, mcp.RoundTripRequest{
				Method: input.Message.Method, Name: canonicalToolName, Arguments: arguments, InputResponses: toolCall.InputResponses, InputResponsesPresent: toolCall.InputResponsesPresent,
				ClientInfo: input.Context.ClientInfo, ClientCapabilities: input.Context.ClientCapabilities, PrincipalID: input.Context.PrincipalID,
			}, toolCall.RequestState, map[string]any{"io.modelcontextprotocol/serverInfo": map[string]any{"name": "godot-mcp-go", "version": mcp.ServerVersion}}, roundTripTool.ExecuteRoundTrip)
			if executeErr != nil {
				if errors.Is(executeErr, mcpv20260728.ErrInvalidRequestState) {
					return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInvalidParams), "Invalid requestState", nil)
				}
				if errors.Is(executeErr, mcpv20260728.ErrInvalidRoundTripInput) {
					return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInvalidParams), "Invalid inputResponses", nil)
				}
				if errors.Is(executeErr, mcpv20260728.ErrInvalidRoundTripOutcome) || errors.Is(executeErr, mcpv20260728.ErrRequestStateEncoding) {
					return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInternalError), "Invalid multi round-trip result", nil)
				}
				if semanticErr, ok := tooltypes.AsSemanticError(executeErr); ok {
					return buildToolSemanticErrorResponse(input.Message.ID, canonicalToolName, semanticErr, true)
				}
				return jsonrpc.NewResponse(input.Message.ID, buildToolExecutionErrorResultForProtocol(canonicalToolName, true))
			}
			if inputRequired, ok := resultValue.(mcp.InputRequiredResult); ok {
				return jsonrpc.NewResponse(input.Message.ID, inputRequired)
			}
			normalized, normalizeErr := normalizeToolCompleteResult(resultValue)
			if normalizeErr != nil {
				return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInternalError), "Invalid tool result", nil)
			}
			return jsonrpc.NewResponse(input.Message.ID, normalized)
		}
		if contentTool, ok := tool.(tooltypes.ContentResultTool); ok {
			rawArguments, marshalErr := json.Marshal(arguments)
			if marshalErr != nil {
				return jsonrpc.NewResponse(input.Message.ID, buildToolExecutionErrorResultForProtocol(canonicalToolName, true))
			}
			result, executeErr := contentTool.ExecuteContent(rawArguments)
			if executeErr != nil {
				if semanticErr, ok := tooltypes.AsSemanticError(executeErr); ok {
					return buildToolSemanticErrorResponse(input.Message.ID, canonicalToolName, semanticErr, true)
				}
				return jsonrpc.NewResponse(input.Message.ID, buildToolExecutionErrorResultForProtocol(canonicalToolName, true))
			}
			normalized, normalizeErr := normalizeToolCompleteResult(result)
			if normalizeErr != nil {
				return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInternalError), "Invalid tool result", nil)
			}
			return jsonrpc.NewResponse(input.Message.ID, normalized)
		}
	}
	result, err := input.ToolManager.CallTool(canonicalToolName, arguments)
	if err != nil {
		if input.Context.Modern && errors.Is(err, tools.ErrToolResultTooLarge) {
			return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInternalError), "Invalid tool result", nil)
		}
		if semanticErr, ok := tooltypes.AsSemanticError(err); ok {
			return buildToolSemanticErrorResponse(input.Message.ID, canonicalToolName, semanticErr, input.Context.Modern)
		}
		if tools.IsToolNotFound(err) {
			return jsonrpc.NewErrorResponse(input.Message.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
		}
		return jsonrpc.NewResponse(input.Message.ID, buildToolExecutionErrorResultForProtocol(canonicalToolName, input.Context.Modern))
	}

	return buildToolSuccessResponse(input.Message.ID, canonicalToolName, result, input.Context.Modern)
}

func normalizeToolCompleteResult(result any) (map[string]any, error) {
	normalized, err := mcpv20260728.NormalizeMethodCompleteResult("tools/call", result, map[string]any{
		"io.modelcontextprotocol/serverInfo": map[string]any{"name": "godot-mcp-go", "version": mcp.ServerVersion},
	})
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func buildToolSuccessResponse(id any, toolName string, result any, modern bool) *jsonrpc.Response {
	built := buildToolSuccessResultForProtocol(toolName, result, modern)
	if !modern {
		return jsonrpc.NewResponse(id, built)
	}
	normalized, err := normalizeToolCompleteResult(built)
	if err != nil {
		return jsonrpc.NewErrorResponse(id, int(jsonrpc.ErrInternalError), "Invalid tool result", nil)
	}
	return jsonrpc.NewResponse(id, normalized)
}

func buildToolSuccessResultForProtocol(toolName string, result any, modern bool) map[string]any {
	if !modern {
		return BuildToolSuccessResult(toolName, result)
	}
	return map[string]any{
		"resultType":        "complete",
		"_meta":             map[string]any{"io.modelcontextprotocol/serverInfo": map[string]any{"name": "godot-mcp-go", "version": mcp.ServerVersion}},
		"content":           ToolContentFromResult(result),
		"structuredContent": result,
		"isError":           false,
	}
}

func buildToolExecutionErrorResultForProtocol(toolName string, modern bool) map[string]any {
	if !modern {
		return buildToolExecutionErrorResult(toolName)
	}
	return map[string]any{
		"resultType": "complete",
		"_meta":      map[string]any{"io.modelcontextprotocol/serverInfo": map[string]any{"name": "godot-mcp-go", "version": mcp.ServerVersion}},
		"content":    []map[string]any{{"type": "text", "text": toolExecutionErrorMessage}},
		"isError":    true,
	}
}

func buildToolSemanticErrorResultForProtocol(toolName string, semanticErr *tooltypes.SemanticError, modern bool) map[string]any {
	if !modern {
		return buildToolSemanticErrorResult(toolName, semanticErr)
	}
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
		"resultType":        "complete",
		"_meta":             map[string]any{"io.modelcontextprotocol/serverInfo": map[string]any{"name": "godot-mcp-go", "version": mcp.ServerVersion}},
		"content":           []map[string]any{{"type": "text", "text": message}},
		"structuredContent": errorPayload,
		"isError":           true,
	}
}

func buildToolSemanticErrorResponse(id any, toolName string, semanticErr *tooltypes.SemanticError, modern bool) *jsonrpc.Response {
	result := buildToolSemanticErrorResultForProtocol(toolName, semanticErr, modern)
	if !modern {
		return jsonrpc.NewResponse(id, result)
	}
	normalized, err := normalizeToolCompleteResult(result)
	if err != nil {
		return jsonrpc.NewErrorResponse(id, int(jsonrpc.ErrInternalError), "Invalid tool result", nil)
	}
	return jsonrpc.NewResponse(id, normalized)
}

func BuildToolSuccessResult(toolName string, result any) map[string]any {
	return map[string]any{
		"resultType":        "complete",
		"_meta":             map[string]any{"io.modelcontextprotocol/serverInfo": map[string]any{"name": "godot-mcp-go", "version": mcp.ServerVersion}},
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
		"resultType": "complete",
		"_meta":      map[string]any{"io.modelcontextprotocol/serverInfo": map[string]any{"name": "godot-mcp-go", "version": mcp.ServerVersion}},
		"content":    []map[string]any{{"type": "text", "text": toolExecutionErrorMessage}},
		"isError":    true,
		"type":       string(mcp.TypeResult),
		"tool":       toolName,
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
		"resultType":        "complete",
		"_meta":             map[string]any{"io.modelcontextprotocol/serverInfo": map[string]any{"name": "godot-mcp-go", "version": mcp.ServerVersion}},
		"content":           []map[string]any{{"type": "text", "text": message}},
		"isError":           true,
		"type":              string(mcp.TypeResult),
		"tool":              toolName,
		"error":             errorPayload,
		"structuredContent": errorPayload,
	}
}

func enrichToolCallArguments(arguments map[string]any, callContext ToolCallContext, options ToolCallOptions, progressToken any, hasProgressToken bool) map[string]any {
	enriched := make(map[string]any, len(arguments)+1)
	maps.Copy(enriched, arguments)
	context := map[string]any{
		"request_id":                  strings.TrimSpace(callContext.RequestID),
		"progress_route_key":          strings.TrimSpace(callContext.ProgressRouteKey),
		"session_id":                  strings.TrimSpace(callContext.SessionID),
		"editor_session_id":           strings.TrimSpace(callContext.EditorSessionID),
		"runtime_session_id":          strings.TrimSpace(callContext.RuntimeSessionID),
		"runtime_command_session_id":  strings.TrimSpace(callContext.RuntimeCommandSessionID),
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
