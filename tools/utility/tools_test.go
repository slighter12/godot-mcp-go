package utility

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/runtimebridge"
)

func TestRuntimeDiagnoseTool_ReturnsChecklist(t *testing.T) {
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)

	// Set up game session + editor but no runtime companion
	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, now)
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game-1", "editor-1", "res://Main.tscn", "token-1", now)

	tool := NewRuntimeDiagnoseTool()
	resultRaw, err := tool.Execute(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute runtime.diagnose: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	checklist, ok := result["pipeline_checklist"].([]any)
	if !ok {
		t.Fatalf("expected pipeline_checklist array, got %T", result["pipeline_checklist"])
	}
	if len(checklist) != 5 {
		t.Fatalf("expected 5 checklist steps, got %d", len(checklist))
	}

	// Step 1: game_session_exists should be true
	step1, _ := checklist[0].(map[string]any)
	if step1["step"] != "game_session_exists" || step1["ok"] != true {
		t.Fatalf("expected game_session_exists=true, got %v", step1)
	}

	// Step 2: editor_session_fresh should be true
	step2, _ := checklist[1].(map[string]any)
	if step2["step"] != "editor_session_fresh" || step2["ok"] != true {
		t.Fatalf("expected editor_session_fresh=true, got %v", step2)
	}

	// Step 3: runtime_session_connected should be false (no session info provider)
	step3, _ := checklist[2].(map[string]any)
	if step3["step"] != "runtime_session_connected" || step3["ok"] != false {
		t.Fatalf("expected runtime_session_connected=false, got %v", step3)
	}

	// Step 4: runtime_session_registered should be false
	step4, _ := checklist[3].(map[string]any)
	if step4["step"] != "runtime_session_registered" || step4["ok"] != false {
		t.Fatalf("expected runtime_session_registered=false, got %v", step4)
	}

	// Step 5: first_snapshot_received should be false
	step5, _ := checklist[4].(map[string]any)
	if step5["step"] != "first_snapshot_received" || step5["ok"] != false {
		t.Fatalf("expected first_snapshot_received=false, got %v", step5)
	}
}

func TestRuntimeDiagnoseTool_AllGreen(t *testing.T) {
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)

	now := time.Now().UTC()

	// Editor session
	runtimebridge.DefaultEditorStore().Upsert("editor-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, now)

	// Game session with runtime registered
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game-1", "editor-1", "res://Main.tscn", "token-1", now)
	runtimebridge.DefaultGameSessionRegistry().RegisterRuntimeTransport("game-1", "runtime-1", "editor-1", "res://Main.tscn", now, "token-1")

	// Snapshot received
	runtimebridge.DefaultRuntimeSnapshotStore().Upsert("game-1", runtimebridge.RuntimeSnapshot{
		SessionID:  "game-1",
		SnapshotID: "snap_1",
		Running:    true,
		NodeCount:  1,
	}, now)
	runtimebridge.DefaultGameSessionRegistry().MarkSnapshotReceived("game-1", now)

	tool := NewRuntimeDiagnoseTool()
	resultRaw, err := tool.Execute(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute runtime.diagnose: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	checklist, ok := result["pipeline_checklist"].([]any)
	if !ok {
		t.Fatalf("expected pipeline_checklist array, got %T", result["pipeline_checklist"])
	}
	for i, step := range checklist {
		stepMap, _ := step.(map[string]any)
		if stepMap["ok"] != true {
			t.Fatalf("expected step %d (%s) to be ok=true, got %v", i, stepMap["step"], stepMap)
		}
		if _, hasHint := stepMap["hint"]; hasHint {
			t.Fatalf("expected no hint for passing step %d (%s), got %v", i, stepMap["step"], stepMap["hint"])
		}
	}
}

func TestRuntimeHealthTool_ReportsStatelessTransport(t *testing.T) {
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultRuntimeLogStoreForTests(50)
	runtimebridge.ResetDefaultCommandBrokerForTests(500 * time.Millisecond)

	tool := NewRuntimeHealthTool()
	resultRaw, err := tool.Execute(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute runtime.health.get: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	transport, ok := result["transport"].(map[string]any)
	if !ok {
		t.Fatalf("expected transport map, got %T", result["transport"])
	}
	if transport["protocol_version"] != "2026-07-28" {
		t.Fatalf("expected modern protocol version, got %v", transport["protocol_version"])
	}
	if transport["session_model"] != "stateless" {
		t.Fatalf("expected stateless transport model, got %v", transport["session_model"])
	}
}
