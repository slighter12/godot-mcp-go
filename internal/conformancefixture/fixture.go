package conformancefixture

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/slighter12/godot-mcp-go/config"
	"github.com/slighter12/godot-mcp-go/internal/infra/notifications"
	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
	"github.com/slighter12/godot-mcp-go/tools"
	transporthttp "github.com/slighter12/godot-mcp-go/transport/http"
	"github.com/slighter12/godot-mcp-go/transport/shared"
)

type descriptor struct {
	name         string
	description  string
	inputSchema  mcp.InputSchema
	outputSchema map[string]any
	execute      func(json.RawMessage) (mcp.CompleteResult, error)
}

type roundTripDescriptor struct {
	descriptor
	handler func(context.Context, mcp.RoundTripRequest) (mcp.RoundTripOutcome, error)
}

func (d descriptor) ExecuteContent(args json.RawMessage) (mcp.CompleteResult, error) {
	if d.execute == nil {
		return mcp.CompleteResult{}, errors.New("fixture tool has no content handler")
	}
	return d.execute(args)
}

func (d descriptor) Name() string                 { return d.name }
func (d descriptor) Description() string          { return d.description }
func (d descriptor) InputSchema() mcp.InputSchema { return d.inputSchema }
func (d descriptor) OutputSchema() map[string]any { return d.outputSchema }
func (d descriptor) Execute(json.RawMessage) ([]byte, error) {
	return nil, errors.New("conformance fixture tools execute through shared protocol dispatch")
}

func (d roundTripDescriptor) ExecuteRoundTrip(ctx context.Context, request mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
	if d.handler == nil {
		return mcp.RoundTripOutcome{}, errors.New("fixture tool has no MRTR handler")
	}
	return d.handler(ctx, request)
}

type Fixture struct {
	manager *tools.Manager
	catalog *promptcatalog.Registry
	codec   *mcpv20260728.RequestStateCodec
}

// New constructs a hermetic conformance fixture. The supplied key may be nil,
// in which case random per-process AES-256-GCM key material is generated.
func New(key []byte) (*Fixture, error) {
	if len(key) == 0 {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("generate requestState key: %w", err)
		}
	}
	if len(key) < 32 {
		return nil, errors.New("requestState key must be at least 32 bytes")
	}

	codec, err := mcpv20260728.NewRequestStateCodec(key)
	if err != nil {
		return nil, err
	}
	fixture := &Fixture{
		manager: tools.NewManagerWithNameValidator(func(name string) bool {
			return strings.HasPrefix(name, "test_") || name == "json_schema_2020_12_tool"
		}),
		catalog: promptcatalog.NewRegistry(true),
		codec:   codec,
	}
	if err := fixture.registerTools(); err != nil {
		return nil, err
	}
	fixture.registerPrompts()
	return fixture, nil
}

// NewHTTPServer creates the opt-in loopback fixture server.
func (f *Fixture) NewHTTPServer(host string, port int) *transporthttp.Server {
	cfg := config.NewConfig()
	cfg.Server.Host = host
	cfg.Server.Port = port
	cfg.PromptCatalog.Enabled = true
	server := transporthttp.NewConformanceServer(cfg, f.manager, f.catalog, f.Dispatch, shared.DispatchProviders{Resources: f, Completion: f}, f.codec)
	server.AttachProgressDispatchHook(f.ProgressDispatch)
	return server
}

