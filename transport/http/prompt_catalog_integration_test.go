package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/slighter12/godot-mcp-go/config"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
)

var initHTTPTestLogger sync.Once

func TestInitializeCapabilitiesReflectPromptCatalog(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{name: "enabled", enabled: true},
		{name: "disabled", enabled: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestHTTPServer(t, tc.enabled)

			body := map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "initialize",
				"params": map[string]any{
					"protocolVersion": "2025-11-25",
				},
			}
			respBody, _, status := postMCP(t, server, body, "", "")
			if status != http.StatusOK {
				t.Fatalf("expected status %d, got %d", http.StatusOK, status)
			}

			result := mustMap(t, respBody["result"])
			capabilities := mustMap(t, result["capabilities"])
			_, hasPrompts := capabilities["prompts"]
			if tc.enabled && !hasPrompts {
				t.Fatal("expected prompts capability when prompt catalog is enabled")
			}
			if !tc.enabled && hasPrompts {
				t.Fatal("did not expect prompts capability when prompt catalog is disabled")
			}
		})
	}
}

func TestStreamableHTTPPromptsFlow(t *testing.T) {
	server := newTestHTTPServer(t, true)
	server.promptCatalog.RegisterPrompt(promptcatalog.Prompt{
		Name:        "scene-review",
		Description: "desc",
		Template:    "Review {{scene_path}}",
	})

	initBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-11-25",
		},
	}
	initResp, sessionID, status := postMCP(t, server, initBody, "", "")
	if status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status)
	}
	if sessionID == "" {
		t.Fatal("expected session id in initialize response header")
	}
	if initResp["error"] != nil {
		t.Fatalf("initialize returned error: %+v", initResp["error"])
	}

	listBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "prompts/list",
		"params":  map[string]any{},
	}
	listResp, _, status := postMCP(t, server, listBody, sessionID, "2025-11-25")
	if status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status)
	}
	listResult := mustMap(t, listResp["result"])
	promptsRaw, ok := listResult["prompts"].([]any)
	if !ok {
		t.Fatalf("expected prompts array, got %T", listResult["prompts"])
	}
	if len(promptsRaw) != 1 {
		t.Fatalf("expected 1 prompt, got %d", len(promptsRaw))
	}
	prompt := mustMap(t, promptsRaw[0])
	if prompt["name"] != "scene-review" {
		t.Fatalf("expected prompt name scene-review, got %v", prompt["name"])
	}

	getBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "prompts/get",
		"params": map[string]any{
			"name":      "scene-review",
			"arguments": map[string]any{"scene_path": "res://Main.tscn"},
		},
	}
	getResp, _, status := postMCP(t, server, getBody, sessionID, "2025-11-25")
	if status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status)
	}
	getResult := mustMap(t, getResp["result"])
	messagesRaw, ok := getResult["messages"].([]any)
	if !ok || len(messagesRaw) != 1 {
		t.Fatalf("expected one message, got %T %v", getResult["messages"], getResult["messages"])
	}
	message := mustMap(t, messagesRaw[0])
	content := mustMap(t, message["content"])
	if content["text"] != "Review res://Main.tscn" {
		t.Fatalf("expected rendered prompt text, got %v", content["text"])
	}
}

func TestStreamableHTTPPromptsNotSupportedWhenCatalogDisabled(t *testing.T) {
	server := newTestHTTPServer(t, false)

	initBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-11-25",
		},
	}
	initResp, sessionID, status := postMCP(t, server, initBody, "", "")
	if status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status)
	}
	if initResp["error"] != nil {
		t.Fatalf("initialize returned error: %+v", initResp["error"])
	}
	if sessionID == "" {
		t.Fatal("expected session id in initialize response header")
	}

	listBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "prompts/list",
		"params":  map[string]any{},
	}
	listResp, _, status := postMCP(t, server, listBody, sessionID, "2025-11-25")
	if status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status)
	}
	assertNotSupportedError(t, listResp)

	getBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "prompts/get",
		"params": map[string]any{
			"name": "scene-review",
		},
	}
	getResp, _, status := postMCP(t, server, getBody, sessionID, "2025-11-25")
	if status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status)
	}
	assertNotSupportedError(t, getResp)
}

func newTestHTTPServer(t *testing.T, promptCatalogEnabled bool) *Server {
	t.Helper()
	initHTTPTestLogger.Do(func() {
		if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
			t.Fatalf("init logger: %v", err)
		}
	})

	cfg := config.NewConfig()
	cfg.PromptCatalog.Enabled = promptCatalogEnabled

	server := NewServer(cfg)
	server.promptCatalog = promptcatalog.NewRegistry(promptCatalogEnabled)
	server.toolManager.RegisterDefaultTools()
	if err := server.registry.RegisterServer("default", server.toolManager.GetTools()); err != nil {
		t.Fatalf("register default server: %v", err)
	}
	return server
}

func postMCP(t *testing.T, server *Server, body map[string]any, sessionID string, protocolVersion string) (map[string]any, string, int) {
	t.Helper()

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(raw))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if sessionID != "" {
		req.Header.Set(headerSessionID, sessionID)
	}
	if protocolVersion != "" {
		req.Header.Set(headerProtocolVersion, protocolVersion)
	}

	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)
	if err := server.handleStreamableHTTPPost(ctx); err != nil {
		t.Fatalf("handleStreamableHTTPPost: %v", err)
	}

	var parsed map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
	}
	return parsed, rec.Header().Get(headerSessionID), rec.Code
}

func mustMap(t *testing.T, value any) map[string]any {
	t.Helper()
	out, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", value)
	}
	return out
}

func assertNotSupportedError(t *testing.T, response map[string]any) {
	t.Helper()
	errObj := mustMap(t, response["error"])

	code, ok := errObj["code"].(float64)
	if !ok {
		t.Fatalf("expected error code number, got %T", errObj["code"])
	}
	if int(code) != int(jsonrpc.ErrMethodNotFound) {
		t.Fatalf("expected method not found code %d, got %d", int(jsonrpc.ErrMethodNotFound), int(code))
	}

	data := mustMap(t, errObj["data"])
	if data["kind"] != "not_supported" {
		t.Fatalf("expected kind not_supported, got %v", data["kind"])
	}
}
