package shared

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
	"github.com/slighter12/godot-mcp-go/tools"
)

type roundTripTestTool struct{}

type roundTripResourceCatalog struct{}

type typedCompleteResourceCatalog struct{}

type stateOnlyResourceCatalog struct{}

type invalidContentResourceCatalog struct{}

type countingRoundTripResourceCatalog struct {
	roundTripCalls *int
	readCalls      *int
}

type countingRoundTripTool struct{ calls *int }

type unsupportedSchemaRoundTripTool struct{}

type schemaValidationRoundTripTool struct {
	name   string
	params map[string]any
}

type contractRoundTripTool struct {
	calls    *int
	received *map[string]any
}

func testElicitationParams(message, property, typeName string) map[string]any {
	return map[string]any{
		"message": message,
		"requestedSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{property: map[string]any{"type": typeName}},
			"required":   []any{property},
		},
	}
}

func (roundTripResourceCatalog) ListResources() []map[string]any         { return nil }
func (roundTripResourceCatalog) ListResourceTemplates() []map[string]any { return nil }
func (roundTripResourceCatalog) ReadResource(string) ([]map[string]any, error) {
	return nil, errors.New("round-trip path required")
}
func (roundTripResourceCatalog) ReadResourceRoundTrip(_ context.Context, request mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
	if request.Round == 1 {
		return mcp.RoundTripOutcome{InputRequired: &mcp.InputRequiredSpec{InputRequests: map[string]mcp.InputRequest{
			"access": {Method: "elicitation/create", Params: testElicitationParams("Allow resource read", "ok", "boolean")},
		}, ProtectState: true}}, nil
	}
	return mcp.RoundTripOutcome{Complete: map[string]any{"resultType": "complete", "contents": []map[string]any{{"uri": request.URI, "text": "allowed"}}}}, nil
}

func (typedCompleteResourceCatalog) ListResources() []map[string]any         { return nil }
func (typedCompleteResourceCatalog) ListResourceTemplates() []map[string]any { return nil }
func (typedCompleteResourceCatalog) ReadResource(string) ([]map[string]any, error) {
	return nil, errors.New("round-trip path required")
}
func (typedCompleteResourceCatalog) ReadResourceRoundTrip(context.Context, mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
	return mcp.RoundTripOutcome{Complete: mcp.CompleteResult{Content: []any{mcp.TextContent{Type: "text", Text: "typed"}}}}, nil
}

func (invalidContentResourceCatalog) ListResources() []map[string]any         { return nil }
func (invalidContentResourceCatalog) ListResourceTemplates() []map[string]any { return nil }
func (invalidContentResourceCatalog) ReadResource(string) ([]map[string]any, error) {
	return nil, errors.New("round-trip path required")
}
func (invalidContentResourceCatalog) ReadResourceRoundTrip(context.Context, mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
	return mcp.RoundTripOutcome{Complete: map[string]any{"contents": []any{map[string]any{"uri": "test://invalid", "blob": "not-base64", "mimeType": "image/png"}}}}, nil
}

func (c countingRoundTripResourceCatalog) ListResources() []map[string]any { return nil }
func (c countingRoundTripResourceCatalog) ListResourceTemplates() []map[string]any {
	return nil
}
func (c countingRoundTripResourceCatalog) ReadResource(string) ([]map[string]any, error) {
	*c.readCalls++
	return []map[string]any{{"uri": "test://fallback", "text": "fallback"}}, nil
}
func (c countingRoundTripResourceCatalog) ReadResourceRoundTrip(context.Context, mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
	*c.roundTripCalls++
	return mcp.RoundTripOutcome{Complete: map[string]any{"contents": []any{}}}, nil
}

func (stateOnlyResourceCatalog) ListResources() []map[string]any         { return nil }
func (stateOnlyResourceCatalog) ListResourceTemplates() []map[string]any { return nil }
func (stateOnlyResourceCatalog) ReadResource(string) ([]map[string]any, error) {
	return nil, errors.New("round-trip path required")
}
func (stateOnlyResourceCatalog) ReadResourceRoundTrip(_ context.Context, request mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
	if request.Round == 1 {
		return mcp.RoundTripOutcome{InputRequired: &mcp.InputRequiredSpec{Continuation: json.RawMessage(`{"wait":true}`)}}, nil
	}
	return mcp.RoundTripOutcome{Complete: map[string]any{"contents": []any{}}}, nil
}

func (t countingRoundTripTool) Name() string        { return "godot.test.mrtr.counting" }
func (t countingRoundTripTool) Description() string { return "Counts MRTR handler entries" }
func (t countingRoundTripTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object"}
}
func (t countingRoundTripTool) Execute(json.RawMessage) ([]byte, error) { return nil, nil }
func (t countingRoundTripTool) ExecuteRoundTrip(context.Context, mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
	*t.calls++
	return mcp.RoundTripOutcome{InputRequired: &mcp.InputRequiredSpec{InputRequests: map[string]mcp.InputRequest{
		"confirm": {Method: "elicitation/create", Params: testElicitationParams("Confirm", "ok", "boolean")},
	}}}, nil
}

