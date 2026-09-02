package conformancefixture

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/transport/shared"
)

func TestTamperedRequestStateDoesNotDiscloseVerificationDetails(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("debug"), logger.FormatJSON); err != nil {
		t.Fatalf("initialize logger: %v", err)
	}
	fixture, err := New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	params, err := json.Marshal(map[string]any{
		"name":         "test_input_required_result_tampered_state",
		"arguments":    map[string]any{},
		"requestState": "tampered",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	response := dispatchFixture(t, fixture, jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      "tampered-state",
		Method:  "tools/call",
		Params:  params,
	}, mcpv20260728.RequestMeta{})
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("expected invalid params, got %#v", response)
	}
	if response.Error.Data != nil {
		t.Fatalf("requestState verification details must not be exposed, got %#v", response.Error.Data)
	}
}

func TestStateRequiredRetryRejectsMissingRequestState(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("debug"), logger.FormatJSON); err != nil {
		t.Fatalf("initialize logger: %v", err)
	}
	fixture, err := New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	params, err := json.Marshal(map[string]any{
		"name":      "test_input_required_result_request_state",
		"arguments": map[string]any{},
		"inputResponses": map[string]any{
			"confirm": map[string]any{"action": "accept", "content": map[string]any{"ok": true}},
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	response := dispatchFixture(t, fixture, jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "missing-state", Method: "tools/call", Params: params}, mcpv20260728.RequestMeta{})
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("expected missing requestState to be rejected, got %#v", response)
	}
}

func TestDispatchLeavesOrdinaryPromptsAndToolsToSharedProductionPaths(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
		t.Fatalf("initialize logger: %v", err)
	}
	fixture, err := New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for _, request := range []jsonrpc.Request{
		{JSONRPC: jsonrpc.Version, ID: 1, Method: "prompts/get", Params: rawJSON(t, map[string]any{"name": "test_simple_prompt"})},
		{JSONRPC: jsonrpc.Version, ID: 2, Method: "tools/call", Params: rawJSON(t, map[string]any{"name": "test_simple_text", "arguments": map[string]any{}})},
	} {
		if response, handled := fixture.Dispatch(context.Background(), request, mcpv20260728.RequestMeta{}); handled {
			t.Fatalf("%s was intercepted by fixture hook: %#v", request.Method, response)
		}
	}
}

func TestPromptRequestStateIsBoundToPromptArguments(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
		t.Fatalf("initialize logger: %v", err)
	}
	fixture, err := New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	first := dispatchFixture(t, fixture, jsonrpc.Request{
		JSONRPC: jsonrpc.Version, ID: 1, Method: "prompts/get",
		Params: rawJSON(t, map[string]any{"name": "test_input_required_result_prompt", "arguments": map[string]any{"topic": "alpha"}}),
	}, mcpv20260728.RequestMeta{})
	state := first.Result.(mcp.InputRequiredResult).RequestState

	response := dispatchFixture(t, fixture, jsonrpc.Request{
		JSONRPC: jsonrpc.Version, ID: 2, Method: "prompts/get",
		Params: rawJSON(t, map[string]any{
			"name": "test_input_required_result_prompt", "arguments": map[string]any{"topic": "beta"}, "requestState": state,
		}),
	}, mcpv20260728.RequestMeta{})
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("expected state reuse with changed prompt arguments to fail: %#v", response)
	}
}

func TestMalformedElicitationResponseDoesNotCompleteMRTRRequest(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
		t.Fatalf("initialize logger: %v", err)
	}
	fixture, err := New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	first := dispatchFixture(t, fixture, jsonrpc.Request{
		JSONRPC: jsonrpc.Version, ID: 1, Method: "tools/call",
		Params: rawJSON(t, map[string]any{"name": "test_input_required_result_request_state", "arguments": map[string]any{}}),
	}, mcpv20260728.RequestMeta{})
	state := first.Result.(mcp.InputRequiredResult).RequestState

	response := dispatchFixture(t, fixture, jsonrpc.Request{
		JSONRPC: jsonrpc.Version, ID: 2, Method: "tools/call",
		Params: rawJSON(t, map[string]any{
			"name": "test_input_required_result_request_state", "arguments": map[string]any{}, "requestState": state,
			"inputResponses": map[string]any{"confirm": map[string]any{}},
		}),
	}, mcpv20260728.RequestMeta{})
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("expected malformed elicitation response to be rejected: %#v", response)
	}
}

