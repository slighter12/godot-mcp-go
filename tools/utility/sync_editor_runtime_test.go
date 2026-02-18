package utility

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

func TestSyncEditorRuntimeTool_RequiresInitializedSession(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	tool := NewSyncEditorRuntimeTool()

	payload := map[string]any{
		"snapshot": map[string]any{
			"root_summary": map[string]any{"active_scene": "res://Main.tscn"},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	_, execErr := tool.Execute(raw)
	if execErr == nil {
		t.Fatal("expected semantic error")
	}
	semanticErr, ok := tooltypes.AsSemanticError(execErr)
	if !ok {
		t.Fatalf("expected semantic error type, got %T", execErr)
	}
	if semanticErr.Kind != tooltypes.SemanticKindNotAvailable {
		t.Fatalf("expected kind %q, got %q", tooltypes.SemanticKindNotAvailable, semanticErr.Kind)
	}
}

func TestSyncEditorRuntimeTool_StoresSnapshot(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	tool := NewSyncEditorRuntimeTool()

	payload := map[string]any{
		"snapshot": map[string]any{
			"root_summary": map[string]any{"active_scene": "res://Main.tscn"},
			"scene_tree":   map[string]any{"path": "/Root", "name": "Root", "type": "Node2D", "child_count": 0},
			"node_details": map[string]any{
				"/Root": map[string]any{"path": "/Root", "name": "Root", "type": "Node2D", "child_count": 0},
			},
		},
		"_mcp": map[string]any{
			"session_id":          "session-ok",
			"session_initialized": true,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	resultRaw, execErr := tool.Execute(raw)
	if execErr != nil {
		t.Fatalf("execute sync tool: %v", execErr)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["synced"] != true {
		t.Fatalf("expected synced=true, got %v", result["synced"])
	}

	stored, ok, reason := runtimebridge.DefaultStore().LatestFresh(time.Now().UTC())
	if !ok {
		t.Fatalf("expected stored runtime snapshot, reason=%s", reason)
	}
	if stored.SessionID != "session-ok" {
		t.Fatalf("expected session-ok, got %s", stored.SessionID)
	}
	if stored.Snapshot.RootSummary.ActiveScene != "res://Main.tscn" {
		t.Fatalf("expected active scene res://Main.tscn, got %s", stored.Snapshot.RootSummary.ActiveScene)
	}
}
