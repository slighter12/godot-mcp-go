package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/slighter12/godot-mcp-go/config"
	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
	"github.com/slighter12/godot-mcp-go/tools"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
	"github.com/slighter12/godot-mcp-go/transport/shared"
)

type headerEchoTool struct{}

func (headerEchoTool) Name() string        { return "godot.test.header.echo" }
func (headerEchoTool) Description() string { return "Echo a header-bound value" }
func (headerEchoTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"value": map[string]any{"type": "string", "x-mcp-header": "Value"},
		},
		Required: []string{"value"},
	}
}
func (headerEchoTool) Execute(args json.RawMessage) ([]byte, error) { return args, nil }

type nestedHeaderEchoTool struct{}

func (nestedHeaderEchoTool) Name() string        { return "godot.test.header.nested" }
func (nestedHeaderEchoTool) Description() string { return "Echo a nested header-bound value" }
func (nestedHeaderEchoTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{
		"address": map[string]any{"type": "object", "properties": map[string]any{
			"city": map[string]any{"type": "string", "x-mcp-header": "City"},
		}},
	}}
}
func (nestedHeaderEchoTool) Execute(args json.RawMessage) ([]byte, error) { return args, nil }

type booleanHeaderEchoTool struct{}

type httpCompletionProviderFunc func(shared.CompletionRequest) ([]string, int, bool, error)

type httpRoundTripTool struct{}

func (httpRoundTripTool) Name() string                            { return "godot.test.mrtr.http" }
func (httpRoundTripTool) Description() string                     { return "Production HTTP MRTR seam" }
func (httpRoundTripTool) InputSchema() mcp.InputSchema            { return mcp.InputSchema{Type: "object"} }
func (httpRoundTripTool) Execute(json.RawMessage) ([]byte, error) { return nil, nil }
func (httpRoundTripTool) ExecuteRoundTrip(context.Context, mcp.RoundTripRequest) (mcp.RoundTripOutcome, error) {
	return mcp.RoundTripOutcome{InputRequired: &mcp.InputRequiredSpec{ProtectState: true, InputRequests: map[string]mcp.InputRequest{
		"confirm": {Method: "elicitation/create", Params: map[string]any{
			"message": "Confirm",
			"requestedSchema": map[string]any{
				"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}, "required": []any{"ok"},
			},
		}},
	}}}, nil
}

func (f httpCompletionProviderFunc) Complete(request shared.CompletionRequest) ([]string, int, bool, error) {
	return f(request)
}

func (booleanHeaderEchoTool) Name() string        { return "godot.test.header.boolean" }
func (booleanHeaderEchoTool) Description() string { return "Echo a boolean header-bound value" }
func (booleanHeaderEchoTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{
		"enabled": map[string]any{"type": "boolean", "x-mcp-header": "Enabled"},
	}}
}
func (booleanHeaderEchoTool) Execute(args json.RawMessage) ([]byte, error) { return args, nil }

func TestConformanceConstructorDoesNotRegisterProductionGlobalCallbacks(t *testing.T) {
	server := NewConformanceServer(config.NewConfig(), tools.NewManager(), promptcatalog.NewRegistry(true), nil, shared.DispatchProviders{}, nil)
	if server.releaseNotificationSender != nil || server.releaseProgressNotifier != nil {
		t.Fatal("conformance constructor registered production global callbacks")
	}
}