func TestMalformedSamplingAndRootsResponsesAreRejected(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
		t.Fatalf("initialize logger: %v", err)
	}
	fixture, err := New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, test := range []struct {
		name     string
		tool     string
		key      string
		response map[string]any
	}{
		{name: "sampling misses required content", tool: "test_input_required_result_sampling", key: "capital_question", response: map[string]any{"model": "fixture", "role": "assistant"}},
		{name: "roots contain entry without URI", tool: "test_input_required_result_list_roots", key: "client_roots", response: map[string]any{"roots": []any{map[string]any{"name": "missing-uri"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := dispatchFixture(t, fixture, jsonrpc.Request{
				JSONRPC: jsonrpc.Version, ID: test.name, Method: "tools/call",
				Params: rawJSON(t, map[string]any{
					"name": test.tool, "arguments": map[string]any{}, "inputResponses": map[string]any{test.key: test.response},
				}),
			}, mcpv20260728.RequestMeta{})
			if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInvalidParams) {
				t.Fatalf("expected malformed response to be rejected: %#v", response)
			}
		})
	}
}

func TestProgressDispatchOnlyEmitsNotificationsForValidToken(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
		t.Fatalf("initialize logger: %v", err)
	}
	fixture, err := New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	request := jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "progress", Method: "tools/call", Params: json.RawMessage(`{"name":"test_tool_with_progress","arguments":{}}`)}

	for _, test := range []struct {
		name              string
		token             any
		wantNotifications int
		wantError         bool
	}{
		{name: "missing", wantNotifications: 0},
		{name: "invalid", token: true, wantNotifications: 0, wantError: true},
		{name: "string", token: "progress-token", wantNotifications: 3},
		{name: "number", token: float64(7), wantNotifications: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			notifications, result, handled := fixture.ProgressDispatch(context.Background(), request, mcpv20260728.RequestMeta{ProgressToken: test.token})
			if !handled || len(notifications) != test.wantNotifications {
				t.Fatalf("unexpected progress dispatch: handled=%v notifications=%d result=%#v", handled, len(notifications), result)
			}
			response := result.(*jsonrpc.Response)
			if (response.Error != nil) != test.wantError {
				t.Fatalf("unexpected progress response: %#v", response)
			}
		})
	}
}

func TestStreamingElicitationCompletesAfterValidatedResponse(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
		t.Fatalf("initialize logger: %v", err)
	}
	fixture, err := New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	meta := mcpv20260728.RequestMeta{ClientCapabilities: map[string]any{"elicitation": map[string]any{}}}
	first := dispatchFixture(t, fixture, jsonrpc.Request{
		JSONRPC: jsonrpc.Version, ID: 1, Method: "tools/call",
		Params: rawJSON(t, map[string]any{"name": "test_streaming_elicitation", "arguments": map[string]any{}}),
	}, meta)
	required := first.Result.(mcp.InputRequiredResult)
	second := dispatchFixture(t, fixture, jsonrpc.Request{
		JSONRPC: jsonrpc.Version, ID: 2, Method: "tools/call",
		Params: rawJSON(t, map[string]any{
			"name": "test_streaming_elicitation", "arguments": map[string]any{}, "requestState": required.RequestState,
			"inputResponses": map[string]any{"stream": map[string]any{"action": "accept", "content": map[string]any{"value": "done"}}},
		}),
	}, meta)
	result, ok := second.Result.(map[string]any)
	if second.Error != nil || !ok || result["resultType"] != "complete" {
		t.Fatalf("streaming elicitation did not complete: %#v", second)
	}
}

func TestCapabilityFixtureCompletesWhenClientOffersNoInputCapabilities(t *testing.T) {
	if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
		t.Fatalf("initialize logger: %v", err)
	}
	fixture, err := New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	response := dispatchFixture(t, fixture, jsonrpc.Request{
		JSONRPC: jsonrpc.Version, ID: 1, Method: "tools/call",
		Params: rawJSON(t, map[string]any{"name": "test_input_required_result_capabilities", "arguments": map[string]any{}}),
	}, mcpv20260728.RequestMeta{})
	result, ok := response.Result.(map[string]any)
	if response.Error != nil || !ok || result["resultType"] != "complete" {
		t.Fatalf("empty capability set did not complete: %#v", response)
	}
}

func rawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture request: %v", err)
	}
	return raw
}

func dispatchFixture(t *testing.T, fixture *Fixture, request jsonrpc.Request, meta mcpv20260728.RequestMeta) *jsonrpc.Response {
	t.Helper()
	if request.Method == "tools/call" {
		return shared.BuildToolCallResponseWithContextAndOptions(request, fixture.manager, nil, shared.ToolCallContext{
			Context: context.Background(), Modern: true, ClientInfo: meta.ClientInfo, ClientCapabilities: meta.ClientCapabilities, RequestStateCodec: fixture.codec,
		}, shared.DefaultToolCallOptions())
	}
	value := shared.DispatchStandardMethodWithContextAndProviders(request, fixture.manager, fixture.catalog, nil, shared.DefaultPromptRenderOptions(), shared.DefaultToolCallOptions(), shared.DispatchProviders{Resources: fixture, Completion: fixture}, shared.DispatchContext{
		Context: context.Background(), RequestMeta: meta, RequestStateCodec: fixture.codec,
	})
	response, ok := value.(*jsonrpc.Response)
	if !ok {
		t.Fatalf("shared fixture dispatch returned %T", value)
	}
	return response
}
