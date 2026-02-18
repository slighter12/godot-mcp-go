package utility

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

func TestPingEditorRuntimeTool_RequiresInitializedSession(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	tool := NewPingEditorRuntimeTool()

	raw, err := json.Marshal(map[string]any{})
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

func TestPingEditorRuntimeTool_RequiresExistingSnapshot(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	tool := NewPingEditorRuntimeTool()

	raw, err := json.Marshal(map[string]any{
		"_mcp": map[string]any{
			"session_id":          "session-ok",
			"session_initialized": true,
		},
	})
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

func TestPingEditorRuntimeTool_TouchesSnapshot(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	store := runtimebridge.DefaultStore()
	tool := NewPingEditorRuntimeTool()

	base := time.Now().UTC().Add(-9 * time.Second)
	store.Upsert("session-ok", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
		SceneTree:   runtimebridge.CompactNode{Path: "/Root", Name: "Root", Type: "Node2D", ChildCount: 0},
		NodeDetails: map[string]runtimebridge.NodeDetail{
			"/Root": {Path: "/Root", Name: "Root", Type: "Node2D", ChildCount: 0},
		},
	}, base)

	raw, err := json.Marshal(map[string]any{
		"_mcp": map[string]any{
			"session_id":          "session-ok",
			"session_initialized": true,
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	resultRaw, execErr := tool.Execute(raw)
	if execErr != nil {
		t.Fatalf("execute ping tool: %v", execErr)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["pong"] != true {
		t.Fatalf("expected pong=true, got %v", result["pong"])
	}

	stored, ok, reason := store.LatestFresh(time.Now().UTC())
	if !ok {
		t.Fatalf("expected fresh snapshot after ping, reason=%s", reason)
	}
	if stored.Snapshot.RootSummary.ActiveScene != "res://Main.tscn" {
		t.Fatalf("expected snapshot unchanged, got %s", stored.Snapshot.RootSummary.ActiveScene)
	}
}
