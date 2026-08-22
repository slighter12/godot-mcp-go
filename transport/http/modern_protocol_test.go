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
	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

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
