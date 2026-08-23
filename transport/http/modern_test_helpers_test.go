package http

import (
	"bytes"
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
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
)

var initHTTPTestLogger sync.Once

type modernClientOptions struct {
	EditorSessionID string
	Mutating        bool
	ClientName      string
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
	if err := server.registerRuntimeTools(); err != nil {
		t.Fatalf("register runtime tools: %v", err)
	}
	return server
}

func postModernMCP(t *testing.T, server *Server, body map[string]any, editorSessionID, protocolVersion string) (map[string]any, string, int) {
	return postModernMCPWithOptions(t, server, body, modernClientOptions{EditorSessionID: editorSessionID}, protocolVersion)
}

func postModernMCPWithOptions(t *testing.T, server *Server, body map[string]any, options modernClientOptions, protocolVersion string) (map[string]any, string, int) {
	t.Helper()
	requestBody := cloneModernMap(body)
	existingParams, _ := requestBody["params"].(map[string]any)
	requestBody["params"] = modernParams(existingParams, options)
	headers := map[string]string{
		headerProtocolVersion: protocolVersion,
		headerMethod:          stringValue(requestBody["method"]),
	}
	name := requestName(jsonrpc.Request{
		Method: stringValue(requestBody["method"]),
		Params: mustRawMap(t, requestBody["params"].(map[string]any)),
	})
	if name != "" {
		headers[headerName] = name
	}
	return postRawMCP(t, server, requestBody, headers)
}

func postRawMCP(t *testing.T, server *Server, body map[string]any, headers map[string]string) (map[string]any, string, int) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(raw))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAccept, "application/json, text/event-stream")
	for key, value := range headers {
		if value != "" {
			req.Header.Set(key, value)
		}
	}
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)
	if err := server.handleStreamableHTTPPost(ctx); err != nil {
		t.Fatalf("handleStreamableHTTPPost: %v", err)
	}
	return decodeHTTPResponse(t, rec), "", rec.Code
}

func postRawMCPWithHeaderValues(t *testing.T, server *Server, body map[string]any, headers map[string][]string) (map[string]any, int) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(raw))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAccept, "application/json, text/event-stream")
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	recorder := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, recorder)
	if err := server.handleStreamableHTTPPost(ctx); err != nil {
		t.Fatalf("handleStreamableHTTPPost: %v", err)
	}
	return decodeHTTPResponse(t, recorder), recorder.Code
}

func modernParams(params map[string]any, options modernClientOptions) map[string]any {
	result := cloneModernMap(params)
	clientName := options.ClientName
	if clientName == "" {
		clientName = "modern-test-client"
	}
	godotSettings := map[string]any{
		"version":  "1",
		"mutating": options.Mutating,
	}
	if options.EditorSessionID != "" {
		godotSettings["editor_session_id"] = options.EditorSessionID
	}
	result["_meta"] = map[string]any{
		"io.modelcontextprotocol/protocolVersion": mcpv20260728.ProtocolVersion,
		"io.modelcontextprotocol/clientInfo": map[string]any{
			"name":    clientName,
			"version": "0.3.0",
		},
		"io.modelcontextprotocol/clientCapabilities": map[string]any{
			"extensions": map[string]any{
				mcpv20260728.GodotExtensionID: godotSettings,
			},
		},
	}
	return result
}

func decodeHTTPResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if recorder.Body.Len() == 0 {
		return nil
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v; body=%q", err, recorder.Body.String())
	}
	return response
}

func cloneModernMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func mustMap(t *testing.T, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", value)
	}
	return result
}

func mustRawMap(t *testing.T, value map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal raw map: %v", err)
	}
	return raw
}

func startSubscriptionStream(t *testing.T, server *Server, body map[string]any) (context.CancelFunc, *synchronizedResponseRecorder, <-chan error) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal subscription request: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(raw)).WithContext(ctx)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAccept, "application/json, text/event-stream")
	req.Header.Set(headerProtocolVersion, mcpv20260728.ProtocolVersion)
	req.Header.Set(headerMethod, "subscriptions/listen")
	recorder := newSynchronizedResponseRecorder()
	done := make(chan error, 1)
	go func() {
		done <- server.handleStreamableHTTPPost(echo.New().NewContext(req, recorder))
	}()
	return cancel, recorder, done
}

type synchronizedResponseRecorder struct {
	mu     sync.RWMutex
	header http.Header
	body   bytes.Buffer
	code   int
}

func newSynchronizedResponseRecorder() *synchronizedResponseRecorder {
	return &synchronizedResponseRecorder{header: make(http.Header)}
}

func (r *synchronizedResponseRecorder) Header() http.Header {
	return r.header
}

func (r *synchronizedResponseRecorder) WriteHeader(code int) {
	r.mu.Lock()
	if r.code == 0 {
		r.code = code
	}
	r.mu.Unlock()
}

func (r *synchronizedResponseRecorder) Write(data []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.code == 0 {
		r.code = http.StatusOK
	}
	return r.body.Write(data)
}

func (r *synchronizedResponseRecorder) Flush() {}

func (r *synchronizedResponseRecorder) BodyString() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.body.String()
}

func assertRPCError(t *testing.T, response map[string]any, code jsonrpc.ErrorCode) map[string]any {
	t.Helper()
	errorObject := mustMap(t, response["error"])
	got, ok := errorObject["code"].(float64)
	if !ok || int(got) != int(code) {
		t.Fatalf("expected JSON-RPC error %d, got %v", int(code), errorObject["code"])
	}
	return errorObject
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
