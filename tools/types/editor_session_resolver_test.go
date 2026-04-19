package types

import (
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/runtimebridge"
)

func TestResolveFreshEditorSnapshot_UsesLatestFreshWhenCallerMissing(t *testing.T) {
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-a", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, now)

	stored, semErr := ResolveFreshEditorSnapshot(map[string]any{}, MCPContext{
		SessionID:          "ai-session",
		SessionInitialized: true,
	}, "godot.editor.state.get", "Editor state is unavailable until editor snapshot is healthy")
	if semErr != nil {
		t.Fatalf("resolve fresh editor snapshot: %v", semErr)
	}
	if stored.SessionID != "editor-a" {
		t.Fatalf("expected editor-a, got %q", stored.SessionID)
	}
}

func TestResolveFreshEditorSnapshot_UsesExplicitOverride(t *testing.T) {
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-a", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://A.tscn"},
	}, now.Add(2*time.Second))
	runtimebridge.DefaultEditorStore().Upsert("editor-b", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://B.tscn"},
	}, now)

	stored, semErr := ResolveFreshEditorSnapshot(map[string]any{
		"editor_session_id": "editor-b",
	}, MCPContext{
		SessionID:          "ai-session",
		SessionInitialized: true,
	}, "godot.editor.state.get", "Editor state is unavailable until editor snapshot is healthy")
	if semErr != nil {
		t.Fatalf("resolve fresh editor snapshot: %v", semErr)
	}
	if stored.SessionID != "editor-b" {
		t.Fatalf("expected editor-b, got %q", stored.SessionID)
	}
}

func TestResolveFreshEditorSnapshot_RejectsInvalidEditorSessionIDType(t *testing.T) {
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)

	_, semErr := ResolveFreshEditorSnapshot(map[string]any{
		"editor_session_id": 123.0,
	}, MCPContext{
		SessionID:          "ai-session",
		SessionInitialized: true,
	}, "godot.editor.state.get", "Editor state is unavailable until editor snapshot is healthy")
	if semErr == nil {
		t.Fatal("expected semantic error")
	}
	if semErr.Kind != SemanticKindInvalidParams {
		t.Fatalf("expected invalid_params kind, got %s", semErr.Kind)
	}
	if semErr.Data["reason"] != "invalid_editor_session_id_type" {
		t.Fatalf("expected reason invalid_editor_session_id_type, got %v", semErr.Data["reason"])
	}
}

func TestResolveFreshEditorSnapshot_ReturnsNotAvailableWhenNoHealthySession(t *testing.T) {
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)

	_, semErr := ResolveFreshEditorSnapshot(map[string]any{}, MCPContext{
		SessionID:          "ai-session",
		SessionInitialized: true,
	}, "godot.editor.state.get", "Editor state is unavailable until editor snapshot is healthy")
	if semErr == nil {
		t.Fatal("expected semantic error")
	}
	if semErr.Kind != SemanticKindNotAvailable {
		t.Fatalf("expected not_available kind, got %s", semErr.Kind)
	}
	if semErr.Data["code"] != "runtime_snapshot_missing" {
		t.Fatalf("expected code runtime_snapshot_missing, got %v", semErr.Data["code"])
	}
}
