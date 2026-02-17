package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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
			promptsCapabilityRaw, hasPrompts := capabilities["prompts"]
			if tc.enabled && !hasPrompts {
				t.Fatal("expected prompts capability when prompt catalog is enabled")
			}
			if !tc.enabled && hasPrompts {
				t.Fatal("did not expect prompts capability when prompt catalog is disabled")
			}
			if tc.enabled {
				promptsCapability := mustMap(t, promptsCapabilityRaw)
				if promptsCapability["listChanged"] != true {
					t.Fatalf("expected prompts.listChanged=true, got %v", promptsCapability["listChanged"])
				}
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
	if content["text"] != "Review <user_input name=\"scene_path\" format=\"json\">\n\"res://Main.tscn\"\n</user_input>" {
		t.Fatalf("expected rendered prompt text, got %v", content["text"])
	}
}

func TestStreamableHTTPPromptsGetRejectsNonStringArguments(t *testing.T) {
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
	_, sessionID, status := postMCP(t, server, initBody, "", "")
	if status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status)
	}
	if sessionID == "" {
		t.Fatal("expected session id in initialize response header")
	}

	getBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "prompts/get",
		"params": map[string]any{
			"name":      "scene-review",
			"arguments": map[string]any{"scene_path": float64(42)},
		},
	}
	getResp, _, status := postMCP(t, server, getBody, sessionID, "2025-11-25")
	if status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status)
	}
	errObj := mustMap(t, getResp["error"])
	code, ok := errObj["code"].(float64)
	if !ok || int(code) != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("expected invalid params code %d, got %v", int(jsonrpc.ErrInvalidParams), errObj["code"])
	}
	data := mustMap(t, errObj["data"])
	if data["kind"] != "invalid_params" {
		t.Fatalf("expected kind invalid_params, got %v", data["kind"])
	}
	if data["field"] != "arguments" {
		t.Fatalf("expected field arguments, got %v", data["field"])
	}
}

