package toolpipeline

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/tools"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

type contentResultTestTool struct{}

type oversizedOrdinaryTool struct{}

type oversizedSemanticErrorTool struct{}

type roundTripSemanticErrorTool struct {
	failRound int
}

type roundTripMapCompleteTool struct {
	completeRound int
	complete      any
}

type invalidContentResultTool struct {
	content    []any
	resultType string
	structured any
}

func (oversizedOrdinaryTool) Name() string        { return "godot.test.ordinary.oversized" }
func (oversizedOrdinaryTool) Description() string { return "Returns an oversized ordinary result" }
func (oversizedOrdinaryTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object"}
}
func (oversizedOrdinaryTool) Execute(json.RawMessage) ([]byte, error) {
	return []byte(`{"payload":"` + strings.Repeat("x", mcpv20260728.MaxDecodedContentBlockBytes) + `"}`), nil
}

func (oversizedSemanticErrorTool) Name() string { return "godot.test.semantic.oversized" }
func (oversizedSemanticErrorTool) Description() string {
	return "Returns oversized semantic error data"
}
func (oversizedSemanticErrorTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object"}
}
func (oversizedSemanticErrorTool) Execute(json.RawMessage) ([]byte, error) {
	return nil, tooltypes.NewSemanticError(tooltypes.SemanticKindNotAvailable, "Unavailable", map[string]any{
		"payload": strings.Repeat("x", mcpv20260728.MaxSerializedCompleteBytes),
	})
}

func (t roundTripSemanticErrorTool) Name() string { return "godot.test.mrtr.semantic-error" }
func (t roundTripSemanticErrorTool) Description() string {
	return "Returns a semantic error from an MRTR handler"
}
func (t roundTripSemanticErrorTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object"}
}
func (t roundTripSemanticErrorTool) Execute(json.RawMessage) ([]byte, error) { return nil, nil }
func (t roundTripSemanticErrorTool) ExecuteRoundTrip(_ context.Context, request mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
	if request.Round == t.failRound {
		return mcp.RoundTripOutcome{}, tooltypes.NewSemanticError(
			tooltypes.SemanticKindNotAvailable,
			"MRTR dependency is unavailable",
			map[string]any{"reason": "mrtr_dependency_unavailable"},
		)
	}
	return mcp.RoundTripOutcome{InputRequired: &mcp.InputRequiredSpec{InputRequests: map[string]mcp.InputRequest{
		"confirm": {Method: "elicitation/create", Params: map[string]any{
			"message": "Continue?",
			"requestedSchema": map[string]any{
				"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}, "required": []any{"ok"},
			},
		}},
	}}}, nil
}

func (t roundTripMapCompleteTool) Name() string { return "godot.test.mrtr.map-complete" }
func (t roundTripMapCompleteTool) Description() string {
	return "Returns a map-shaped complete result from an MRTR handler"
}
func (t roundTripMapCompleteTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object"}
}
func (t roundTripMapCompleteTool) Execute(json.RawMessage) ([]byte, error) { return nil, nil }
func (t roundTripMapCompleteTool) ExecuteRoundTrip(_ context.Context, request mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
	if request.Round == t.completeRound {
		complete := t.complete
		if complete == nil {
			complete = map[string]any{
				"content":           []any{mcp.TextContent{Type: "text", Text: "map complete"}},
				"structuredContent": map[string]any{"shape": "map"},
			}
		}
		return mcp.RoundTripOutcome{Complete: complete}, nil
	}
	return mcp.RoundTripOutcome{InputRequired: &mcp.InputRequiredSpec{InputRequests: map[string]mcp.InputRequest{
		"confirm": {Method: "elicitation/create", Params: map[string]any{
			"message": "Continue?",
			"requestedSchema": map[string]any{
				"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}, "required": []any{"ok"},
			},
		}},
	}}}, nil
}