func TestProductionHTTPCanEnableCompletionThroughServerOptions(t *testing.T) {
	server, err := NewServerWithOptions(config.NewConfig(), ServerOptions{DispatchProviders: shared.DispatchProviders{
		Completion: httpCompletionProviderFunc(func(shared.CompletionRequest) ([]string, int, bool, error) {
			return []string{"scene/Main.tscn"}, 1, false, nil
		}),
	}})
	if err != nil {
		t.Fatalf("create configured production server: %v", err)
	}
	server.promptCatalog = promptcatalog.NewRegistry(true)

	discover, _, status := postModernMCP(t, server, map[string]any{"jsonrpc": "2.0", "id": "discover", "method": "server/discover", "params": map[string]any{}}, "", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK {
		t.Fatalf("discover status=%d response=%#v", status, discover)
	}
	capabilities := mustMap(t, mustMap(t, discover["result"])["capabilities"])
	if _, ok := capabilities["completions"]; !ok {
		t.Fatalf("configured production server did not advertise completions: %#v", capabilities)
	}

	response, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0", "id": "completion", "method": "completion/complete",
		"params": map[string]any{"ref": map[string]any{"type": "ref/prompt", "name": "scene"}, "argument": map[string]any{"name": "path", "value": "scene/"}},
	}, "", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK || response["error"] != nil {
		t.Fatalf("configured completion failed, status=%d response=%#v", status, response)
	}
	completion := mustMap(t, mustMap(t, response["result"])["completion"])
	values, ok := completion["values"].([]any)
	if !ok || len(values) != 1 || values[0] != "scene/Main.tscn" {
		t.Fatalf("unexpected completion result: %#v", completion)
	}
}

func TestProductionHTTPCanEnableIntegrityProtectedMRTR(t *testing.T) {
	initHTTPTestLogger.Do(func() {
		if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
			t.Fatalf("init logger: %v", err)
		}
	})
	server, err := NewServerWithOptions(config.NewConfig(), ServerOptions{RequestStateKeyRing: &mcpv20260728.RequestStateKeyRing{
		Active: mcpv20260728.RequestStateKey{ID: "active-2026-08", Key: []byte("0123456789abcdef0123456789abcdef")},
	}})
	if err != nil {
		t.Fatalf("create MRTR-enabled production server: %v", err)
	}
	if err := server.toolManager.RegisterTool(httpRoundTripTool{}); err != nil {
		t.Fatalf("register MRTR tool: %v", err)
	}
	response, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0", "id": "mrtr", "method": "tools/call", "params": map[string]any{"name": "godot.test.mrtr.http", "arguments": map[string]any{}},
	}, "", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK || response["error"] != nil {
		t.Fatalf("production MRTR failed, status=%d response=%#v", status, response)
	}
	result := mustMap(t, response["result"])
	if result["resultType"] != "input_required" || result["requestState"] == "" {
		t.Fatalf("unexpected production MRTR result: %#v", result)
	}
}

func TestProductionHTTPRejectsAmbiguousRequestStateKeyConfiguration(t *testing.T) {
	_, err := NewServerWithOptions(config.NewConfig(), ServerOptions{
		RequestStateKey: []byte("0123456789abcdef0123456789abcdef"),
		RequestStateKeyRing: &mcpv20260728.RequestStateKeyRing{
			Active: mcpv20260728.RequestStateKey{ID: "active", Key: []byte("abcdef0123456789abcdef0123456789")},
		},
	})
	if err == nil {
		t.Fatal("ambiguous legacy and key-ring configuration was accepted")
	}
}

func TestModernHTTPRejectsMissingHeaderForAnnotatedToolArgument(t *testing.T) {
	server := newTestHTTPServer(t, true)
	if err := server.toolManager.RegisterTool(headerEchoTool{}); err != nil {
		t.Fatalf("register header tool: %v", err)
	}
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      "header",
		"method":  "tools/call",
		"params": modernParams(map[string]any{
			"name":      "godot.test.header.echo",
			"arguments": map[string]any{"value": "Hello"},
		}, modernClientOptions{}),
	}
	response, _, status := postRawMCP(t, server, body, map[string]string{
		headerProtocolVersion: mcpv20260728.ProtocolVersion,
		headerMethod:          "tools/call",
		headerName:            "godot.test.header.echo",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400, got %d response=%#v", status, response)
	}
	errorObject := mustMap(t, response["error"])
	if errorObject["code"] != float64(-32020) {
		t.Fatalf("expected -32020, got %#v", errorObject)
	}
}