func (unsupportedSchemaRoundTripTool) Name() string { return "godot.test.mrtr.unsupported-schema" }
func (unsupportedSchemaRoundTripTool) Description() string {
	return "Returns an unsupported elicitation schema"
}
func (unsupportedSchemaRoundTripTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object"}
}
func (unsupportedSchemaRoundTripTool) Execute(json.RawMessage) ([]byte, error) { return nil, nil }
func (unsupportedSchemaRoundTripTool) ExecuteRoundTrip(context.Context, mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
	request := mcp.InputRequest{Method: "elicitation/create", Params: map[string]any{
		"message": "Nested",
		"requestedSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"nested": map[string]any{"type": "object", "properties": map[string]any{}},
			},
		},
	}}
	return mcp.RoundTripOutcome{InputRequired: &mcp.InputRequiredSpec{
		InputRequests: map[string]mcp.InputRequest{"nested": request},
	}}, nil
}

func (t schemaValidationRoundTripTool) Name() string        { return t.name }
func (t schemaValidationRoundTripTool) Description() string { return "Validates elicitation responses" }
func (t schemaValidationRoundTripTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object"}
}
func (t schemaValidationRoundTripTool) Execute(json.RawMessage) ([]byte, error) { return nil, nil }
func (t schemaValidationRoundTripTool) ExecuteRoundTrip(_ context.Context, request mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
	if request.Round == 1 {
		return mcp.RoundTripOutcome{InputRequired: &mcp.InputRequiredSpec{InputRequests: map[string]mcp.InputRequest{
			"answer": {Method: "elicitation/create", Params: t.params},
		}}}, nil
	}
	return mcp.RoundTripOutcome{Complete: mcp.CompleteResult{Content: []any{}}}, nil
}

func (t contractRoundTripTool) Name() string        { return "godot.test.mrtr.contract" }
func (t contractRoundTripTool) Description() string { return "Exercises the MRTR response contract" }
func (t contractRoundTripTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object"}
}
func (t contractRoundTripTool) Execute(json.RawMessage) ([]byte, error) { return nil, nil }
func (t contractRoundTripTool) ExecuteRoundTrip(_ context.Context, request mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
	*t.calls++
	if request.Round == 1 {
		return mcp.RoundTripOutcome{InputRequired: &mcp.InputRequiredSpec{InputRequests: map[string]mcp.InputRequest{
			"confirm": {Method: "elicitation/create", Params: map[string]any{"message": "Confirm", "requestedSchema": map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}, "required": []any{"ok"}}}},
			"roots":   {Method: "roots/list", Params: map[string]any{}},
		}}}, nil
	}
	*t.received = request.InputResponses
	return mcp.RoundTripOutcome{Complete: mcp.CompleteResult{Content: []any{mcp.TextContent{Type: "text", Text: "complete"}}}}, nil
}

func (roundTripTestTool) Name() string        { return "godot.test.mrtr" }
func (roundTripTestTool) Description() string { return "MRTR shared-dispatch test tool" }
func (roundTripTestTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{"topic": map[string]any{"type": "string"}}, Required: []string{"topic"}}
}
func (roundTripTestTool) Execute(json.RawMessage) ([]byte, error) {
	return nil, errors.New("MRTR path required")
}
func (roundTripTestTool) ExecuteRoundTrip(_ context.Context, request mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
	if request.Round == 1 {
		return mcp.RoundTripOutcome{InputRequired: &mcp.InputRequiredSpec{
			InputRequests: map[string]mcp.InputRequest{"answer": {Method: "elicitation/create", Params: testElicitationParams("Answer", "value", "string")}},
			Continuation:  json.RawMessage(`{"step":"answer"}`),
		}}, nil
	}
	if _, ok := request.InputResponses["answer"]; !ok || string(request.Continuation) != `{"step":"answer"}` {
		return mcp.RoundTripOutcome{}, errors.New("missing round-trip input")
	}
	return mcp.RoundTripOutcome{Complete: mcp.CompleteResult{Content: []any{mcp.TextContent{Type: "text", Text: "accepted"}}}}, nil
}

type completionProviderFunc func(CompletionRequest) ([]string, int, bool, error)

func (f completionProviderFunc) Complete(request CompletionRequest) ([]string, int, bool, error) {
	return f(request)
}

func TestCompleteToolResultAlwaysSerializesRequiredContentArray(t *testing.T) {
	response := BuildProtocolCompleteResponse("empty-content", mcp.CompleteResult{})
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal complete response: %v", err)
	}
	if !strings.Contains(string(raw), `"content":[]`) {
		t.Fatalf("required empty content array was omitted: %s", raw)
	}
}