func TestExecuteAcceptsFirstRoundMapShapedMRTRCompleteResult(t *testing.T) {
	manager := tools.NewManager()
	if err := manager.RegisterTool(roundTripMapCompleteTool{completeRound: 1}); err != nil {
		t.Fatalf("register map-shaped MRTR tool: %v", err)
	}
	codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create requestState codec: %v", err)
	}
	response := Execute(ExecuteInput{
		Message: jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "mrtr-map-first", Method: "tools/call", Params: mustMarshalParams(t, map[string]any{
			"name": "godot.test.mrtr.map-complete", "arguments": map[string]any{},
		})},
		ToolManager: manager,
		Context:     ToolCallContext{Modern: true, RequestStateCodec: codec},
		Options:     ToolCallOptions{SchemaValidationEnabled: true, PermissionMode: "allow_all"},
	})
	assertMapShapedMRTRCompleteResult(t, response)
}

func TestExecuteAcceptsRetryRoundMapShapedMRTRCompleteResult(t *testing.T) {
	manager := tools.NewManager()
	if err := manager.RegisterTool(roundTripMapCompleteTool{completeRound: 2}); err != nil {
		t.Fatalf("register map-shaped MRTR tool: %v", err)
	}
	codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create requestState codec: %v", err)
	}
	callContext := ToolCallContext{Modern: true, RequestStateCodec: codec}
	options := ToolCallOptions{SchemaValidationEnabled: true, PermissionMode: "allow_all"}
	first := Execute(ExecuteInput{
		Message: jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "mrtr-map-round-1", Method: "tools/call", Params: mustMarshalParams(t, map[string]any{
			"name": "godot.test.mrtr.map-complete", "arguments": map[string]any{},
		})},
		ToolManager: manager, Context: callContext, Options: options,
	})
	inputRequired, ok := first.Result.(mcp.InputRequiredResult)
	if first.Error != nil || !ok || inputRequired.RequestState == "" {
		t.Fatalf("unexpected first MRTR response: %#v", first)
	}
	retry := Execute(ExecuteInput{
		Message: jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "mrtr-map-round-2", Method: "tools/call", Params: mustMarshalParams(t, map[string]any{
			"name": "godot.test.mrtr.map-complete", "arguments": map[string]any{}, "requestState": inputRequired.RequestState,
			"inputResponses": map[string]any{"confirm": map[string]any{"action": "accept", "content": map[string]any{"ok": true}}},
		})},
		ToolManager: manager, Context: callContext, Options: options,
	})
	if retry.ID != "mrtr-map-round-2" {
		t.Fatalf("retry response did not retain fresh request ID: %#v", retry.ID)
	}
	assertMapShapedMRTRCompleteResult(t, retry)
}

func TestExecuteRejectsNonObjectMRTRCompleteResult(t *testing.T) {
	manager := tools.NewManager()
	if err := manager.RegisterTool(roundTripMapCompleteTool{completeRound: 1, complete: "not an object"}); err != nil {
		t.Fatalf("register invalid MRTR complete tool: %v", err)
	}
	codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create requestState codec: %v", err)
	}
	response := Execute(ExecuteInput{
		Message: jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "mrtr-invalid-complete", Method: "tools/call", Params: mustMarshalParams(t, map[string]any{
			"name": "godot.test.mrtr.map-complete", "arguments": map[string]any{},
		})},
		ToolManager: manager,
		Context:     ToolCallContext{Modern: true, RequestStateCodec: codec},
		Options:     ToolCallOptions{SchemaValidationEnabled: true, PermissionMode: "allow_all"},
	})
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInternalError) || response.Error.Message != "Invalid tool result" {
		t.Fatalf("non-object MRTR result crossed normalization: %#v", response)
	}
}

func assertMapShapedMRTRCompleteResult(t *testing.T, response *jsonrpc.Response) {
	t.Helper()
	if response.Error != nil {
		t.Fatalf("map-shaped complete result returned JSON-RPC error: %#v", response.Error)
	}
	result := mustMap(t, response.Result)
	if result["resultType"] != "complete" || result["_meta"] == nil {
		t.Fatalf("map-shaped result was not normalized: %#v", result)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) != 1 || mustMap(t, content[0])["text"] != "map complete" {
		t.Fatalf("map-shaped content was not preserved: %#v", result["content"])
	}
	if structured := mustMap(t, result["structuredContent"]); structured["shape"] != "map" {
		t.Fatalf("map-shaped structured content was not preserved: %#v", structured)
	}
}

