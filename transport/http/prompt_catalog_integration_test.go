package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
)

func TestModernHTTPPromptsFlow(t *testing.T) {
	server := newTestHTTPServer(t, true)
	server.promptCatalog.RegisterPrompt(promptcatalog.Prompt{
		Name:        "scene-review",
		Description: "Review a scene",
		Template:    "Review {{scene_path}}",
	})

	list, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "prompts-list",
		"method":  "prompts/list",
		"params":  map[string]any{},
	}, "editor-prompts", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK {
		t.Fatalf("prompts/list status=%d", status)
	}
	listResult := mustMap(t, list["result"])
	if listResult["resultType"] != "complete" || listResult["cacheScope"] != "public" {
		t.Fatalf("unexpected prompts/list result: %#v", listResult)
	}
	if _, ok := listResult["ttlMs"].(float64); !ok {
		t.Fatalf("expected numeric ttlMs, got %T", listResult["ttlMs"])
	}

	get, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "prompts-get",
		"method":  "prompts/get",
		"params": map[string]any{
			"name":      "scene-review",
			"arguments": map[string]any{"scene_path": "res://Main.tscn"},
		},
	}, "editor-prompts", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK {
		t.Fatalf("prompts/get status=%d", status)
	}
	getResult := mustMap(t, get["result"])
	if getResult["resultType"] != "complete" || getResult["cacheScope"] != "public" {
		t.Fatalf("unexpected prompts/get result: %#v", getResult)
	}
	messages := getResult["messages"].([]any)
	content := mustMap(t, mustMap(t, messages[0])["content"])
	if !strings.Contains(content["text"].(string), "res://Main.tscn") {
		t.Fatalf("expected rendered scene path, got %v", content["text"])
	}
}

func TestModernHTTPPromptValidation(t *testing.T) {
	server := newTestHTTPServer(t, true)
	server.promptCatalog.RegisterPrompt(promptcatalog.Prompt{
		Name:     "scene-review",
		Template: "Review {{scene_path}}",
	})

	response, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "invalid-arguments",
		"method":  "prompts/get",
		"params": map[string]any{
			"name":      "scene-review",
			"arguments": map[string]any{"scene_path": 42},
		},
	}, "editor-prompts", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK {
		t.Fatalf("prompts/get invalid arguments status=%d", status)
	}
	errorObject := assertRPCError(t, response, jsonrpc.ErrInvalidParams)
	data := mustMap(t, errorObject["data"])
	if data["field"] != "arguments" || data["problem"] != "invalid_type" {
		t.Fatalf("unexpected validation data: %#v", data)
	}

	server.config.PromptCatalog.Rendering.Mode = "strict"
	response, _, status = postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "missing-arguments",
		"method":  "prompts/get",
		"params": map[string]any{
			"name":      "scene-review",
			"arguments": map[string]any{},
		},
	}, "editor-prompts", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK {
		t.Fatalf("strict prompts/get status=%d", status)
	}
	errorObject = assertRPCError(t, response, jsonrpc.ErrInvalidParams)
	data = mustMap(t, errorObject["data"])
	if data["problem"] != "missing_required_arguments" {
		t.Fatalf("unexpected strict validation data: %#v", data)
	}
}

func TestModernHTTPPromptsDisabledUsesNotFound(t *testing.T) {
	server := newTestHTTPServer(t, false)
	response, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "disabled-prompts",
		"method":  "prompts/list",
		"params":  map[string]any{},
	}, "editor-prompts", mcpv20260728.ProtocolVersion)
	if status != http.StatusNotFound {
		t.Fatalf("expected disabled prompts status %d, got %d", http.StatusNotFound, status)
	}
	errorObject := assertRPCError(t, response, jsonrpc.ErrMethodNotFound)
	data := mustMap(t, errorObject["data"])
	if data["kind"] != "not_supported" {
		t.Fatalf("unexpected disabled prompt data: %#v", data)
	}
}

func TestModernHTTPRemovedLifecycleMethodUsesNotFoundAndDiagnostics(t *testing.T) {
	server := newTestHTTPServer(t, true)
	response, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "initialize-removed",
		"method":  "initialize",
		"params":  map[string]any{},
	}, "", mcpv20260728.ProtocolVersion)
	if status != http.StatusNotFound {
		t.Fatalf("expected initialize status %d, got %d", http.StatusNotFound, status)
	}
	errorObject := assertRPCError(t, response, jsonrpc.ErrMethodNotFound)
	data := mustMap(t, errorObject["data"])
	if supported, ok := data["supported"].([]any); !ok || len(supported) != 1 || supported[0] != mcpv20260728.ProtocolVersion {
		t.Fatalf("expected supported version diagnostic, got %#v", data["supported"])
	}
}

func TestModernHTTPGetAndDeleteReturn405(t *testing.T) {
	server := newTestHTTPServer(t, true)
	server.setupEcho()
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/mcp", nil)
			recorder := httptest.NewRecorder()
			server.echo.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected 405, got %d", recorder.Code)
			}
			var response map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode 405 response: %v", err)
			}
			assertRPCError(t, response, jsonrpc.ErrMethodNotFound)
		})
	}
}

func TestModernHTTPPromptListChangedSubscription(t *testing.T) {
	server := newTestHTTPServer(t, true)
	params := modernParams(map[string]any{
		"notifications": map[string]any{"promptsListChanged": true},
	}, modernClientOptions{})
	cancel, recorder, done := startSubscriptionStream(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      17,
		"method":  "subscriptions/listen",
		"params":  params,
	})
	defer cancel()
	waitForBodyContains(t, recorder, "notifications/subscriptions/acknowledged")
	if sent := server.BroadcastPromptListChanged(); sent != 1 {
		t.Fatalf("expected one prompt notification, sent=%d", sent)
	}
	waitForBodyContains(t, recorder, "notifications/prompts/list_changed")
	if !strings.Contains(recorder.BodyString(), `"io.modelcontextprotocol/subscriptionId":17`) {
		t.Fatalf("expected numeric subscription id, body=%q", recorder.BodyString())
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

func TestModernHTTPPostRejectsMismatchedProtocolHeader(t *testing.T) {
	server := newTestHTTPServer(t, true)
	response, _, status := postRawMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "header-mismatch",
		"method":  "tools/list",
		"params":  modernParams(map[string]any{}, modernClientOptions{}),
	}, map[string]string{
		headerProtocolVersion: "2025-11-25",
		headerMethod:          "tools/list",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, status)
	}
	assertRPCError(t, response, jsonrpc.ErrHeaderMismatch)
}

func TestModernHTTPOriginValidation(t *testing.T) {
	server := newTestHTTPServer(t, true)
	server.setupEcho()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	req.Header.Set(echo.HeaderOrigin, "https://evil.example")
	recorder := httptest.NewRecorder()
	server.echo.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden origin status, got %d", recorder.Code)
	}
}

func waitForBodyContains(t *testing.T, recorder *synchronizedResponseRecorder, needle string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(recorder.BodyString(), needle) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q; body=%q", needle, recorder.BodyString())
}