func TestSharedToolDispatchReentersMRTRToolWithIntegrityProtectedState(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	manager := tools.NewManager()
	if err := manager.RegisterTool(roundTripTestTool{}); err != nil {
		t.Fatalf("register MRTR tool: %v", err)
	}
	codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create requestState codec: %v", err)
	}
	callContext := ToolCallContext{
		Modern: true, RequestStateCodec: codec,
		ClientInfo: map[string]any{"name": "shared-test", "version": "1"},
	}
	first := BuildToolCallResponseWithContextAndOptions(jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "round-1", Method: "tools/call", Params: json.RawMessage(`{
		"name":"godot.test.mrtr","arguments":{"topic":"MCP"}
	}`)}, manager, nil, callContext, DefaultToolCallOptions())
	inputRequired, ok := first.Result.(mcp.InputRequiredResult)
	if first.Error != nil || !ok || inputRequired.ResultType != "input_required" || inputRequired.RequestState == "" {
		t.Fatalf("unexpected first MRTR response: %#v", first)
	}

	retryParams, _ := json.Marshal(map[string]any{
		"name": "godot.test.mrtr", "arguments": map[string]any{"topic": "MCP"},
		"inputResponses": map[string]any{"answer": map[string]any{"action": "accept", "content": map[string]any{"value": "yes"}}},
		"requestState":   inputRequired.RequestState,
	})
	second := BuildToolCallResponseWithContextAndOptions(jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "round-2", Method: "tools/call", Params: retryParams}, manager, nil, callContext, DefaultToolCallOptions())
	complete, ok := second.Result.(map[string]any)
	content, contentOK := complete["content"].([]any)
	if second.Error != nil || !ok || complete["resultType"] != "complete" || !contentOK || len(content) != 1 {
		t.Fatalf("unexpected completed MRTR response: %#v", second)
	}
}