func TestExecutePreservesFirstRoundMRTRSemanticError(t *testing.T) {
	manager := tools.NewManager()
	if err := manager.RegisterTool(roundTripSemanticErrorTool{failRound: 1}); err != nil {
		t.Fatalf("register MRTR semantic error tool: %v", err)
	}
	codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create requestState codec: %v", err)
	}
	response := Execute(ExecuteInput{
		Message: jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "mrtr-semantic-first", Method: "tools/call", Params: mustMarshalParams(t, map[string]any{
			"name": "godot.test.mrtr.semantic-error", "arguments": map[string]any{},
		})},
		ToolManager: manager,
		Context:     ToolCallContext{Modern: true, RequestStateCodec: codec},
		Options:     ToolCallOptions{SchemaValidationEnabled: true, PermissionMode: "allow_all"},
	})
	assertMRTRSemanticErrorResult(t, response)
}

func TestExecutePreservesRetryRoundMRTRSemanticError(t *testing.T) {
	manager := tools.NewManager()
	if err := manager.RegisterTool(roundTripSemanticErrorTool{failRound: 2}); err != nil {
		t.Fatalf("register MRTR semantic error tool: %v", err)
	}
	codec, err := mcpv20260728.NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create requestState codec: %v", err)
	}
	callContext := ToolCallContext{Modern: true, RequestStateCodec: codec}
	options := ToolCallOptions{SchemaValidationEnabled: true, PermissionMode: "allow_all"}
	first := Execute(ExecuteInput{
		Message: jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "mrtr-semantic-round-1", Method: "tools/call", Params: mustMarshalParams(t, map[string]any{
			"name": "godot.test.mrtr.semantic-error", "arguments": map[string]any{},
		})},
		ToolManager: manager, Context: callContext, Options: options,
	})
	inputRequired, ok := first.Result.(mcp.InputRequiredResult)
	if first.Error != nil || !ok || inputRequired.RequestState == "" {
		t.Fatalf("unexpected first MRTR response: %#v", first)
	}
	retry := Execute(ExecuteInput{
		Message: jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "mrtr-semantic-round-2", Method: "tools/call", Params: mustMarshalParams(t, map[string]any{
			"name": "godot.test.mrtr.semantic-error", "arguments": map[string]any{}, "requestState": inputRequired.RequestState,
			"inputResponses": map[string]any{"confirm": map[string]any{"action": "accept", "content": map[string]any{"ok": true}}},
		})},
		ToolManager: manager, Context: callContext, Options: options,
	})
	if retry.ID != "mrtr-semantic-round-2" {
		t.Fatalf("retry response did not retain fresh request ID: %#v", retry.ID)
	}
	assertMRTRSemanticErrorResult(t, retry)
}

func assertMRTRSemanticErrorResult(t *testing.T, response *jsonrpc.Response) {
	t.Helper()
	if response.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %#v", response.Error)
	}
	result := mustMap(t, response.Result)
	if result["isError"] != true {
		t.Fatalf("semantic error lost isError marker: %#v", result)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) != 1 || mustMap(t, content[0])["text"] != "MRTR dependency is unavailable" {
		t.Fatalf("semantic error message was not preserved: %#v", result["content"])
	}
	structured := mustMap(t, result["structuredContent"])
	if structured["kind"] != tooltypes.SemanticKindNotAvailable || structured["reason"] != "mrtr_dependency_unavailable" {
		t.Fatalf("semantic error data was not preserved: %#v", structured)
	}
}

func (invalidContentResultTool) Name() string        { return "godot.test.content.invalid" }
func (invalidContentResultTool) Description() string { return "Returns invalid MCP content" }
func (invalidContentResultTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object"}
}
func (invalidContentResultTool) Execute(json.RawMessage) ([]byte, error) { return nil, nil }
func (t invalidContentResultTool) ExecuteContent(json.RawMessage) (mcp.CompleteResult, error) {
	return mcp.CompleteResult{ResultType: t.resultType, Content: t.content, StructuredContent: t.structured}, nil
}