// ProgressDispatch emits deterministic progress notifications for the one
// progress fixture tool while leaving all other requests on normal dispatch.
func (f *Fixture) ProgressDispatch(_ context.Context, request jsonrpc.Request, meta mcpv20260728.RequestMeta) ([]*jsonrpc.Notification, any, bool) {
	var params struct {
		Name string `json:"name"`
	}
	if request.Method != "tools/call" || json.Unmarshal(request.Params, &params) != nil || params.Name != "test_tool_with_progress" {
		return nil, nil, false
	}
	if meta.ProgressToken == nil {
		return nil, completeText(request.ID, "Progress tool completed"), true
	}
	if !notifications.IsValidProgressToken(meta.ProgressToken) {
		return nil, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInvalidParams), "Invalid progress token", nil), true
	}
	notifications := make([]*jsonrpc.Notification, 0, 3)
	for _, progress := range []int{0, 50, 100} {
		notifications = append(notifications, jsonrpc.NewNotification("notifications/progress", map[string]any{
			"progressToken": meta.ProgressToken,
			"progress":      progress,
			"total":         100,
		}))
	}
	return notifications, completeText(request.ID, "Progress tool completed"), true
}

func objectSchema(properties map[string]any, required ...string) mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: properties, Required: required}
}

func (f *Fixture) registerTools() error {
	empty := objectSchema(map[string]any{})
	names := []string{
		"test_simple_text", "test_image_content", "test_audio_content",
		"test_embedded_resource", "test_multiple_content_types",
		"test_error_handling", "test_tool_with_progress", "test_logging_tool",
		"test_missing_capability", "test_streaming_elicitation",
		"test_input_required_result_elicitation", "test_input_required_result_sampling",
		"test_input_required_result_list_roots", "test_input_required_result_request_state",
		"test_input_required_result_multiple_inputs", "test_input_required_result_multi_round",
		"test_input_required_result_tampered_state", "test_input_required_result_capabilities",
	}
	for _, name := range names {
		tool := descriptor{name: name, description: "MCP conformance fixture", inputSchema: empty, execute: fixtureToolResult(name)}
		if name == "test_simple_text" {
			tool.outputSchema = map[string]any{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"type":    "object",
				"properties": map[string]any{
					"message": map[string]any{"type": "string"},
				},
			}
		}
		var registerErr error
		if strings.HasPrefix(name, "test_input_required_result_") || name == "test_streaming_elicitation" {
			registerErr = f.manager.RegisterTool(roundTripDescriptor{descriptor: tool, handler: f.handleRoundTripTool(name)})
		} else {
			registerErr = f.manager.RegisterTool(tool)
		}
		if registerErr != nil {
			return fmt.Errorf("register fixture tool %s: %w", name, registerErr)
		}
	}
	if err := f.manager.RegisterTool(descriptor{
		name:        "json_schema_2020_12_tool",
		description: "Tool with JSON Schema 2020-12 features",
		execute:     fixtureToolResult("json_schema_2020_12_tool"),
		inputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]any{
				"name":          map[string]any{"type": "string"},
				"address":       map[string]any{"$ref": "#/$defs/address"},
				"contactMethod": map[string]any{"type": "string", "enum": []string{"phone", "email"}},
				"phone":         map[string]any{"type": "string"},
				"email":         map[string]any{"type": "string"},
			},
			Required: []string{},
			Extras: map[string]any{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"$defs": map[string]any{"address": map[string]any{
					"$anchor": "addressDef", "type": "object",
					"properties": map[string]any{"street": map[string]any{"type": "string"}, "city": map[string]any{"type": "string"}},
				}},
				"allOf":                []any{map[string]any{"anyOf": []any{map[string]any{"required": []string{"phone"}}, map[string]any{"required": []string{"email"}}}}},
				"if":                   map[string]any{"properties": map[string]any{"contactMethod": map[string]any{"const": "phone"}}, "required": []string{"contactMethod"}},
				"then":                 map[string]any{"required": []string{"phone"}},
				"else":                 map[string]any{"required": []string{"email"}},
				"additionalProperties": false,
			},
		},
	}); err != nil {
		return fmt.Errorf("register JSON Schema fixture: %w", err)
	}
	return f.manager.RegisterTool(descriptor{
		name:        "test_header_echo",
		description: "MCP custom header validation fixture",
		inputSchema: objectSchema(map[string]any{
			"value": map[string]any{"type": "string", "x-mcp-header": "Value"},
		}, "value"),
		execute: fixtureToolResult("test_header_echo"),
	})
}

