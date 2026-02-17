package http

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestReloadPromptCatalogIfSourcesChanged_AddModifyDelete(t *testing.T) {
	server := newTestHTTPServer(t, true)
	root := t.TempDir()
	server.config.PromptCatalog.Paths = []string{root}
	server.config.PromptCatalog.AllowedRoots = []string{root}

	initial := server.reloadPromptCatalog()
	if initial["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", initial["status"])
	}

	if result, reloaded := server.reloadPromptCatalogIfSourcesChanged(); reloaded {
		t.Fatalf("did not expect reload without source changes, got %#v", result)
	}

	skill := filepath.Join(root, "scene-review", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	contentV1 := "---\nname: scene-review\ndescription: Prompt v1\n---\nReview {{scene_path}}\n"
	if err := os.WriteFile(skill, []byte(contentV1), 0644); err != nil {
		t.Fatalf("write skill v1: %v", err)
	}

	addResult, reloaded := server.reloadPromptCatalogIfSourcesChanged()
	if !reloaded {
		t.Fatalf("expected reload after adding skill file")
	}
	if addResult["changed"] != true {
		t.Fatalf("expected changed=true after add, got %v", addResult["changed"])
	}

	if result, reloaded := server.reloadPromptCatalogIfSourcesChanged(); reloaded {
		t.Fatalf("did not expect second reload without changes, got %#v", result)
	}

	time.Sleep(10 * time.Millisecond)
	contentV2 := "---\nname: scene-review\ndescription: Prompt v2\n---\nReview {{scene_path}}\n"
	if err := os.WriteFile(skill, []byte(contentV2), 0644); err != nil {
		t.Fatalf("write skill v2: %v", err)
	}
	modifyResult, reloaded := server.reloadPromptCatalogIfSourcesChanged()
	if !reloaded {
		t.Fatalf("expected reload after modifying skill file")
	}
	if modifyResult["changed"] != true {
		t.Fatalf("expected changed=true after modify, got %v", modifyResult["changed"])
	}

	time.Sleep(10 * time.Millisecond)
	if err := os.Remove(skill); err != nil {
		t.Fatalf("remove skill file: %v", err)
	}
	deleteResult, reloaded := server.reloadPromptCatalogIfSourcesChanged()
	if !reloaded {
		t.Fatalf("expected reload after deleting skill file")
	}
	if deleteResult["changed"] != true {
		t.Fatalf("expected changed=true after delete, got %v", deleteResult["changed"])
	}
	if deleteResult["promptCount"] != 0 {
		t.Fatalf("expected promptCount=0 after delete, got %v", deleteResult["promptCount"])
	}
}

func TestPromptCatalogAutoReloadLifecycle_StopClearsRunner(t *testing.T) {
	server := newTestHTTPServer(t, true)
	server.config.PromptCatalog.AutoReload.Enabled = true
	server.config.PromptCatalog.AutoReload.IntervalSeconds = 2

	server.startPromptCatalogAutoReload()

	server.promptCatalogAutoReloadMu.Lock()
	cancel := server.promptCatalogAutoReloadCancel
	done := server.promptCatalogAutoReloadDone
	server.promptCatalogAutoReloadMu.Unlock()
	if cancel == nil || done == nil {
		t.Fatal("expected auto-reload runner to be initialized")
	}

	server.stopPromptCatalogAutoReload()
	server.stopPromptCatalogAutoReload()

	server.promptCatalogAutoReloadMu.Lock()
	cancel = server.promptCatalogAutoReloadCancel
	done = server.promptCatalogAutoReloadDone
	server.promptCatalogAutoReloadMu.Unlock()
	if cancel != nil || done != nil {
		t.Fatal("expected auto-reload runner to be fully cleared after stop")
	}
}

func TestLogPromptCatalogSnapshotWarningsLocked_DeduplicatesAndRecovers(t *testing.T) {
	server := newTestHTTPServer(t, true)
	warnings := []string{"  path not found  ", "permission denied"}

	server.promptCatalogReloadMu.Lock()
	server.logPromptCatalogSnapshotWarningsLocked(warnings)
	firstFingerprint := server.promptCatalogSnapshotWarningFingerprint
	firstLogged := server.promptCatalogSnapshotWarningLastLogged
	server.promptCatalogReloadMu.Unlock()
	if firstFingerprint == "" {
		t.Fatal("expected warning fingerprint after first warning emission")
	}
	if firstLogged.IsZero() {
		t.Fatal("expected non-zero last-logged timestamp")
	}

	server.promptCatalogReloadMu.Lock()
	server.logPromptCatalogSnapshotWarningsLocked([]string{"permission denied", "path not found"})
	secondLogged := server.promptCatalogSnapshotWarningLastLogged
	server.promptCatalogReloadMu.Unlock()
	if !secondLogged.Equal(firstLogged) {
		t.Fatalf("expected duplicate warning set to be suppressed, first=%v second=%v", firstLogged, secondLogged)
	}

	server.promptCatalogReloadMu.Lock()
	server.promptCatalogSnapshotWarningLastLogged = time.Now().Add(-snapshotWarningHeartbeatInterval - time.Second)
	previous := server.promptCatalogSnapshotWarningLastLogged
	server.logPromptCatalogSnapshotWarningsLocked([]string{"permission denied", "path not found"})
	thirdLogged := server.promptCatalogSnapshotWarningLastLogged
	server.promptCatalogReloadMu.Unlock()
	if !thirdLogged.After(previous) {
		t.Fatalf("expected warning heartbeat to refresh timestamp, previous=%v third=%v", previous, thirdLogged)
	}

	server.promptCatalogReloadMu.Lock()
	server.logPromptCatalogSnapshotWarningsLocked(nil)
	clearedFingerprint := server.promptCatalogSnapshotWarningFingerprint
	clearedLogged := server.promptCatalogSnapshotWarningLastLogged
	server.promptCatalogReloadMu.Unlock()
	if clearedFingerprint != "" {
		t.Fatalf("expected cleared warning fingerprint, got %q", clearedFingerprint)
	}
	if !clearedLogged.IsZero() {
		t.Fatalf("expected cleared warning timestamp, got %v", clearedLogged)
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