func (contentResultTestTool) Name() string        { return "godot.test.content.result" }
func (contentResultTestTool) Description() string { return "Returns standard MCP content" }
func (contentResultTestTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object"}
}
func (contentResultTestTool) Execute(json.RawMessage) ([]byte, error) {
	return []byte(`{"legacy":true}`), nil
}
func (contentResultTestTool) ExecuteContent(json.RawMessage) (mcp.CompleteResult, error) {
	return mcp.CompleteResult{
		Content: []any{
			mcp.TextContent{Type: "text", Text: "hello"},
			mcp.ImageContent{Type: "image", Data: "aW1hZ2U=", MimeType: "image/png"},
			mcp.AudioContent{Type: "audio", Data: "YXVkaW8=", MimeType: "audio/wav"},
			mcp.EmbeddedResourceContent{Type: "resource", Resource: mcp.ResourceContents{URI: "test://embedded", Text: "body"}},
			mcp.ResourceLinkContent{Type: "resource_link", URI: "test://linked", Name: "linked"},
		},
		StructuredContent: map[string]any{"preserved": true},
	}, nil
}

func TestMain(m *testing.M) {
	_ = logger.Init(logger.GetLevelFromString("debug"), logger.FormatJSON)
	os.Exit(m.Run())
}

func TestExecute_AllowsRuntimeSyncInReadOnlyMode(t *testing.T) {
	manager := tools.NewManager()
	manager.RegisterDefaultTools()

	params := mustMarshalParams(t, map[string]any{
		"name": "godot.bridge.editor.sync",
		"arguments": map[string]any{
			"snapshot": map[string]any{
				"root_summary": map[string]any{"active_scene": "res://Main.tscn"},
				"scene_tree":   map[string]any{"path": "/Root", "name": "Root", "type": "Node2D", "child_count": 0},
				"node_details": map[string]any{
					"/Root": map[string]any{"path": "/Root", "name": "Root", "type": "Node2D", "child_count": 0},
				},
			},
		},
	})

	resp := Execute(ExecuteInput{
		Message: jsonrpc.Request{
			JSONRPC: jsonrpc.Version,
			ID:      "sync-read-only",
			Method:  "tools/call",
			Params:  params,
		},
		ToolManager: manager,
		Context: ToolCallContext{
			SessionID:          "session-read-only",
			SessionInitialized: true,
			MutatingAllowed:    false,
		},
		Options: ToolCallOptions{
			SchemaValidationEnabled: true,
			PermissionMode:          "read_only",
		},
	})

	if resp.Error != nil {
		t.Fatalf("expected JSON-RPC success, got %+v", resp.Error)
	}
	result := mustMap(t, resp.Result)
	if result["isError"] != false {
		t.Fatalf("expected runtime sync success, got isError=%v", result["isError"])
	}
}

func TestExecute_AllowsRuntimeAckInAllowListMode(t *testing.T) {
	manager := tools.NewManager()
	manager.RegisterDefaultTools()

	params := mustMarshalParams(t, map[string]any{
		"name": "godot.bridge.command.ack",
		"arguments": map[string]any{
			"command_id": "cmd-missing",
			"success":    true,
			"result":     map[string]any{},
		},
	})

	resp := Execute(ExecuteInput{
		Message: jsonrpc.Request{
			JSONRPC: jsonrpc.Version,
			ID:      "ack-allow-list",
			Method:  "tools/call",
			Params:  params,
		},
		ToolManager: manager,
		Context: ToolCallContext{
			SessionID:          "session-allow-list",
			SessionInitialized: true,
			MutatingAllowed:    false,
		},
		Options: ToolCallOptions{
			SchemaValidationEnabled: true,
			PermissionMode:          "allow_list",
			AllowedTools:            []string{"godot.editor.state.get"},
		},
	})

	if resp.Error != nil {
		t.Fatalf("expected JSON-RPC success, got %+v", resp.Error)
	}
	result := mustMap(t, resp.Result)
	if result["isError"] != true {
		t.Fatalf("expected runtime ack semantic error envelope, got isError=%v", result["isError"])
	}
	errPayload := mustMap(t, result["error"])
	if errPayload["kind"] != tooltypes.SemanticKindNotAvailable {
		t.Fatalf("expected kind %q, got %v", tooltypes.SemanticKindNotAvailable, errPayload["kind"])
	}
	if errPayload["reason"] != "unknown_or_expired_command" {
		t.Fatalf("expected reason unknown_or_expired_command, got %v", errPayload["reason"])
	}
}