func fixtureToolResult(name string) func(json.RawMessage) (mcp.CompleteResult, error) {
	return func(json.RawMessage) (mcp.CompleteResult, error) {
		result := mcp.CompleteResult{Meta: resultMeta()}
		switch name {
		case "test_simple_text":
			result.Content = []any{mcp.TextContent{Type: "text", Text: "Simple text content"}}
			result.StructuredContent = map[string]any{"message": "Simple text content"}
		case "test_image_content":
			result.Content = []any{mcp.ImageContent{Type: "image", Data: onePixelPNG, MimeType: "image/png"}}
		case "test_audio_content":
			result.Content = []any{mcp.AudioContent{Type: "audio", Data: silentWAV, MimeType: "audio/wav"}}
		case "test_embedded_resource":
			result.Content = []any{mcp.EmbeddedResourceContent{Type: "resource", Resource: mcp.ResourceContents{URI: "test://embedded-resource", MimeType: "text/plain", Text: "Embedded resource content for testing."}}}
		case "test_multiple_content_types":
			result.Content = []any{mcp.TextContent{Type: "text", Text: "Multiple content types test:"}, mcp.ImageContent{Type: "image", Data: onePixelPNG, MimeType: "image/png"}, mcp.EmbeddedResourceContent{Type: "resource", Resource: mcp.ResourceContents{URI: "test://mixed-content-resource", MimeType: "application/json", Text: `{"test":"data","value":123}`}}}
		case "test_error_handling":
			result.Content, result.IsError = []any{mcp.TextContent{Type: "text", Text: "Intentional tool error"}}, true
		default:
			result.Content = []any{mcp.TextContent{Type: "text", Text: "Tool completed"}}
		}
		return result, nil
	}
}

func (f *Fixture) registerPrompts() {
	for _, prompt := range []promptcatalog.Prompt{
		{Name: "test_simple_prompt", Description: "Simple fixture prompt", Template: "Simple prompt", RenderMessages: func(map[string]string) ([]map[string]any, error) {
			return promptMessages(map[string]any{"type": "text", "text": "Simple prompt for conformance testing."}), nil
		}},
		{Name: "test_prompt_with_arguments", Description: "Argument fixture prompt", Arguments: []promptcatalog.PromptArgument{{Name: "arg1", Required: true}, {Name: "arg2", Required: true}}, Template: "{{arg1}} {{arg2}}", RenderMessages: func(args map[string]string) ([]map[string]any, error) {
			return promptMessages(map[string]any{"type": "text", "text": fmt.Sprintf("Prompt with arguments: arg1='%s', arg2='%s'", args["arg1"], args["arg2"])}), nil
		}},
		{Name: "test_prompt_with_embedded_resource", Description: "Resource fixture prompt", Arguments: []promptcatalog.PromptArgument{{Name: "resourceUri", Required: true}}, Template: "{{resourceUri}}", RenderMessages: func(args map[string]string) ([]map[string]any, error) {
			return promptMessages(map[string]any{"type": "resource", "resource": map[string]any{"uri": args["resourceUri"], "mimeType": "text/plain", "text": "Embedded resource content for testing."}}), nil
		}},
		{Name: "test_prompt_with_image", Description: "Image fixture prompt", Template: "image", RenderMessages: func(map[string]string) ([]map[string]any, error) {
			return promptMessages(map[string]any{"type": "image", "mimeType": "image/png", "data": onePixelPNG}), nil
		}},
		{Name: "test_input_required_result_prompt", Description: "MRTR fixture prompt", Template: "MRTR", RoundTripHandler: func(_ context.Context, request mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
			complete, err := validateResponses(request.InputResponses, []string{"prompt_input"})
			if err != nil {
				return mcp.RoundTripOutcome{}, mcpv20260728.ErrInvalidRoundTripInput
			}
			if request.Round == 1 && request.InputResponses != nil {
				return mcp.RoundTripOutcome{}, mcpv20260728.ErrInvalidRequestState
			}
			if complete {
				return mcp.RoundTripOutcome{Complete: map[string]any{
					"resultType": "complete", "_meta": resultMeta(), "messages": promptMessages(map[string]any{"type": "text", "text": "Prompt input accepted"}),
				}}, nil
			}
			return fixtureInputRequired(map[string]mcp.InputRequest{"prompt_input": elicitation("Provide prompt input", "context", "string")}, true), nil
		}},
	} {
		f.catalog.RegisterPrompt(prompt)
	}
}

