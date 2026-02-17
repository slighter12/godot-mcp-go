package http

import (
	"errors"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/slighter12/godot-mcp-go/promptcatalog"
)

func TestPromptListFingerprint_UnchangedAndChanged(t *testing.T) {
	base := []promptcatalog.Prompt{
		{
			Name:        "scene-review",
			Title:       "Scene Review",
			Description: "desc",
			Arguments: []promptcatalog.PromptArgument{
				{Name: "scene_path", Required: false},
			},
		},
	}

	same := []promptcatalog.Prompt{
		{
			Name:        "scene-review",
			Title:       "Scene Review",
			Description: "desc",
			Arguments: []promptcatalog.PromptArgument{
				{Name: "scene_path", Required: false},
			},
		},
	}

	changed := []promptcatalog.Prompt{
		{
			Name:        "scene-review",
			Title:       "Scene Review Updated",
			Description: "desc",
			Arguments: []promptcatalog.PromptArgument{
				{Name: "scene_path", Required: false},
			},
		},
	}

	if promptListFingerprint(base) != promptListFingerprint(same) {
		t.Fatal("expected equal fingerprint for unchanged prompt list")
	}
	if promptListFingerprint(base) == promptListFingerprint(changed) {
		t.Fatal("expected different fingerprint when visible prompt metadata changes")
	}
}

func TestReloadPromptCatalog_Disabled(t *testing.T) {
	server := newTestHTTPServer(t, false)
	result := server.reloadPromptCatalog()
	if result["status"] != "disabled" {
		t.Fatalf("expected disabled status, got %v", result["status"])
	}
}

func TestReloadPromptCatalog_WarningStatusOnLoadError(t *testing.T) {
	server := newTestHTTPServer(t, true)
	missingPath := filepath.Join(t.TempDir(), "missing")
	server.config.PromptCatalog.Paths = []string{missingPath}

	result := server.reloadPromptCatalog()
	if result["status"] != "warning" {
		t.Fatalf("expected warning status, got %v", result["status"])
	}
	count, ok := result["loadErrorCount"].(int)
	if !ok || count < 1 {
		t.Fatalf("expected positive loadErrorCount, got %T %v", result["loadErrorCount"], result["loadErrorCount"])
	}
	if result["changed"] != false {
		t.Fatalf("expected changed=false for empty-to-empty failed reload, got %v", result["changed"])
	}
}

func TestSendJSONRPCNotificationToSession_DoesNotClearReplacedTransport(t *testing.T) {
	server := newTestHTTPServer(t, true)
	sessionID := "session-race"
	server.sessionManager.CreateSession(sessionID)

	blockingWriter := newBlockingErrorResponseWriter()
	original := NewStreamableHTTPTransport(blockingWriter, blockingWriter)
	if !server.sessionManager.SetTransport(sessionID, original) {
		t.Fatal("failed to bind original transport")
	}

	sendDone := make(chan bool, 1)
	go func() {
		sendDone <- server.SendJSONRPCNotificationToSession(sessionID, map[string]any{
			"jsonrpc": "2.0",
			"method":  "notifications/prompts/list_changed",
		})
	}()

	<-blockingWriter.started

	replacementWriter := &discardResponseWriter{}
	replacement := NewStreamableHTTPTransport(replacementWriter, replacementWriter)

	server.sessionManager.mu.Lock()
	session := server.sessionManager.sessions[sessionID]
	if session == nil {
		server.sessionManager.mu.Unlock()
		t.Fatal("expected session to exist")
	}
	session.Transport = replacement
	server.sessionManager.mu.Unlock()

	close(blockingWriter.release)

	if sent := <-sendDone; sent {
		t.Fatal("expected failed send result")
	}

	current, ok := server.sessionManager.GetTransport(sessionID)
	if !ok {
		t.Fatal("expected replacement transport to remain bound")
	}
	if current != replacement {
		t.Fatalf("expected replacement transport, got %p", current)
	}
}

type blockingErrorResponseWriter struct {
	header  http.Header
	started chan struct{}
	release chan struct{}
}

func newBlockingErrorResponseWriter() *blockingErrorResponseWriter {
	return &blockingErrorResponseWriter{
		header:  make(http.Header),
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (w *blockingErrorResponseWriter) Header() http.Header {
	return w.header
}

func (w *blockingErrorResponseWriter) WriteHeader(_ int) {}

func (w *blockingErrorResponseWriter) Write(_ []byte) (int, error) {
	select {
	case <-w.started:
	default:
		close(w.started)
	}
	<-w.release
	return 0, errors.New("forced write failure")
}

func (w *blockingErrorResponseWriter) Flush() {}

type discardResponseWriter struct {
	header http.Header
}

func (w *discardResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *discardResponseWriter) WriteHeader(_ int) {}

func (w *discardResponseWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (w *discardResponseWriter) Flush() {}