func TestExecute_MutatingToolRequiresMutatingCapability(t *testing.T) {
	manager := tools.NewManager()
	manager.RegisterDefaultTools()

	params := mustMarshalParams(t, map[string]any{
		"name":      "godot.project.run",
		"arguments": map[string]any{},
	})

	resp := Execute(ExecuteInput{
		Message: jsonrpc.Request{
			JSONRPC: jsonrpc.Version,
			ID:      "run-mutating-false",
			Method:  "tools/call",
			Params:  params,
		},
		ToolManager: manager,
		Context: ToolCallContext{
			SessionID:          "session-mutating-false",
			SessionInitialized: true,
			MutatingAllowed:    false,
		},
		Options: ToolCallOptions{
			SchemaValidationEnabled: true,
			PermissionMode:          "allow_all",
		},
	})

	if resp.Error != nil {
		t.Fatalf("expected JSON-RPC success, got %+v", resp.Error)
	}
	result := mustMap(t, resp.Result)
	if result["isError"] != true {
		t.Fatalf("expected mutating capability semantic error, got isError=%v", result["isError"])
	}
	errPayload := mustMap(t, result["error"])
	if errPayload["kind"] != tooltypes.SemanticKindNotSupported {
		t.Fatalf("expected kind %q, got %v", tooltypes.SemanticKindNotSupported, errPayload["kind"])
	}
	if errPayload["reason"] != "mutating_capability_required" {
		t.Fatalf("expected reason mutating_capability_required, got %v", errPayload["reason"])
	}
}

func TestExecutePreservesStandardContentResultBlocks(t *testing.T) {
	manager := tools.NewManagerWithNameValidator(func(string) bool { return true })
	if err := manager.RegisterTool(contentResultTestTool{}); err != nil {
		t.Fatalf("register content result tool: %v", err)
	}

	resp := Execute(ExecuteInput{
		Message: jsonrpc.Request{
			JSONRPC: jsonrpc.Version,
			ID:      "content-result",
			Method:  "tools/call",
			Params:  mustMarshalParams(t, map[string]any{"name": "godot.test.content.result", "arguments": map[string]any{}}),
		},
		ToolManager: manager,
		Context:     ToolCallContext{Modern: true},
		Options:     ToolCallOptions{SchemaValidationEnabled: true, PermissionMode: "allow_all"},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %+v", resp.Error)
	}
	raw, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) != 5 {
		t.Fatalf("expected five standard content blocks, got %#v", result["content"])
	}
	wantTypes := []string{"text", "image", "audio", "resource", "resource_link"}
	for i, want := range wantTypes {
		block := mustMap(t, content[i])
		if block["type"] != want {
			t.Fatalf("content[%d] type: want %q, got %#v", i, want, block["type"])
		}
	}
	structured := mustMap(t, result["structuredContent"])
	if structured["preserved"] != true {
		t.Fatalf("structured content was not preserved: %#v", structured)
	}
}

func TestExecuteRejectsInvalidBase64ContentAsProtocolError(t *testing.T) {
	manager := tools.NewManagerWithNameValidator(func(string) bool { return true })
	if err := manager.RegisterTool(invalidContentResultTool{content: []any{mcp.ImageContent{Type: "image", Data: "not base64", MimeType: "image/png"}}}); err != nil {
		t.Fatalf("register invalid content tool: %v", err)
	}
	response := Execute(ExecuteInput{
		Message:     jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "invalid-content", Method: "tools/call", Params: mustMarshalParams(t, map[string]any{"name": "godot.test.content.invalid", "arguments": map[string]any{}})},
		ToolManager: manager, Context: ToolCallContext{Modern: true}, Options: ToolCallOptions{SchemaValidationEnabled: true, PermissionMode: "allow_all"},
	})
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInternalError) || response.Error.Data != nil {
		t.Fatalf("expected generic internal protocol error, got %#v", response)
	}
}