func TestSharedDispatchPassesContextToMRTRTool(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	manager := tools.NewManager()
	if err := manager.RegisterTool(roundTripTestTool{}); err != nil {
		t.Fatalf("register MRTR tool: %v", err)
	}
	codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create request state codec: %v", err)
	}
	ctx := DispatchContext{
		Context: context.Background(),
		RequestMeta: mcpv20260728.RequestMeta{
			ProtocolVersion:    mcpv20260728.ProtocolVersion,
			ClientInfo:         map[string]any{"name": "shared-dispatch-client"},
			ClientCapabilities: map[string]any{"sampling": map[string]any{}},
		},
		RequestStateCodec: codec,
	}
	response := DispatchStandardMethodWithContextAndProviders(jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      "shared-tool-mrtr",
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"godot.test.mrtr","arguments":{"topic":"MCP"}}`),
	}, manager, nil, nil, DefaultPromptRenderOptions(), DefaultToolCallOptions(), DispatchProviders{}, ctx).(*jsonrpc.Response)
	result, ok := response.Result.(mcp.InputRequiredResult)
	if !ok || result.RequestState == "" {
		t.Fatalf("shared dispatch did not pass MRTR state context: %#v", response)
	}
}

func TestSharedDispatchDoesNotEnterMRTRHandlerWithoutStateKey(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	calls := 0
	manager := tools.NewManager()
	if err := manager.RegisterTool(countingRoundTripTool{calls: &calls}); err != nil {
		t.Fatalf("register counting MRTR tool: %v", err)
	}
	response := BuildToolCallResponseWithContextAndOptions(jsonrpc.Request{
		JSONRPC: jsonrpc.Version, ID: "missing-key", Method: "tools/call",
		Params: json.RawMessage(`{"name":"godot.test.mrtr.counting","arguments":{}}`),
	}, manager, nil, ToolCallContext{Modern: true}, DefaultToolCallOptions())
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInternalError) || calls != 0 {
		t.Fatalf("MRTR handler ran without state key: calls=%d response=%#v", calls, response)
	}
}

func TestSharedMRTRRejectsUnsupportedElicitationSchemaBeforeMintingState(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	manager := tools.NewManager()
	if err := manager.RegisterTool(unsupportedSchemaRoundTripTool{}); err != nil {
		t.Fatalf("register unsupported-schema tool: %v", err)
	}
	codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create requestState codec: %v", err)
	}
	response := DispatchStandardMethodWithContextAndProviders(jsonrpc.Request{
		JSONRPC: jsonrpc.Version, ID: "unsupported-schema", Method: "tools/call",
		Params: json.RawMessage(`{"name":"godot.test.mrtr.unsupported-schema","arguments":{}}`),
	}, manager, nil, nil, DefaultPromptRenderOptions(), DefaultToolCallOptions(), DispatchProviders{}, DispatchContext{
		RequestMeta:       mcpv20260728.RequestMeta{ProtocolVersion: mcpv20260728.ProtocolVersion},
		RequestStateCodec: codec,
	}).(*jsonrpc.Response)
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInternalError) || response.Result != nil {
		t.Fatalf("unsupported elicitation schema was minted into state: %#v", response)
	}
}

func TestSharedMRTRValidatesReleasedElicitationResponseShapes(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	tests := []struct {
		name     string
		params   map[string]any
		response map[string]any
	}{
		{name: "string constraints", params: map[string]any{"message": "Text", "requestedSchema": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string", "minLength": 2, "maxLength": 4, "format": "email"}}, "required": []any{"value"}}}, response: map[string]any{"action": "accept", "content": map[string]any{"value": "x"}}},
		{name: "numeric constraints", params: map[string]any{"message": "Number", "requestedSchema": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "integer", "minimum": 2, "maximum": 4}}, "required": []any{"value"}}}, response: map[string]any{"action": "accept", "content": map[string]any{"value": 5}}},
		{name: "single enum", params: map[string]any{"message": "Choice", "requestedSchema": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string", "enum": []any{"a", "b"}}}, "required": []any{"value"}}}, response: map[string]any{"action": "accept", "content": map[string]any{"value": "c"}}},
		{name: "multi enum", params: map[string]any{"message": "Choices", "requestedSchema": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []any{"a", "b"}}, "maxItems": 1}}, "required": []any{"value"}}}, response: map[string]any{"action": "accept", "content": map[string]any{"value": []any{"a", "b"}}}},
		{name: "URL content forbidden", params: map[string]any{"mode": "url", "message": "Open", "url": "https://example.com/approve"}, response: map[string]any{"action": "accept", "content": map[string]any{}}},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tool := schemaValidationRoundTripTool{name: fmt.Sprintf("godot.test.mrtr.schema-%d", index), params: test.params}
			manager := tools.NewManager()
			if err := manager.RegisterTool(tool); err != nil {
				t.Fatalf("register tool: %v", err)
			}
			codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
			if err != nil {
				t.Fatalf("create codec: %v", err)
			}
			context := ToolCallContext{Modern: true, RequestStateCodec: codec}
			first := BuildToolCallResponseWithContextAndOptions(jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "first", Method: "tools/call", Params: rawJSONForTest(t, map[string]any{"name": tool.name, "arguments": map[string]any{}})}, manager, nil, context, DefaultToolCallOptions())
			required := first.Result.(mcp.InputRequiredResult)
			second := BuildToolCallResponseWithContextAndOptions(jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "second", Method: "tools/call", Params: rawJSONForTest(t, map[string]any{"name": tool.name, "arguments": map[string]any{}, "requestState": required.RequestState, "inputResponses": map[string]any{"answer": test.response}})}, manager, nil, context, DefaultToolCallOptions())
			if second.Error == nil || second.Error.Code != int(jsonrpc.ErrInvalidParams) {
				t.Fatalf("invalid elicitation response was accepted: %#v", second)
			}
		})
	}
}

func TestSharedMRTRRejectsFractionalIntegerSchemaBeforeMintingState(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	tool := schemaValidationRoundTripTool{name: "godot.test.mrtr.fractional-integer", params: map[string]any{
		"message": "Integer", "requestedSchema": map[string]any{
			"type": "object", "properties": map[string]any{"value": map[string]any{"type": "integer", "default": 1.5}},
		},
	}}
	manager := tools.NewManager()
	if err := manager.RegisterTool(tool); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create codec: %v", err)
	}
	response := BuildToolCallResponseWithContextAndOptions(jsonrpc.Request{
		JSONRPC: jsonrpc.Version, ID: "fractional", Method: "tools/call", Params: rawJSONForTest(t, map[string]any{"name": tool.name, "arguments": map[string]any{}}),
	}, manager, nil, ToolCallContext{Modern: true, RequestStateCodec: codec}, DefaultToolCallOptions())
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInternalError) {
		t.Fatalf("fractional integer schema was minted into state: %#v", response)
	}
}

func TestSharedMRTRCollectsOnlyValidatedCurrentRoundResponses(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	calls := 0
	received := map[string]any(nil)
	manager := tools.NewManager()
	if err := manager.RegisterTool(contractRoundTripTool{calls: &calls, received: &received}); err != nil {
		t.Fatalf("register contract MRTR tool: %v", err)
	}
	codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create codec: %v", err)
	}
	call := func(id string, params map[string]any) *jsonrpc.Response {
		return BuildToolCallResponseWithContextAndOptions(jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: id, Method: "tools/call", Params: rawJSONForTest(t, params)}, manager, nil, ToolCallContext{Modern: true, RequestStateCodec: codec}, DefaultToolCallOptions())
	}
	first := call("contract-1", map[string]any{"name": "godot.test.mrtr.contract", "arguments": map[string]any{}})
	firstRequired := first.Result.(mcp.InputRequiredResult)
	partial := call("contract-2", map[string]any{
		"name": "godot.test.mrtr.contract", "arguments": map[string]any{}, "requestState": firstRequired.RequestState,
		"inputResponses": map[string]any{
			"confirm": map[string]any{"action": "accept", "content": map[string]any{"ok": true}},
			"extra":   map[string]any{"action": "accept", "content": map[string]any{"ignored": true}},
		},
	})
	partialRequired, ok := partial.Result.(mcp.InputRequiredResult)
	if partial.Error != nil || !ok || calls != 1 || len(partialRequired.InputRequests) != 1 || partialRequired.InputRequests["roots"].Method != "roots/list" {
		t.Fatalf("partial responses were not re-requested safely: calls=%d response=%#v", calls, partial)
	}
	complete := call("contract-3", map[string]any{
		"name": "godot.test.mrtr.contract", "arguments": map[string]any{}, "requestState": partialRequired.RequestState,
		"inputResponses": map[string]any{"roots": map[string]any{"roots": []any{map[string]any{"uri": "file:///tmp", "name": "tmp"}}}},
	})
	if complete.Error != nil || calls != 2 || len(received) != 2 || received["extra"] != nil || received["confirm"] == nil || received["roots"] == nil {
		t.Fatalf("handler did not receive the filtered complete response set: calls=%d received=%#v response=%#v", calls, received, complete)
	}
}

func TestSharedMRTRRejectsMalformedKnownAndExtraResponses(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	for _, test := range []struct {
		name      string
		responses map[string]any
		withState bool
	}{
		{name: "known schema mismatch", withState: true, responses: map[string]any{"confirm": map[string]any{"action": "accept", "content": map[string]any{"ok": "yes"}}}},
		{name: "malformed extra", withState: true, responses: map[string]any{"extra": 12345}},
		{name: "responses without state", responses: map[string]any{"confirm": map[string]any{"action": "accept", "content": map[string]any{"ok": true}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			received := map[string]any(nil)
			manager := tools.NewManager()
			if err := manager.RegisterTool(contractRoundTripTool{calls: &calls, received: &received}); err != nil {
				t.Fatalf("register contract MRTR tool: %v", err)
			}
			codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
			if err != nil {
				t.Fatalf("create codec: %v", err)
			}
			params := map[string]any{"name": "godot.test.mrtr.contract", "arguments": map[string]any{}, "inputResponses": test.responses}
			if test.withState {
				first := BuildToolCallResponseWithContextAndOptions(jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "first", Method: "tools/call", Params: rawJSONForTest(t, map[string]any{"name": "godot.test.mrtr.contract", "arguments": map[string]any{}})}, manager, nil, ToolCallContext{Modern: true, RequestStateCodec: codec}, DefaultToolCallOptions())
				params["requestState"] = first.Result.(mcp.InputRequiredResult).RequestState
			}
			response := BuildToolCallResponseWithContextAndOptions(jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: test.name, Method: "tools/call", Params: rawJSONForTest(t, params)}, manager, nil, ToolCallContext{Modern: true, RequestStateCodec: codec}, DefaultToolCallOptions())
			if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInvalidParams) || calls != boolInt(test.withState) {
				t.Fatalf("malformed responses reached handler: calls=%d response=%#v", calls, response)
			}
		})
	}
}

func TestSharedMRTRRejectsResourceExhaustingInputResponsesBeforeHandler(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	for _, test := range []struct {
		name      string
		responses string
	}{
		{name: "raw bytes", responses: `{"confirm":{"action":"accept","content":{"ok":true,"padding":"` + strings.Repeat("x", 256*1024) + `"}}}`},
		{name: "entries", responses: func() string {
			entries := make([]string, 129)
			for index := range entries {
				entries[index] = fmt.Sprintf(`"response-%d":{"action":"cancel"}`, index)
			}
			return "{" + strings.Join(entries, ",") + "}"
		}()},
		{name: "depth", responses: strings.Repeat(`{"nested":`, 33) + `true` + strings.Repeat("}", 33)},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			received := map[string]any(nil)
			manager := tools.NewManager()
			if err := manager.RegisterTool(contractRoundTripTool{calls: &calls, received: &received}); err != nil {
				t.Fatalf("register contract MRTR tool: %v", err)
			}
			codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
			if err != nil {
				t.Fatalf("create codec: %v", err)
			}
			first := BuildToolCallResponseWithContextAndOptions(jsonrpc.Request{
				JSONRPC: jsonrpc.Version, ID: "first", Method: "tools/call",
				Params: json.RawMessage(`{"name":"godot.test.mrtr.contract","arguments":{}}`),
			}, manager, nil, ToolCallContext{Modern: true, RequestStateCodec: codec}, DefaultToolCallOptions())
			state := first.Result.(mcp.InputRequiredResult).RequestState
			raw := json.RawMessage(`{"name":"godot.test.mrtr.contract","arguments":{},"requestState":` + string(rawJSONForTest(t, state)) + `,"inputResponses":` + test.responses + `}`)
			response := BuildToolCallResponseWithContextAndOptions(jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: test.name, Method: "tools/call", Params: raw}, manager, nil, ToolCallContext{Modern: true, RequestStateCodec: codec}, DefaultToolCallOptions())
			if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInvalidParams) || calls != 1 {
				t.Fatalf("resource-exhausting responses reached handler: calls=%d response=%#v", calls, response)
			}
		})
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestSharedResourceDispatchSupportsMRTRProvider(t *testing.T) {
	codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create requestState codec: %v", err)
	}
	providers := DispatchProviders{Resources: roundTripResourceCatalog{}}
	ctx := DispatchContext{Context: context.Background(), RequestStateCodec: codec, RequestMeta: mcpv20260728.RequestMeta{ClientInfo: map[string]any{"name": "resource-test"}}}
	first := DispatchStandardMethodWithContextAndProviders(jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: 1, Method: "resources/read", Params: json.RawMessage(`{"uri":"test://protected"}`)}, tools.NewManager(), nil, nil, DefaultPromptRenderOptions(), DefaultToolCallOptions(), providers, ctx).(*jsonrpc.Response)
	inputRequired, ok := first.Result.(mcp.InputRequiredResult)
	if first.Error != nil || !ok || inputRequired.RequestState == "" {
		t.Fatalf("unexpected resource MRTR response: %#v", first)
	}
	retry := map[string]any{"uri": "test://protected", "requestState": inputRequired.RequestState, "inputResponses": map[string]any{"access": map[string]any{"action": "accept", "content": map[string]any{"ok": true}}}}
	second := DispatchStandardMethodWithContextAndProviders(jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: 2, Method: "resources/read", Params: rawJSONForTest(t, retry)}, tools.NewManager(), nil, nil, DefaultPromptRenderOptions(), DefaultToolCallOptions(), providers, ctx).(*jsonrpc.Response)
	result := second.Result.(map[string]any)
	if second.Error != nil || result["resultType"] != "complete" || fmt.Sprint(result["ttlMs"]) != "0" || result["cacheScope"] != "private" {
		t.Fatalf("unexpected completed resource MRTR response: %#v", second)
	}
}

func TestSharedResourceMRTRWithoutStateCodecDoesNotEnterHandlerOrFallback(t *testing.T) {
	roundTripCalls := 0
	readCalls := 0
	response := DispatchStandardMethodWithContextAndProviders(jsonrpc.Request{
		JSONRPC: jsonrpc.Version, ID: "resource-no-codec", Method: "resources/read", Params: json.RawMessage(`{"uri":"test://protected"}`),
	}, tools.NewManager(), nil, nil, DefaultPromptRenderOptions(), DefaultToolCallOptions(), DispatchProviders{Resources: countingRoundTripResourceCatalog{
		roundTripCalls: &roundTripCalls,
		readCalls:      &readCalls,
	}}, DispatchContext{}).(*jsonrpc.Response)
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInternalError) || roundTripCalls != 0 || readCalls != 0 {
		t.Fatalf("MRTR resource used handler or fallback without codec: roundTrip=%d read=%d response=%#v", roundTripCalls, readCalls, response)
	}
}

func TestSharedPromptMRTRWithoutStateCodecDoesNotEnterHandlerOrFallback(t *testing.T) {
	roundTripCalls := 0
	renderCalls := 0
	catalog := promptcatalog.NewRegistry(true)
	catalog.RegisterPrompt(promptcatalog.Prompt{
		Name: "protected-prompt",
		RoundTripHandler: func(context.Context, mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
			roundTripCalls++
			return mcp.RoundTripOutcome{Complete: map[string]any{"messages": []any{}}}, nil
		},
		RenderMessages: func(map[string]string) ([]map[string]any, error) {
			renderCalls++
			return []map[string]any{}, nil
		},
	})
	response := DispatchStandardMethodWithContextAndProviders(jsonrpc.Request{
		JSONRPC: jsonrpc.Version, ID: "prompt-no-codec", Method: "prompts/get", Params: json.RawMessage(`{"name":"protected-prompt"}`),
	}, tools.NewManager(), catalog, nil, DefaultPromptRenderOptions(), DefaultToolCallOptions(), DispatchProviders{}, DispatchContext{}).(*jsonrpc.Response)
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInternalError) || roundTripCalls != 0 || renderCalls != 0 {
		t.Fatalf("MRTR prompt used handler or fallback without codec: roundTrip=%d render=%d response=%#v", roundTripCalls, renderCalls, response)
	}
}

func TestSharedPromptMRTRRejectsToolShapedCompleteResult(t *testing.T) {
	codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create requestState codec: %v", err)
	}
	catalog := promptcatalog.NewRegistry(true)
	catalog.RegisterPrompt(promptcatalog.Prompt{
		Name: "invalid-complete",
		RoundTripHandler: func(context.Context, mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
			return mcp.RoundTripOutcome{Complete: mcp.CompleteResult{Content: []any{mcp.TextContent{Type: "text", Text: "wrong method"}}}}, nil
		},
	})
	response := DispatchStandardMethodWithContextAndProviders(jsonrpc.Request{
		JSONRPC: jsonrpc.Version, ID: "prompt-complete", Method: "prompts/get", Params: json.RawMessage(`{"name":"invalid-complete"}`),
	}, tools.NewManager(), catalog, nil, DefaultPromptRenderOptions(), DefaultToolCallOptions(), DispatchProviders{}, DispatchContext{RequestStateCodec: codec}).(*jsonrpc.Response)
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInternalError) {
		t.Fatalf("tool-shaped prompt result crossed shared dispatch: %#v", response)
	}
}

func TestSharedResourceMRTRRejectsToolShapedCompleteResult(t *testing.T) {
	codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create requestState codec: %v", err)
	}
	response := DispatchStandardMethodWithContextAndProviders(jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      "typed-resource",
		Method:  "resources/read",
		Params:  json.RawMessage(`{"uri":"test://typed"}`),
	}, tools.NewManager(), nil, nil, DefaultPromptRenderOptions(), DefaultToolCallOptions(), DispatchProviders{Resources: typedCompleteResourceCatalog{}}, DispatchContext{RequestStateCodec: codec}).(*jsonrpc.Response)
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInternalError) {
		t.Fatalf("tool-shaped resource result crossed shared dispatch: %#v", response)
	}
}

func TestSharedResourceMRTRRejectsInvalidTypedContent(t *testing.T) {
	codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create requestState codec: %v", err)
	}
	response := DispatchStandardMethodWithContextAndProviders(jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      "invalid-resource-content",
		Method:  "resources/read",
		Params:  json.RawMessage(`{"uri":"test://invalid-content"}`),
	}, tools.NewManager(), nil, nil, DefaultPromptRenderOptions(), DefaultToolCallOptions(), DispatchProviders{Resources: invalidContentResourceCatalog{}}, DispatchContext{RequestStateCodec: codec}).(*jsonrpc.Response)
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInternalError) {
		t.Fatalf("invalid typed content crossed shared dispatch: %#v", response)
	}
}

func TestSharedResourceMRTRRejectsNullInputResponses(t *testing.T) {
	codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create requestState codec: %v", err)
	}
	providers := DispatchProviders{Resources: roundTripResourceCatalog{}}
	ctx := DispatchContext{RequestStateCodec: codec}
	first := DispatchStandardMethodWithContextAndProviders(jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: 1, Method: "resources/read", Params: json.RawMessage(`{"uri":"test://protected"}`)}, tools.NewManager(), nil, nil, DefaultPromptRenderOptions(), DefaultToolCallOptions(), providers, ctx).(*jsonrpc.Response)
	state := first.Result.(mcp.InputRequiredResult).RequestState
	second := DispatchStandardMethodWithContextAndProviders(jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: 2, Method: "resources/read", Params: rawJSONForTest(t, map[string]any{
		"uri": "test://protected", "requestState": state, "inputResponses": nil,
	})}, tools.NewManager(), nil, nil, DefaultPromptRenderOptions(), DefaultToolCallOptions(), providers, ctx).(*jsonrpc.Response)
	if second.Error == nil || second.Error.Code != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("null inputResponses was not rejected: %#v", second)
	}
}

func TestSharedResourceMRTRAllowsStateOnlyContinuation(t *testing.T) {
	codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create requestState codec: %v", err)
	}
	providers := DispatchProviders{Resources: stateOnlyResourceCatalog{}}
	ctx := DispatchContext{RequestStateCodec: codec}
	first := DispatchStandardMethodWithContextAndProviders(jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: 1, Method: "resources/read", Params: json.RawMessage(`{"uri":"test://state-only"}`)}, tools.NewManager(), nil, nil, DefaultPromptRenderOptions(), DefaultToolCallOptions(), providers, ctx).(*jsonrpc.Response)
	required, ok := first.Result.(mcp.InputRequiredResult)
	if first.Error != nil || !ok || required.RequestState == "" || len(required.InputRequests) != 0 {
		t.Fatalf("state-only continuation was rejected: %#v", first)
	}
	second := DispatchStandardMethodWithContextAndProviders(jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: 2, Method: "resources/read", Params: rawJSONForTest(t, map[string]any{
		"uri": "test://state-only", "requestState": required.RequestState,
	})}, tools.NewManager(), nil, nil, DefaultPromptRenderOptions(), DefaultToolCallOptions(), providers, ctx).(*jsonrpc.Response)
	if second.Error != nil || second.Result.(map[string]any)["resultType"] != "complete" {
		t.Fatalf("state-only continuation did not complete: %#v", second)
	}
}

func rawJSONForTest(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return raw
}

func TestDispatchCompletionReturnsSuccessfulEmptyCandidatesFromProvider(t *testing.T) {
	request := jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "completion-empty", Method: "completion/complete", Params: json.RawMessage(`{
		"ref":{"type":"ref/prompt","name":"fixture"},
		"argument":{"name":"value","value":""}
	}`)}
	response := DispatchStandardMethodWithProviders(request, tools.NewManager(), nil, nil, DefaultPromptRenderOptions(), DefaultToolCallOptions(), DispatchProviders{
		Completion: completionProviderFunc(func(got CompletionRequest) ([]string, int, bool, error) {
			if got.RefType != "ref/prompt" || got.Name != "fixture" || got.ArgumentName != "value" {
				t.Fatalf("unexpected provider request: %#v", got)
			}
			return []string{}, 0, false, nil
		}),
	}).(*jsonrpc.Response)
	if response.Error != nil {
		t.Fatalf("unexpected completion error: %#v", response.Error)
	}
	result := response.Result.(map[string]any)
	completion := result["completion"].(map[string]any)
	if values := completion["values"].([]string); len(values) != 0 || completion["total"] != 0 || completion["hasMore"] != false {
		t.Fatalf("unexpected empty completion result: %#v", completion)
	}
}

func TestDispatchCompletionRejectsReferenceWithWrongIdentityField(t *testing.T) {
	called := false
	request := jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "completion-invalid-ref", Method: "completion/complete", Params: json.RawMessage(`{
		"ref":{"type":"ref/prompt","uri":"test://wrong-field"},
		"argument":{"name":"value","value":"x"}
	}`)}
	response := DispatchStandardMethodWithProviders(request, tools.NewManager(), nil, nil, DefaultPromptRenderOptions(), DefaultToolCallOptions(), DispatchProviders{
		Completion: completionProviderFunc(func(CompletionRequest) ([]string, int, bool, error) {
			called = true
			return []string{"unexpected"}, 1, false, nil
		}),
	}).(*jsonrpc.Response)
	if called || response.Error == nil || response.Error.Code != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("expected malformed reference to fail before provider: called=%t response=%#v", called, response)
	}
}

func TestDispatchCompletionCapsCandidatesAtOneHundred(t *testing.T) {
	values := make([]string, 101)
	for index := range values {
		values[index] = fmt.Sprintf("value-%03d", index)
	}
	request := jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "completion-cap", Method: "completion/complete", Params: json.RawMessage(`{
		"ref":{"type":"ref/resource","uri":"test://template/{id}"},
		"argument":{"name":"id","value":"value"}
	}`)}
	response := DispatchStandardMethodWithProviders(request, tools.NewManager(), nil, nil, DefaultPromptRenderOptions(), DefaultToolCallOptions(), DispatchProviders{
		Completion: completionProviderFunc(func(CompletionRequest) ([]string, int, bool, error) { return values, 101, false, nil }),
	}).(*jsonrpc.Response)
	completion := response.Result.(map[string]any)["completion"].(map[string]any)
	if got := completion["values"].([]string); len(got) != 100 || completion["total"] != 101 || completion["hasMore"] != true {
		t.Fatalf("unexpected capped completion: %#v", completion)
	}
}

func TestDispatchCompletionMapsUnknownReferenceToInvalidParams(t *testing.T) {
	request := jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "completion-unknown", Method: "completion/complete", Params: json.RawMessage(`{
		"ref":{"type":"ref/prompt","name":"missing"},
		"argument":{"name":"value","value":""}
	}`)}
	response := DispatchStandardMethodWithProviders(request, tools.NewManager(), nil, nil, DefaultPromptRenderOptions(), DefaultToolCallOptions(), DispatchProviders{
		Completion: completionProviderFunc(func(CompletionRequest) ([]string, int, bool, error) {
			return nil, 0, false, errors.New("private provider detail")
		}),
	}).(*jsonrpc.Response)
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInvalidParams) || strings.Contains(response.Error.Message, "private") {
		t.Fatalf("unexpected unknown completion response: %#v", response)
	}
}

func TestParseJSONRPCFramePreservesRequestIDRepresentation(t *testing.T) {
	tests := []struct {
		name  string
		id    any
		frame string
	}{
		{
			name:  "string id",
			id:    "1",
			frame: `{"jsonrpc":"2.0","id":"1","method":"tools/list","params":{}}`,
		},
		{
			name:  "large integer id",
			id:    json.Number("9007199254740993"),
			frame: `{"jsonrpc":"2.0","id":9007199254740993,"method":"tools/list","params":{}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests, responses, acceptedOneWay, err := ParseJSONRPCFrame([]byte(test.frame))
			if err != nil {
				t.Fatalf("parse frame: %v", err)
			}
			if len(responses) != 0 || acceptedOneWay {
				t.Fatalf("unexpected prebuilt response state: responses=%#v acceptedOneWay=%t", responses, acceptedOneWay)
			}
			if len(requests) != 1 {
				t.Fatalf("expected one request, got %d", len(requests))
			}
			if !reflect.DeepEqual(requests[0].ID, test.id) {
				t.Fatalf("expected request ID %#v (%T), got %#v (%T)", test.id, test.id, requests[0].ID, requests[0].ID)
			}
		})
	}
}