// Dispatch implements the isolated conformance surface at the shared HTTP
// JSON-RPC dispatch seam.
func (f *Fixture) Dispatch(_ context.Context, request jsonrpc.Request, meta mcpv20260728.RequestMeta) (any, bool) {
	if request.Method == "tools/call" {
		var params struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(request.Params, &params) == nil && params.Name == "test_missing_capability" {
			return f.toolsCall(request, meta), true
		}
	}
	return nil, false
}

func (f *Fixture) ListResources() []map[string]any {
	return []map[string]any{
		{"uri": "test://static-text", "name": "Static text", "mimeType": "text/plain"},
		{"uri": "test://static-binary", "name": "Static binary", "mimeType": "application/octet-stream"},
	}
}

func (f *Fixture) ListResourceTemplates() []map[string]any {
	return []map[string]any{{"uriTemplate": "test://template/{id}/data", "name": "Template data", "mimeType": "text/plain"}}
}

func (f *Fixture) ReadResource(uri string) ([]map[string]any, error) {
	var content map[string]any
	switch {
	case uri == "test://static-text":
		content = map[string]any{"uri": uri, "mimeType": "text/plain", "text": "Static text resource for conformance testing."}
	case uri == "test://static-binary":
		content = map[string]any{"uri": uri, "mimeType": "application/octet-stream", "blob": base64.StdEncoding.EncodeToString([]byte{0, 1, 2, 3})}
	case strings.HasPrefix(uri, "test://template/") && strings.HasSuffix(uri, "/data"):
		content = map[string]any{"uri": uri, "mimeType": "text/plain", "text": "Template resource data for " + uri}
	default:
		return nil, errors.New("resource not found")
	}
	return []map[string]any{content}, nil
}

func (f *Fixture) Complete(request shared.CompletionRequest) ([]string, int, bool, error) {
	known := request.RefType == "ref/prompt" && request.Name == "test_prompt_with_arguments" && (request.ArgumentName == "arg1" || request.ArgumentName == "arg2")
	known = known || (request.RefType == "ref/resource" && request.URI == "test://template/{id}/data")
	if !known {
		return nil, 0, false, errors.New("unknown completion reference")
	}
	values := []string{}
	if request.ArgumentValue != "" {
		values = []string{request.ArgumentValue + "-completion"}
	}
	return values, len(values), false, nil
}

func promptMessages(content map[string]any) []map[string]any {
	return []map[string]any{{"role": "user", "content": content}}
}