func TestExecuteRejectsNonCompleteResultType(t *testing.T) {
	manager := tools.NewManagerWithNameValidator(func(string) bool { return true })
	if err := manager.RegisterTool(invalidContentResultTool{resultType: "input_required", content: []any{mcp.TextContent{Type: "text", Text: "wrong discriminator"}}}); err != nil {
		t.Fatalf("register invalid result tool: %v", err)
	}
	response := Execute(ExecuteInput{
		Message:     jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "invalid-result-type", Method: "tools/call", Params: mustMarshalParams(t, map[string]any{"name": "godot.test.content.invalid", "arguments": map[string]any{}})},
		ToolManager: manager, Context: ToolCallContext{Modern: true}, Options: ToolCallOptions{SchemaValidationEnabled: true, PermissionMode: "allow_all"},
	})
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInternalError) {
		t.Fatalf("expected non-complete discriminator to be rejected, got %#v", response)
	}
}

func TestExecuteRejectsOversizedOrdinaryToolResult(t *testing.T) {
	manager := tools.NewManagerWithNameValidator(func(string) bool { return true })
	if err := manager.RegisterTool(oversizedOrdinaryTool{}); err != nil {
		t.Fatalf("register oversized ordinary tool: %v", err)
	}
	response := Execute(ExecuteInput{
		Message:     jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "oversized-ordinary", Method: "tools/call", Params: mustMarshalParams(t, map[string]any{"name": "godot.test.ordinary.oversized", "arguments": map[string]any{}})},
		ToolManager: manager, Context: ToolCallContext{Modern: true}, Options: ToolCallOptions{SchemaValidationEnabled: true, PermissionMode: "allow_all"},
	})
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInternalError) || response.Error.Data != nil {
		t.Fatalf("expected generic oversized-result protocol error, got %#v", response)
	}
}

func TestExecuteRejectsOversizedModernSemanticErrorResult(t *testing.T) {
	manager := tools.NewManager()
	if err := manager.RegisterTool(oversizedSemanticErrorTool{}); err != nil {
		t.Fatalf("register oversized semantic error tool: %v", err)
	}
	response := Execute(ExecuteInput{
		Message:     jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "oversized-semantic", Method: "tools/call", Params: mustMarshalParams(t, map[string]any{"name": "godot.test.semantic.oversized", "arguments": map[string]any{}})},
		ToolManager: manager,
		Context:     ToolCallContext{Modern: true},
		Options:     ToolCallOptions{SchemaValidationEnabled: true, PermissionMode: "allow_all"},
	})
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInternalError) || response.Result != nil {
		t.Fatalf("expected bounded semantic-error protocol failure, got %#v", response)
	}
}

func TestExecuteRejectsOversizedCompleteResultAfterStructuredContent(t *testing.T) {
	large := strings.Repeat("x", mcpv20260728.MaxDecodedContentBlockBytes)
	manager := tools.NewManagerWithNameValidator(func(string) bool { return true })
	if err := manager.RegisterTool(invalidContentResultTool{
		content:    []any{mcp.TextContent{Type: "text", Text: large}},
		structured: map[string]any{"copy": large},
	}); err != nil {
		t.Fatalf("register amplified result tool: %v", err)
	}
	response := Execute(ExecuteInput{
		Message:     jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "amplified-result", Method: "tools/call", Params: mustMarshalParams(t, map[string]any{"name": "godot.test.content.invalid", "arguments": map[string]any{}})},
		ToolManager: manager, Context: ToolCallContext{Modern: true}, Options: ToolCallOptions{SchemaValidationEnabled: true, PermissionMode: "allow_all"},
	})
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInternalError) {
		t.Fatalf("expected amplified complete result to be rejected, got %#v", response)
	}
}