func TestStreamableHTTPPromptsGetStrictModeRejectsMissingArguments(t *testing.T) {
	server := newTestHTTPServer(t, true)
	server.config.PromptCatalog.Rendering.Mode = "strict"
	server.promptCatalog.RegisterPrompt(promptcatalog.Prompt{
		Name:        "scene-review",
		Description: "desc",
		Template:    "Review {{scene_path}} and {{line}}",
	})

	initBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-11-25",
		},
	}
	_, sessionID, status := postMCP(t, server, initBody, "", "")
	if status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status)
	}
	if sessionID == "" {
		t.Fatal("expected session id in initialize response header")
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
	errObj := mustMap(t, getResp["error"])
	code, ok := errObj["code"].(float64)
	if !ok || int(code) != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("expected invalid params code %d, got %v", int(jsonrpc.ErrInvalidParams), errObj["code"])
	}
	data := mustMap(t, errObj["data"])
	if data["problem"] != "missing_required_arguments" {
		t.Fatalf("expected missing_required_arguments, got %v", data["problem"])
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

func TestStreamableHTTPGetSSEHeadersAndValidation(t *testing.T) {
	server := newTestHTTPServer(t, true)
	sessionID := "session-test"
	server.sessionManager.CreateSession(sessionID)
	server.sessionManager.SetProtocolVersion(sessionID, "2025-11-25")

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set(headerSessionID, sessionID)
	req.Header.Set(headerProtocolVersion, "2025-11-25")
	req.Header.Set(echo.HeaderAccept, "text/event-stream")
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req = req.WithContext(ctx)
	echoCtx := echo.New().NewContext(req, rec)

	done := make(chan error, 1)
	go func() {
		done <- server.handleStreamableHTTPGet(echoCtx)
	}()

	waitForTransport(t, server, sessionID)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if got := rec.Header().Get(echo.HeaderContentType); got != "text/event-stream" {
		t.Fatalf("expected Content-Type text/event-stream, got %q", got)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("handleStreamableHTTPGet: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE handler shutdown")
	}
}

func TestStreamableHTTPGetSSERejectsMissingAcceptHeader(t *testing.T) {
	server := newTestHTTPServer(t, true)
	sessionID := "session-test"
	server.sessionManager.CreateSession(sessionID)
	server.sessionManager.SetProtocolVersion(sessionID, "2025-11-25")

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set(headerSessionID, sessionID)
	req.Header.Set(headerProtocolVersion, "2025-11-25")
	rec := httptest.NewRecorder()
	echoCtx := echo.New().NewContext(req, rec)

	if err := server.handleStreamableHTTPGet(echoCtx); err != nil {
		t.Fatalf("handleStreamableHTTPGet: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestStreamableHTTPGetSSEAllowsMissingProtocolHeaderAfterNegotiation(t *testing.T) {
	server := newTestHTTPServer(t, true)
	sessionID := "session-test"
	server.sessionManager.CreateSession(sessionID)
	server.sessionManager.SetProtocolVersion(sessionID, "2025-11-25")

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set(headerSessionID, sessionID)
	req.Header.Set(echo.HeaderAccept, "text/event-stream")
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req = req.WithContext(ctx)
	echoCtx := echo.New().NewContext(req, rec)

	done := make(chan error, 1)
	go func() {
		done <- server.handleStreamableHTTPGet(echoCtx)
	}()

	waitForTransport(t, server, sessionID)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("handleStreamableHTTPGet: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE handler shutdown")
	}
}

func TestStreamableHTTPSessionReplacementClosesPreviousSSEStream(t *testing.T) {
	server := newTestHTTPServer(t, true)
	sessionID := "session-replace"
	server.sessionManager.CreateSession(sessionID)
	server.sessionManager.SetProtocolVersion(sessionID, "2025-11-25")

	firstReq := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	firstReq.Header.Set(headerSessionID, sessionID)
	firstReq.Header.Set(headerProtocolVersion, "2025-11-25")
	firstReq.Header.Set(echo.HeaderAccept, "text/event-stream")
	firstRec := httptest.NewRecorder()
	firstCtx, firstCancel := context.WithCancel(context.Background())
	defer firstCancel()
	firstReq = firstReq.WithContext(firstCtx)
	firstEchoCtx := echo.New().NewContext(firstReq, firstRec)

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- server.handleStreamableHTTPGet(firstEchoCtx)
	}()
	waitForTransport(t, server, sessionID)

	secondReq := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	secondReq.Header.Set(headerSessionID, sessionID)
	secondReq.Header.Set(headerProtocolVersion, "2025-11-25")
	secondReq.Header.Set(echo.HeaderAccept, "text/event-stream")
	secondRec := httptest.NewRecorder()
	secondCtx, secondCancel := context.WithCancel(context.Background())
	defer secondCancel()
	secondReq = secondReq.WithContext(secondCtx)
	secondEchoCtx := echo.New().NewContext(secondReq, secondRec)

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- server.handleStreamableHTTPGet(secondEchoCtx)
	}()
	waitForTransport(t, server, sessionID)

	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first stream handler error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first SSE stream shutdown after replacement")
	}

	secondCancel()
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("second stream handler error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second SSE stream shutdown")
	}
}

