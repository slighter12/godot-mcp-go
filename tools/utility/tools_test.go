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
	runtimebridge.SetSessionInfoProvider(nil)

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

	// Mock session info provider showing 3 sessions
	runtimebridge.SetSessionInfoProvider(&mockSessionInfoProvider{
		counts: map[string]any{
			"total":             3,
			"fully_initialized": 3,
			"with_transport":    3,
		},
	})
	defer runtimebridge.SetSessionInfoProvider(nil)

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

func TestRuntimeHealthTool_IncludesMCPSessions(t *testing.T) {
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultRuntimeLogStoreForTests(50)
	runtimebridge.ResetDefaultCommandBrokerForTests(500 * time.Millisecond)

	runtimebridge.SetSessionInfoProvider(&mockSessionInfoProvider{
		counts: map[string]any{
			"total":             2,
			"fully_initialized": 2,
			"with_transport":    1,
		},
		summaries: []map[string]any{
			{"session_id": "s1", "initialized": true},
			{"session_id": "s2", "initialized": true},
		},
	})
	defer runtimebridge.SetSessionInfoProvider(nil)

	tool := NewRuntimeHealthTool()
	resultRaw, err := tool.Execute(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute runtime.health.get: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	mcpSessions, ok := result["mcp_sessions"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcp_sessions map, got %T", result["mcp_sessions"])
	}
	if mcpSessions["total"] != float64(2) {
		t.Fatalf("expected total=2, got %v", mcpSessions["total"])
	}

	details, ok := result["mcp_session_details"].([]any)
	if !ok {
		t.Fatalf("expected mcp_session_details array, got %T", result["mcp_session_details"])
	}
	if len(details) != 2 {
		t.Fatalf("expected 2 session details, got %d", len(details))
	}
}

// mockSessionInfoProvider implements runtimebridge.SessionInfoProvider for tests.
type mockSessionInfoProvider struct {
	counts    map[string]any
	summaries []map[string]any
}

func (m *mockSessionInfoProvider) SessionSummaries() []map[string]any {
	return m.summaries
}

func (m *mockSessionInfoProvider) SessionCounts() map[string]any {
	return m.counts
}