func TestModernHTTPRejectsMissingHeaderForNestedAnnotatedArgument(t *testing.T) {
	server := newTestHTTPServer(t, true)
	if err := server.toolManager.RegisterTool(nestedHeaderEchoTool{}); err != nil {
		t.Fatalf("register nested header tool: %v", err)
	}
	response, _, status := postRawMCP(t, server, map[string]any{
		"jsonrpc": "2.0", "id": "nested-header", "method": "tools/call",
		"params": modernParams(map[string]any{
			"name": "godot.test.header.nested", "arguments": map[string]any{"address": map[string]any{"city": "Taipei"}},
		}, modernClientOptions{}),
	}, map[string]string{
		headerProtocolVersion: mcpv20260728.ProtocolVersion,
		headerMethod:          "tools/call",
		headerName:            "godot.test.header.nested",
	})
	if status != http.StatusBadRequest || mustMap(t, response["error"])["code"] != float64(jsonrpc.ErrHeaderMismatch) {
		t.Fatalf("expected nested missing header mismatch, status=%d response=%#v", status, response)
	}
}

func TestModernHTTPAcceptsMatchingNestedAnnotatedArgumentHeader(t *testing.T) {
	server := newTestHTTPServer(t, true)
	if err := server.toolManager.RegisterTool(nestedHeaderEchoTool{}); err != nil {
		t.Fatalf("register nested header tool: %v", err)
	}
	response, _, status := postRawMCP(t, server, map[string]any{
		"jsonrpc": "2.0", "id": "nested-header", "method": "tools/call",
		"params": modernParams(map[string]any{
			"name": "godot.test.header.nested", "arguments": map[string]any{"address": map[string]any{"city": "Taipei"}},
		}, modernClientOptions{}),
	}, map[string]string{
		headerProtocolVersion: mcpv20260728.ProtocolVersion,
		headerMethod:          "tools/call",
		headerName:            "godot.test.header.nested",
		"Mcp-Param-City":      "Taipei",
	})
	if status != http.StatusOK || response["error"] != nil {
		t.Fatalf("expected matching nested header to pass, status=%d response=%#v", status, response)
	}
}

func TestModernHTTPRejectsNonCanonicalBooleanParameterHeader(t *testing.T) {
	server := newTestHTTPServer(t, true)
	if err := server.toolManager.RegisterTool(booleanHeaderEchoTool{}); err != nil {
		t.Fatalf("register boolean header tool: %v", err)
	}
	response, _, status := postRawMCP(t, server, map[string]any{
		"jsonrpc": "2.0", "id": "boolean-header", "method": "tools/call",
		"params": modernParams(map[string]any{
			"name": "godot.test.header.boolean", "arguments": map[string]any{"enabled": true},
		}, modernClientOptions{}),
	}, map[string]string{
		headerProtocolVersion: mcpv20260728.ProtocolVersion,
		headerMethod:          "tools/call",
		headerName:            "godot.test.header.boolean",
		"Mcp-Param-Enabled":   "TRUE",
	})
	if status != http.StatusBadRequest || mustMap(t, response["error"])["code"] != float64(jsonrpc.ErrHeaderMismatch) {
		t.Fatalf("expected non-canonical boolean to fail closed, status=%d response=%#v", status, response)
	}
}

func TestModernHTTPRejectsMismatchedAndDuplicateParameterHeaders(t *testing.T) {
	for _, test := range []struct {
		name   string
		values []string
	}{
		{name: "mismatched", values: []string{"Goodbye"}},
		{name: "duplicated", values: []string{"Hello", "Hello"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := newTestHTTPServer(t, true)
			if err := server.toolManager.RegisterTool(headerEchoTool{}); err != nil {
				t.Fatalf("register header tool: %v", err)
			}
			response, status := postRawMCPWithHeaderValues(t, server, map[string]any{
				"jsonrpc": "2.0", "id": test.name, "method": "tools/call",
				"params": modernParams(map[string]any{"name": "godot.test.header.echo", "arguments": map[string]any{"value": "Hello"}}, modernClientOptions{}),
			}, map[string][]string{
				headerProtocolVersion: {mcpv20260728.ProtocolVersion},
				headerMethod:          {"tools/call"},
				headerName:            {"godot.test.header.echo"},
				"Mcp-Param-Value":     test.values,
			})
			if status != http.StatusBadRequest || mustMap(t, response["error"])["code"] != float64(jsonrpc.ErrHeaderMismatch) {
				t.Fatalf("expected header mismatch, status=%d response=%#v", status, response)
			}
		})
	}
}