func TestStreamableHTTPGetSSERejectsMissingProtocolHeaderBeforeNegotiation(t *testing.T) {
	server := newTestHTTPServer(t, true)
	sessionID := "session-test"
	server.sessionManager.CreateSession(sessionID)

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set(headerSessionID, sessionID)
	req.Header.Set(echo.HeaderAccept, "text/event-stream")
	rec := httptest.NewRecorder()
	echoCtx := echo.New().NewContext(req, rec)

	if err := server.handleStreamableHTTPGet(echoCtx); err != nil {
		t.Fatalf("handleStreamableHTTPGet: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Missing MCP-Protocol-Version header") {
		t.Fatalf("expected missing protocol version error, got %q", rec.Body.String())
	}
}

func TestStreamableHTTPDeleteAllowsMissingProtocolHeaderAfterNegotiation(t *testing.T) {
	server := newTestHTTPServer(t, true)
	sessionID := "session-delete"
	server.sessionManager.CreateSession(sessionID)
	server.sessionManager.SetProtocolVersion(sessionID, "2025-11-25")

	req := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	req.Header.Set(headerSessionID, sessionID)
	rec := httptest.NewRecorder()
	echoCtx := echo.New().NewContext(req, rec)

	if err := server.handleStreamableHTTPDelete(echoCtx); err != nil {
		t.Fatalf("handleStreamableHTTPDelete: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if server.sessionManager.HasSession(sessionID) {
		t.Fatalf("expected session %q to be removed", sessionID)
	}
}

func TestStreamableHTTPDeleteClosesActiveSSEStream(t *testing.T) {
	server := newTestHTTPServer(t, true)
	sessionID := "session-delete-active-stream"
	server.sessionManager.CreateSession(sessionID)
	server.sessionManager.SetProtocolVersion(sessionID, "2025-11-25")

	streamReq := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	streamReq.Header.Set(headerSessionID, sessionID)
	streamReq.Header.Set(headerProtocolVersion, "2025-11-25")
	streamReq.Header.Set(echo.HeaderAccept, "text/event-stream")
	streamRec := httptest.NewRecorder()
	streamCtx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()
	streamReq = streamReq.WithContext(streamCtx)
	streamEchoCtx := echo.New().NewContext(streamReq, streamRec)

	streamDone := make(chan error, 1)
	go func() {
		streamDone <- server.handleStreamableHTTPGet(streamEchoCtx)
	}()
	waitForTransport(t, server, sessionID)

	deleteReq := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	deleteReq.Header.Set(headerSessionID, sessionID)
	deleteReq.Header.Set(headerProtocolVersion, "2025-11-25")
	deleteRec := httptest.NewRecorder()
	deleteEchoCtx := echo.New().NewContext(deleteReq, deleteRec)
	if err := server.handleStreamableHTTPDelete(deleteEchoCtx); err != nil {
		t.Fatalf("handleStreamableHTTPDelete: %v", err)
	}
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, deleteRec.Code)
	}

	select {
	case err := <-streamDone:
		if err != nil {
			t.Fatalf("stream handler error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE stream shutdown after session deletion")
	}
}

func TestStreamableHTTPPostAllowsMissingProtocolHeaderAfterNegotiation(t *testing.T) {
	server := newTestHTTPServer(t, true)

	initBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-11-25",
		},
	}
	_, sessionID, status := postMCP(t, server, initBody, "", "")
	if status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status)
	}
	if sessionID == "" {
		t.Fatal("expected session id in initialize response header")
	}

	pingBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "ping",
		"params":  map[string]any{},
	}
	pingResp, _, status := postMCP(t, server, pingBody, sessionID, "")
	if status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status)
	}
	if pingResp["error"] != nil {
		t.Fatalf("expected ping success, got error %+v", pingResp["error"])
	}
	result := mustMap(t, pingResp["result"])
	if len(result) != 0 {
		t.Fatalf("expected empty ping result, got %v", result)
	}
}

func TestStreamableHTTPPostRejectsMissingProtocolHeaderBeforeNegotiation(t *testing.T) {
	server := newTestHTTPServer(t, true)
	sessionID := "session-post"
	server.sessionManager.CreateSession(sessionID)

	pingBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "ping",
		"params":  map[string]any{},
	}
	pingResp, _, status := postMCP(t, server, pingBody, sessionID, "")
	if status != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, status)
	}
	errObj := mustMap(t, pingResp["error"])
	if errObj["message"] != "Missing MCP-Protocol-Version header" {
		t.Fatalf("expected missing header message, got %v", errObj["message"])
	}
}