func TestExecuteRejectsInvalidOrUnboundedStandardContentBlocks(t *testing.T) {
	oversizedBlock := strings.Repeat("x", mcpv20260728.MaxDecodedContentBlockBytes+1)
	totalBlock := strings.Repeat("x", 6<<20)
	tests := []struct {
		name    string
		content []any
	}{
		{name: "invalid MIME", content: []any{mcp.AudioContent{Type: "audio", Data: "YQ==", MimeType: "not a MIME"}}},
		{name: "wrong media kind", content: []any{mcp.ImageContent{Type: "image", Data: "YQ==", MimeType: "audio/wav"}}},
		{name: "resource has text and blob", content: []any{mcp.EmbeddedResourceContent{Type: "resource", Resource: mcp.ResourceContents{URI: "test://resource", Text: "text", Blob: "YQ=="}}}},
		{name: "relative resource URI", content: []any{mcp.ResourceLinkContent{Type: "resource_link", URI: "relative/path", Name: "relative"}}},
		{name: "unknown block", content: []any{map[string]any{"type": "video", "data": "YQ=="}}},
		{name: "single block limit", content: []any{mcp.TextContent{Type: "text", Text: oversizedBlock}}},
		{name: "aggregate limit", content: []any{mcp.TextContent{Type: "text", Text: totalBlock}, mcp.TextContent{Type: "text", Text: totalBlock}, mcp.TextContent{Type: "text", Text: totalBlock}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := tools.NewManagerWithNameValidator(func(string) bool { return true })
			if err := manager.RegisterTool(invalidContentResultTool{content: test.content}); err != nil {
				t.Fatalf("register invalid content tool: %v", err)
			}
			response := Execute(ExecuteInput{
				Message:     jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: test.name, Method: "tools/call", Params: mustMarshalParams(t, map[string]any{"name": "godot.test.content.invalid", "arguments": map[string]any{}})},
				ToolManager: manager, Context: ToolCallContext{Modern: true}, Options: ToolCallOptions{SchemaValidationEnabled: true, PermissionMode: "allow_all"},
			})
			if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInternalError) || response.Error.Data != nil {
				t.Fatalf("expected generic internal protocol error, got %#v", response)
			}
		})
	}
}

func TestExecuteAcceptsSchemaValidEmptyTextAndBinaryPayloads(t *testing.T) {
	manager := tools.NewManagerWithNameValidator(func(string) bool { return true })
	content := []any{
		mcp.TextContent{Type: "text", Text: ""},
		mcp.ImageContent{Type: "image", Data: "", MimeType: "image/png"},
	}
	if err := manager.RegisterTool(invalidContentResultTool{content: content}); err != nil {
		t.Fatalf("register content tool: %v", err)
	}
	response := Execute(ExecuteInput{
		Message:     jsonrpc.Request{JSONRPC: jsonrpc.Version, ID: "empty-content", Method: "tools/call", Params: mustMarshalParams(t, map[string]any{"name": "godot.test.content.invalid", "arguments": map[string]any{}})},
		ToolManager: manager, Context: ToolCallContext{Modern: true}, Options: ToolCallOptions{SchemaValidationEnabled: true, PermissionMode: "allow_all"},
	})
	if response.Error != nil {
		t.Fatalf("schema-valid empty payload was rejected: %#v", response.Error)
	}
}

func TestEnrichToolCallArgumentsPreservesClientIDAndProgressRouteKey(t *testing.T) {
	arguments := enrichToolCallArguments(map[string]any{}, ToolCallContext{
		RequestID:        "1",
		ProgressRouteKey: "progress-1",
	}, ToolCallOptions{}, nil, false)
	context := mustMap(t, arguments["_mcp"])
	if context["request_id"] != "1" {
		t.Fatalf("expected client request id to remain available, got %v", context["request_id"])
	}
	if context["progress_route_key"] != "progress-1" {
		t.Fatalf("expected internal progress route key, got %v", context["progress_route_key"])
	}
}

func mustMarshalParams(t *testing.T, payload map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return raw
}

func mustMap(t *testing.T, value any) map[string]any {
	t.Helper()
	out, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", value)
	}
	return out
}