func TestModernHTTPRejectsLegacyToolAliasBeforeHeaderValidation(t *testing.T) {
	server := newTestHTTPServer(t, true)
	if err := server.toolManager.RegisterTool(headerEchoTool{}); err != nil {
		t.Fatalf("register header tool: %v", err)
	}
	response, _, status := postRawMCP(t, server, map[string]any{
		"jsonrpc": "2.0", "id": "legacy-tool-alias", "method": "tools/call",
		"params": modernParams(map[string]any{
			"tool": "godot.test.header.echo", "arguments": map[string]any{"value": "bypassed"},
		}, modernClientOptions{}),
	}, map[string]string{
		headerProtocolVersion: mcpv20260728.ProtocolVersion,
		headerMethod:          "tools/call",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400 for legacy alias, got %d response=%#v", status, response)
	}
	assertRPCError(t, response, jsonrpc.ErrInvalidParams)
}

func TestModernHTTPDecodesAndAcceptsMatchingBase64AnnotatedHeader(t *testing.T) {
	server := newTestHTTPServer(t, true)
	if err := server.toolManager.RegisterTool(headerEchoTool{}); err != nil {
		t.Fatalf("register header tool: %v", err)
	}
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      "header",
		"method":  "tools/call",
		"params": modernParams(map[string]any{
			"name":      "godot.test.header.echo",
			"arguments": map[string]any{"value": "Hello"},
		}, modernClientOptions{}),
	}
	response, _, status := postRawMCP(t, server, body, map[string]string{
		headerProtocolVersion: mcpv20260728.ProtocolVersion,
		headerMethod:          "tools/call",
		headerName:            "godot.test.header.echo",
		"Mcp-Param-Value":     "=?base64?SGVsbG8=?=",
	})
	if status != http.StatusOK || response["error"] != nil {
		t.Fatalf("expected accepted request, status=%d response=%#v", status, response)
	}
}

func TestModernHTTPRejectsOversizedAnnotatedHeader(t *testing.T) {
	server := newTestHTTPServer(t, true)
	if err := server.toolManager.RegisterTool(headerEchoTool{}); err != nil {
		t.Fatalf("register header tool: %v", err)
	}
	value := strings.Repeat("a", 8*1024+1)
	response, _, status := postRawMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "oversized-header",
		"method":  "tools/call",
		"params": modernParams(map[string]any{
			"name":      "godot.test.header.echo",
			"arguments": map[string]any{"value": value},
		}, modernClientOptions{}),
	}, map[string]string{
		headerProtocolVersion: mcpv20260728.ProtocolVersion,
		headerMethod:          "tools/call",
		headerName:            "godot.test.header.echo",
		"Mcp-Param-Value":     value,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400, got %d response=%#v", status, response)
	}
	errorObject := mustMap(t, response["error"])
	if errorObject["code"] != float64(jsonrpc.ErrHeaderMismatch) {
		t.Fatalf("expected header mismatch, got %#v", errorObject)
	}
}