func (f *Fixture) toolsCall(request jsonrpc.Request, meta mcpv20260728.RequestMeta) *jsonrpc.Response {
	var params struct {
		Name           string         `json:"name"`
		Arguments      map[string]any `json:"arguments"`
		InputResponses map[string]any `json:"inputResponses"`
		RequestState   string         `json:"requestState"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil || params.Name == "" {
		return jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInvalidParams), "Invalid tools/call payload", nil)
	}
	if params.Name == "test_missing_capability" {
		if !hasCapability(meta.ClientCapabilities, "sampling") {
			return jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrMissingRequiredClientCapability), "Missing required client capability", map[string]any{"requiredCapabilities": map[string]any{"sampling": map[string]any{}}})
		}
		return completeText(request.ID, "Tool completed")
	}
	return jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInvalidParams), "Unknown conformance hook tool", nil)
}

func completeText(id any, text string) *jsonrpc.Response {
	return shared.BuildProtocolCompleteResponse(id, mcp.CompleteResult{Meta: resultMeta(), Content: []any{mcp.TextContent{Type: "text", Text: text}}})
}

func elicitation(message, property, propertyType string) mcp.InputRequest {
	return mcp.InputRequest{Method: "elicitation/create", Params: map[string]any{
		"message":         message,
		"requestedSchema": map[string]any{"type": "object", "properties": map[string]any{property: map[string]any{"type": propertyType}}, "required": []string{property}},
	}}
}

func sampling(prompt string) mcp.InputRequest {
	return mcp.InputRequest{Method: "sampling/createMessage", Params: map[string]any{
		"messages": []map[string]any{{"role": "user", "content": map[string]any{"type": "text", "text": prompt}}}, "maxTokens": 50,
	}}
}

func roots() mcp.InputRequest {
	return mcp.InputRequest{Method: "roots/list", Params: map[string]any{}}
}

func (f *Fixture) handleRoundTripTool(name string) func(context.Context, mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
	return func(_ context.Context, request mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
		if name == "test_streaming_elicitation" {
			if !hasCapability(request.ClientCapabilities, "elicitation") {
				return mcp.RoundTripOutcome{}, errors.New("missing elicitation capability")
			}
			responsesComplete, responseErr := validateResponses(request.InputResponses, []string{"stream"})
			if responseErr != nil {
				return mcp.RoundTripOutcome{}, mcpv20260728.ErrInvalidRoundTripInput
			}
			if responsesComplete {
				return fixtureComplete("Streaming elicitation completed"), nil
			}
			return fixtureInputRequired(map[string]mcp.InputRequest{"stream": elicitation("Streaming elicitation", "value", "string")}, false), nil
		}
		if name == "test_input_required_result_capabilities" {
			requests := map[string]mcp.InputRequest{}
			if hasCapability(request.ClientCapabilities, "sampling") {
				requests["sample"] = sampling("Generate a response")
			}
			if hasCapability(request.ClientCapabilities, "elicitation") {
				requests["elicit"] = elicitation("Provide input", "value", "string")
			}
			if hasCapability(request.ClientCapabilities, "roots") {
				requests["roots"] = roots()
			}
			if len(requests) == 0 {
				return fixtureComplete("No supported input capabilities"), nil
			}
			return fixtureInputRequired(requests, false), nil
		}

		expectedKeys := []string{}
		switch name {
		case "test_input_required_result_elicitation":
			expectedKeys = []string{"user_name"}
		case "test_input_required_result_sampling":
			expectedKeys = []string{"capital_question"}
		case "test_input_required_result_list_roots":
			expectedKeys = []string{"client_roots"}
		case "test_input_required_result_request_state", "test_input_required_result_tampered_state":
			expectedKeys = []string{"confirm"}
		case "test_input_required_result_multiple_inputs":
			expectedKeys = []string{"user_name", "greeting", "client_roots"}
		case "test_input_required_result_multi_round":
			if request.Round >= 3 {
				expectedKeys = []string{"step2"}
			} else {
				expectedKeys = []string{"step1"}
			}
		}

		responsesComplete, responseErr := validateResponses(request.InputResponses, expectedKeys)
		if responseErr != nil {
			return mcp.RoundTripOutcome{}, mcpv20260728.ErrInvalidRoundTripInput
		}
		protectState := name == "test_input_required_result_request_state" || name == "test_input_required_result_multiple_inputs" || name == "test_input_required_result_multi_round" || name == "test_input_required_result_tampered_state"
		if protectState && request.Round == 1 && request.InputResponses != nil {
			return mcp.RoundTripOutcome{}, mcpv20260728.ErrInvalidRequestState
		}
		if responsesComplete {
			if name == "test_input_required_result_multi_round" && request.Round < 3 {
				return fixtureInputRequired(map[string]mcp.InputRequest{"step2": elicitation("Step 2: What is your favorite color?", "color", "string")}, true), nil
			}
			text := "MRTR input accepted"
			if name == "test_input_required_result_request_state" {
				text = "state-ok"
			}
			return fixtureComplete(text), nil
		}

		requests := map[string]mcp.InputRequest{}
		switch name {
		case "test_input_required_result_elicitation":
			requests["user_name"] = elicitation("What is your name?", "name", "string")
		case "test_input_required_result_sampling":
			requests["capital_question"] = sampling("What is the capital of France?")
		case "test_input_required_result_list_roots":
			requests["client_roots"] = roots()
		case "test_input_required_result_request_state", "test_input_required_result_tampered_state":
			requests["confirm"] = elicitation("Please confirm", "ok", "boolean")
		case "test_input_required_result_multiple_inputs":
			requests["user_name"] = elicitation("What is your name?", "name", "string")
			requests["greeting"] = sampling("Generate a greeting")
			requests["client_roots"] = roots()
		case "test_input_required_result_multi_round":
			requests["step1"] = elicitation("Step 1: What is your name?", "name", "string")
		}
		return fixtureInputRequired(requests, protectState), nil
	}
}

func fixtureInputRequired(requests map[string]mcp.InputRequest, protectState bool) mcp.RoundTripOutcome {
	return mcp.RoundTripOutcome{InputRequired: &mcp.InputRequiredSpec{InputRequests: requests, ProtectState: protectState}}
}

func fixtureComplete(text string) mcp.RoundTripOutcome {
	return mcp.RoundTripOutcome{Complete: mcp.CompleteResult{Meta: resultMeta(), Content: []any{mcp.TextContent{Type: "text", Text: text}}}}
}

func validateResponses(responses map[string]any, keys []string) (bool, error) {
	if len(keys) == 0 || responses == nil {
		return false, nil
	}
	for _, key := range keys {
		value, ok := responses[key]
		if !ok {
			return false, nil
		}
		response, ok := value.(map[string]any)
		if !ok || !validResponseForKey(key, response) {
			return false, errors.New("input response does not match requested method")
		}
	}
	return true, nil
}

func validResponseForKey(key string, response map[string]any) bool {
	switch key {
	case "capital_question", "greeting":
		model, modelOK := response["model"].(string)
		role, roleOK := response["role"].(string)
		content, contentOK := response["content"]
		return modelOK && model != "" && roleOK && (role == "assistant" || role == "user") && contentOK && content != nil
	case "client_roots":
		roots, ok := response["roots"].([]any)
		if !ok {
			return false
		}
		for _, rawRoot := range roots {
			root, ok := rawRoot.(map[string]any)
			uri, uriOK := root["uri"].(string)
			if !ok || !uriOK || uri == "" {
				return false
			}
		}
		return true
	default:
		action, ok := response["action"].(string)
		if !ok || (action != "accept" && action != "decline" && action != "cancel") {
			return false
		}
		if action != "accept" {
			return true
		}
		_, ok = response["content"].(map[string]any)
		return ok
	}
}

func hasCapability(capabilities map[string]any, name string) bool {
	value, ok := capabilities[name]
	if !ok || value == nil {
		return false
	}
	_, ok = value.(map[string]any)
	return ok
}

func missingCapability(id any, name string) *jsonrpc.Response {
	return jsonrpc.NewErrorResponse(id, int(jsonrpc.ErrMissingRequiredClientCapability), "Missing required client capability", map[string]any{"requiredCapabilities": map[string]any{name: map[string]any{}}})
}

func resultMeta() map[string]any {
	return map[string]any{"io.modelcontextprotocol/serverInfo": map[string]any{"name": "godot-mcp-go-conformance", "version": mcp.ServerVersion}}
}

const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
const silentWAV = "UklGRiQAAABXQVZFZm10IBAAAAABAAEAQB8AAEAfAAABAAgAZGF0YQAAAAA="