func TestReloadPromptCatalogToolBroadcastsPromptListChanged(t *testing.T) {
	server := newTestHTTPServer(t, true)
	root := t.TempDir()
	skill := filepath.Join(root, "scene-review", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	content := "---\nname: scene-review\ntitle: Scene Review\ndescription: Prompt description\n---\nReview {{scene_path}}\n"
	if err := os.WriteFile(skill, []byte(content), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	server.config.PromptCatalog.Paths = []string{root}

	sessionID := "session-reload"
	server.sessionManager.CreateSession(sessionID)
	server.sessionManager.SetProtocolVersion(sessionID, "2025-11-25")

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set(headerSessionID, sessionID)
	req.Header.Set(headerProtocolVersion, "2025-11-25")
	req.Header.Set(echo.HeaderAccept, "text/event-stream")
	rec := httptest.NewRecorder()
	streamCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req = req.WithContext(streamCtx)
	echoCtx := echo.New().NewContext(req, rec)

	done := make(chan error, 1)
	go func() {
		done <- server.handleStreamableHTTPGet(echoCtx)
	}()
	waitForTransport(t, server, sessionID)

	respAny, err := server.handleMessage(jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      10,
		Method:  "tools/call",
		Params:  mustRawMap(t, map[string]any{"name": "reload-prompt-catalog", "arguments": map[string]any{}}),
	}, sessionID)
	if err != nil {
		t.Fatalf("handleMessage tools/call: %v", err)
	}
	resp, ok := respAny.(*jsonrpc.Response)
	if !ok || resp.Error != nil {
		t.Fatalf("expected tool response, got %#v", respAny)
	}
	result := mustMap(t, resp.Result)
	toolPayload := mustMap(t, result["result"])
	changed, ok := toolPayload["changed"].(bool)
	if !ok || !changed {
		t.Fatalf("expected changed=true, got %v", toolPayload["changed"])
	}

	waitForBodyContains(t, rec, "\"method\":\"notifications/prompts/list_changed\"")
	if got := strings.Count(rec.Body.String(), "\"method\":\"notifications/prompts/list_changed\""); got != 1 {
		t.Fatalf("expected one list_changed notification, got %d body=%q", got, rec.Body.String())
	}

	secondRespAny, err := server.handleMessage(jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      11,
		Method:  "tools/call",
		Params:  mustRawMap(t, map[string]any{"name": "reload-prompt-catalog", "arguments": map[string]any{}}),
	}, sessionID)
	if err != nil {
		t.Fatalf("second handleMessage tools/call: %v", err)
	}
	secondResp, ok := secondRespAny.(*jsonrpc.Response)
	if !ok || secondResp.Error != nil {
		t.Fatalf("expected tool response, got %#v", secondRespAny)
	}
	secondResult := mustMap(t, secondResp.Result)
	secondPayload := mustMap(t, secondResult["result"])
	if changed, _ := secondPayload["changed"].(bool); changed {
		t.Fatalf("expected changed=false on second reload, got %v", secondPayload["changed"])
	}
	time.Sleep(100 * time.Millisecond)
	if got := strings.Count(rec.Body.String(), "\"method\":\"notifications/prompts/list_changed\""); got != 1 {
		t.Fatalf("expected no extra list_changed notification, got %d body=%q", got, rec.Body.String())
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("handleStreamableHTTPGet: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE handler shutdown")
	}
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

func mustRawMap(t *testing.T, value map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal raw map: %v", err)
	}
	return raw
}

func waitForTransport(t *testing.T, server *Server, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := server.sessionManager.GetTransport(sessionID); ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for session transport: %s", sessionID)
}

func waitForBodyContains(t *testing.T, rec *httptest.ResponseRecorder, needle string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(rec.Body.String(), needle) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for body content %q, got body=%q", needle, rec.Body.String())
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