func TestModernHTTPPreflightAllowsValidatedCustomParameterHeader(t *testing.T) {
	server := newTestHTTPServer(t, true)
	server.setupEcho()
	req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	req.Header.Set(echo.HeaderOrigin, "http://127.0.0.1:3000")
	req.Header.Set(echo.HeaderAccessControlRequestMethod, http.MethodPost)
	req.Header.Set(echo.HeaderAccessControlRequestHeaders, "Content-Type,MCP-Protocol-Version,Mcp-Method,Mcp-Name,Mcp-Param-Value")
	recorder := httptest.NewRecorder()
	server.echo.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected preflight 204, got %d: %s", recorder.Code, recorder.Body.String())
	}
	allowed := strings.ToLower(recorder.Header().Get(echo.HeaderAccessControlAllowHeaders))
	if !strings.Contains(allowed, "mcp-param-value") {
		t.Fatalf("custom parameter header was not allowed: %q", allowed)
	}
}

func TestModernHTTPPreflightRejectsUnrecognizedRequestedHeader(t *testing.T) {
	server := newTestHTTPServer(t, true)
	server.setupEcho()
	req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	req.Header.Set(echo.HeaderOrigin, "http://127.0.0.1:3000")
	req.Header.Set(echo.HeaderAccessControlRequestMethod, http.MethodPost)
	req.Header.Set(echo.HeaderAccessControlRequestHeaders, "Content-Type,X-Not-Allowed")
	recorder := httptest.NewRecorder()
	server.echo.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden preflight, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestModernHTTPUsesOptInDispatchHookWithoutChangingProductionDispatch(t *testing.T) {
	server := newTestHTTPServer(t, true)
	server.AttachDispatchHook(func(_ context.Context, request jsonrpc.Request, _ mcpv20260728.RequestMeta) (any, bool) {
		if request.Method != "resources/templates/list" {
			return nil, false
		}
		return jsonrpc.NewResponse(request.ID, map[string]any{
			"resultType":        "complete",
			"resourceTemplates": []map[string]any{{"uriTemplate": "test://template/{id}"}},
			"ttlMs":             int64(0),
			"cacheScope":        "public",
		}), true
	})

	response, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "fixture-template",
		"method":  "resources/templates/list",
		"params":  map[string]any{},
	}, "", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d response=%#v", status, response)
	}
	result := mustMap(t, response["result"])
	templates, ok := result["resourceTemplates"].([]any)
	if !ok || len(templates) != 1 {
		t.Fatalf("expected fixture template, got %#v", result)
	}
}

func TestModernHTTPReportsHeaderMismatchBeforeUnsupportedBodyVersion(t *testing.T) {
	server := newTestHTTPServer(t, true)
	params := modernParams(map[string]any{}, modernClientOptions{})
	meta := params["_meta"].(map[string]any)
	meta["io.modelcontextprotocol/protocolVersion"] = "v999.0.0"
	response, _, status := postRawMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "mismatch",
		"method":  "server/discover",
		"params":  params,
	}, map[string]string{
		headerProtocolVersion: mcpv20260728.ProtocolVersion,
		headerMethod:          "server/discover",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400, got %d", status)
	}
	errorObject := mustMap(t, response["error"])
	if errorObject["code"] != float64(jsonrpc.ErrHeaderMismatch) {
		t.Fatalf("expected header mismatch, got %#v", errorObject)
	}
}