func TestDispatchResourcesTemplatesListReturnsCacheableEmptyProductionCatalog(t *testing.T) {
	response, ok := DispatchStandardMethod(jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      "templates",
		Method:  "resources/templates/list",
		Params:  json.RawMessage(`{}`),
	}, tools.NewManager(), nil, nil).(*jsonrpc.Response)
	if !ok {
		t.Fatal("expected JSON-RPC response")
	}
	if response.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %#v", response.Error)
	}

	result, ok := response.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got %T", response.Result)
	}
	if result["resultType"] != "complete" || result["cacheScope"] != "public" || result["ttlMs"] != int64(0) {
		t.Fatalf("unexpected metadata: %#v", result)
	}
	templates, ok := result["resourceTemplates"].([]map[string]any)
	if !ok || len(templates) != 0 {
		t.Fatalf("expected empty resourceTemplates, got %#v", result["resourceTemplates"])
	}
}

func TestDispatchResourcesTemplatesListRejectsUnsupportedCursor(t *testing.T) {
	response := DispatchStandardMethod(jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      "templates-cursor",
		Method:  "resources/templates/list",
		Params:  json.RawMessage(`{"cursor":"1"}`),
	}, tools.NewManager(), nil, nil).(*jsonrpc.Response)

	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("expected invalid params for unsupported cursor, got %#v", response.Error)
	}
}

func TestResourcesReadNotFoundIncludesRequestedURI(t *testing.T) {
	request := jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: 7, Method: "resources/read", Params: json.RawMessage(`{"uri":"godot://missing"}`)}
	response := BuildResourcesReadResponse(request, func(string) (any, error) {
		return nil, errors.New("resource not found")
	})
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("expected invalid params resource error, got %#v", response)
	}
	data, ok := response.Error.Data.(map[string]any)
	if !ok || data["uri"] != "godot://missing" {
		t.Fatalf("expected requested URI in error data, got %#v", response.Error.Data)
	}
}