func TestModernHTTPResultsUse20260728Envelopes(t *testing.T) {
	server := newTestHTTPServer(t, true)

	discover, sessionID, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "discover",
		"method":  "server/discover",
		"params":  map[string]any{},
	}, "", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK || sessionID != "" {
		t.Fatalf("server/discover status/session mismatch: status=%d session=%q", status, sessionID)
	}
	discoverResult := mustMap(t, discover["result"])
	if discoverResult["resultType"] != "complete" {
		t.Fatalf("expected complete discovery result, got %v", discoverResult["resultType"])
	}
	if discoverResult["supportedVersions"].([]any)[0] != mcpv20260728.ProtocolVersion {
		t.Fatalf("unexpected supported versions: %v", discoverResult["supportedVersions"])
	}
	if _, ok := discoverResult["_meta"].(map[string]any); !ok {
		t.Fatalf("expected server identity metadata, got %T", discoverResult["_meta"])
	}

	list, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "tools",
		"method":  "tools/list",
		"params":  map[string]any{},
	}, "editor-modern", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK {
		t.Fatalf("tools/list status=%d", status)
	}
	listResult := mustMap(t, list["result"])
	if listResult["resultType"] != "complete" || listResult["cacheScope"] != "public" {
		t.Fatalf("expected cacheable modern list result, got %#v", listResult)
	}
	if _, ok := listResult["ttlMs"].(float64); !ok {
		t.Fatalf("expected numeric ttlMs, got %T", listResult["ttlMs"])
	}

	call, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "health",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "godot.runtime.health.get",
			"arguments": map[string]any{},
		},
	}, "editor-modern", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK {
		t.Fatalf("tools/call status=%d", status)
	}
	callResult := mustMap(t, call["result"])
	for _, legacyKey := range []string{"type", "tool", "result"} {
		if _, exists := callResult[legacyKey]; exists {
			t.Fatalf("modern tools/call result contains legacy key %q: %#v", legacyKey, callResult)
		}
	}
	if callResult["resultType"] != "complete" || callResult["isError"] != false {
		t.Fatalf("unexpected modern tools/call result: %#v", callResult)
	}
	if _, ok := callResult["structuredContent"].(map[string]any); !ok {
		t.Fatalf("expected structuredContent, got %T", callResult["structuredContent"])
	}
}

func TestModernHTTPCommandSubscriptionAcknowledgesAndRoutesNotifications(t *testing.T) {
	server := newTestHTTPServer(t, true)
	editorSessionID := "editor-subscription-test"
	requestID := "listen-subscription-test"

	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  "subscriptions/listen",
		"params": map[string]any{
			"_meta": map[string]any{
				"io.modelcontextprotocol/protocolVersion": mcpv20260728.ProtocolVersion,
				"io.modelcontextprotocol/clientInfo": map[string]any{
					"name": "subscription-test", "version": "0.3.0",
				},
				"io.modelcontextprotocol/clientCapabilities": map[string]any{
					"extensions": map[string]any{
						mcpv20260728.CommandStreamExtensionID: map[string]any{"version": "1"},
					},
				},
				mcpv20260728.CommandStreamExtensionID: map[string]any{
					"version": "1", "editor_session_id": editorSessionID,
				},
			},
			"notifications": map[string]any{
				mcpv20260728.CommandStreamExtensionID: map[string]any{"command": true},
			},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal subscription request: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(string(raw))).WithContext(ctx)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAccept, "application/json, text/event-stream")
	req.Header.Set(headerProtocolVersion, mcpv20260728.ProtocolVersion)
	req.Header.Set(headerMethod, "subscriptions/listen")
	rec := newSynchronizedResponseRecorder()
	done := make(chan error, 1)
	go func() {
		done <- server.handleStreamableHTTPPost(echo.New().NewContext(req, rec))
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(rec.BodyString(), "notifications/subscriptions/acknowledged") {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(rec.BodyString(), "notifications/subscriptions/acknowledged") {
		t.Fatalf("subscription acknowledgement not written: %q", rec.BodyString())
	}

	sent := server.SendJSONRPCNotificationToEditor(editorSessionID, map[string]any{
		"jsonrpc": "2.0",
		"method":  mcpv20260728.CommandNotificationMethod,
		"params": map[string]any{
			"command_id": "command-1",
		},
	})
	if !sent {
		t.Fatal("expected command notification to reach subscription")
	}
	if !strings.Contains(rec.BodyString(), `"io.modelcontextprotocol/subscriptionId":"`+requestID+`"`) {
		t.Fatalf("expected subscription metadata on notification: %q", rec.BodyString())
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("subscription handler: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscription handler did not stop")
	}
}

func TestModernHTTPSubscriptionsIsolateDuplicateClientIDs(t *testing.T) {
	server := newTestHTTPServer(t, true)
	params := func(editorSessionID string) map[string]any {
		return modernParams(map[string]any{
			"notifications": map[string]any{"promptsListChanged": true},
		}, modernClientOptions{EditorSessionID: editorSessionID})
	}
	body := func(editorSessionID string) map[string]any {
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "subscriptions/listen",
			"params":  params(editorSessionID),
		}
	}

	cancelA, recorderA, doneA := startSubscriptionStream(t, server, body("editor-a"))
	cancelB, recorderB, doneB := startSubscriptionStream(t, server, body("editor-b"))
	defer cancelA()
	defer cancelB()

	waitForBodyContains(t, recorderA, "notifications/subscriptions/acknowledged")
	waitForBodyContains(t, recorderB, "notifications/subscriptions/acknowledged")

	sent := server.subscriptionManager.SendNotification("promptsListChanged", map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/prompts/list_changed",
		"params":  map[string]any{},
	})
	if sent != 2 {
		t.Fatalf("expected notification to reach both duplicate-ID subscriptions, sent=%d", sent)
	}
	for name, recorder := range map[string]*synchronizedResponseRecorder{"A": recorderA, "B": recorderB} {
		body := recorder.BodyString()
		if !strings.Contains(body, "notifications/prompts/list_changed") {
			t.Fatalf("subscription %s did not receive notification: %q", name, body)
		}
		if !strings.Contains(body, `"io.modelcontextprotocol/subscriptionId":1`) {
			t.Fatalf("subscription %s lost typed client subscription id: %q", name, body)
		}
	}

	cancelA()
	cancelB()
	for name, done := range map[string]<-chan error{"A": doneA, "B": doneB} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("subscription %s handler: %v", name, err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("subscription %s handler did not stop", name)
		}
	}
}

func TestModernHTTPShutdownGracefullyClosesSubscription(t *testing.T) {
	server := newTestHTTPServer(t, true)
	cancel, recorder, done := startSubscriptionStream(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      23,
		"method":  "subscriptions/listen",
		"params":  modernParams(map[string]any{}, modernClientOptions{}),
	})
	defer cancel()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if strings.Contains(recorder.BodyString(), "notifications/subscriptions/acknowledged") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(recorder.BodyString(), "notifications/subscriptions/acknowledged") {
		t.Fatalf("subscription acknowledgement not written: %q", recorder.BodyString())
	}

	if err := server.Shutdown(context.Background()); err != nil {
		t.Fatalf("server shutdown: %v", err)
	}
	if !strings.Contains(recorder.BodyString(), `"resultType":"complete"`) {
		t.Fatalf("expected graceful complete result, body=%q", recorder.BodyString())
	}
	if !strings.Contains(recorder.BodyString(), `"io.modelcontextprotocol/subscriptionId":23`) {
		t.Fatalf("expected typed subscription id in graceful result, body=%q", recorder.BodyString())
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("subscription handler: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscription handler did not stop after shutdown")
	}
}

func TestModernHTTPProgressUsesRequestScopedSSE(t *testing.T) {
	server := newTestHTTPServer(t, true)
	responseA := httptest.NewRecorder()
	responseB := httptest.NewRecorder()
	transportA := NewStreamableHTTPTransport(responseA, responseA)
	transportB := NewStreamableHTTPTransport(responseB, responseB)
	routeA := server.nextStreamRouteKey("progress")
	routeB := server.nextStreamRouteKey("progress")
	if routeA == routeB {
		t.Fatalf("expected unique progress route keys, got %q", routeA)
	}
	server.registerProgressStream(routeA, transportA)
	server.registerProgressStream(routeB, transportB)
	defer server.unregisterProgressStream(routeA, transportA)
	defer server.unregisterProgressStream(routeB, transportB)

	server.SendRuntimeCommandProgressNotification(tooltypes.RuntimeCommandProgressEvent{
		RequestID:        "1",
		ProgressRouteKey: routeA,
		SessionID:        "editor-progress-a",
		CommandName:      "godot.project.run",
		Progress:         0.5,
		Message:          "dispatching A",
		ProgressToken:    "request-progress-token-a",
	})
	server.SendRuntimeCommandProgressNotification(tooltypes.RuntimeCommandProgressEvent{
		RequestID:        "1",
		ProgressRouteKey: routeB,
		SessionID:        "editor-progress-b",
		CommandName:      "godot.project.run",
		Progress:         0.5,
		Message:          "dispatching B",
		ProgressToken:    "request-progress-token-b",
	})

	bodyA := responseA.Body.String()
	bodyB := responseB.Body.String()
	for name, body := range map[string]string{"A": bodyA, "B": bodyB} {
		if !strings.Contains(body, `"method":"notifications/progress"`) {
			t.Fatalf("expected progress notification on stream %s, got %q", name, body)
		}
	}
	if !strings.Contains(bodyA, `"progressToken":"request-progress-token-a"`) || strings.Contains(bodyA, `"progressToken":"request-progress-token-b"`) {
		t.Fatalf("progress stream A was not isolated: %q", bodyA)
	}
	if !strings.Contains(bodyB, `"progressToken":"request-progress-token-b"`) || strings.Contains(bodyB, `"progressToken":"request-progress-token-a"`) {
		t.Fatalf("progress stream B was not isolated: %q", bodyB)
	}
}

func TestModernHTTPCanceledProgressRecordIsRemoved(t *testing.T) {
	server := newTestHTTPServer(t, true)
	response := httptest.NewRecorder()
	transport := NewStreamableHTTPTransport(response, response)
	routeKey := server.nextStreamRouteKey("progress")
	server.registerProgressStream(routeKey, transport)

	server.markCanceledProgressRequest(routeKey)
	if !server.isCanceledProgressRequest(routeKey) {
		t.Fatal("expected progress record to be marked canceled")
	}

	server.unregisterProgressStream(routeKey, transport)
	if server.isCanceledProgressRequest(routeKey) {
		t.Fatal("expected canceled state to be removed with progress record")
	}
	server.progressMu.RLock()
	_, exists := server.progressStreams[routeKey]
	server.progressMu.RUnlock()
	if exists {
		t.Fatal("expected progress record to be deleted")
	}
}

func TestModernHTTPProgressWriteDoesNotHoldServerLock(t *testing.T) {
	server := newTestHTTPServer(t, true)
	writer := newBlockingSSEWriter()
	transport := NewStreamableHTTPTransport(writer, writer)
	routeKey := server.nextStreamRouteKey("progress")
	server.registerProgressStream(routeKey, transport)
	defer server.unregisterProgressStream(routeKey, transport)

	sendDone := make(chan bool, 1)
	go func() {
		sendDone <- server.sendRequestProgress(routeKey, map[string]any{"method": "notifications/progress"})
	}()
	select {
	case <-writer.entered:
	case <-time.After(time.Second):
		t.Fatal("progress write did not reach response writer")
	}

	otherRouteKey := server.nextStreamRouteKey("progress")
	registerDone := make(chan struct{})
	go func() {
		otherResponse := httptest.NewRecorder()
		server.registerProgressStream(otherRouteKey, NewStreamableHTTPTransport(otherResponse, otherResponse))
		close(registerDone)
	}()
	select {
	case <-registerDone:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("progress stream registration blocked behind network write")
	}
	defer func() {
		server.progressMu.Lock()
		delete(server.progressStreams, otherRouteKey)
		server.progressMu.Unlock()
	}()

	close(writer.release)
	select {
	case sent := <-sendDone:
		if !sent {
			t.Fatal("expected progress write to succeed")
		}
	case <-time.After(time.Second):
		t.Fatal("progress write did not finish after writer release")
	}
}

type blockingSSEWriter struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	header  http.Header
}

func newBlockingSSEWriter() *blockingSSEWriter {
	return &blockingSSEWriter{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		header:  make(http.Header),
	}
}

func (w *blockingSSEWriter) Header() http.Header {
	return w.header
}

func (w *blockingSSEWriter) WriteHeader(int) {}

func (w *blockingSSEWriter) Write(payload []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return len(payload), nil
}

func (w *blockingSSEWriter) Flush() {}
