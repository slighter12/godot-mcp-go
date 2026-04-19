package runtime

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

func TestBridgeEditorSyncTool_StoresEditorSnapshot(t *testing.T) {
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)

	tool := &BridgeEditorSyncTool{}
	raw := json.RawMessage(`{
		"snapshot":{"root_summary":{"active_scene":"res://Main.tscn"}},
		"_mcp":{"session_id":"editor-1","session_initialized":true}
	}`)
	if _, err := tool.Execute(raw); err != nil {
		t.Fatalf("execute godot.bridge.editor.sync: %v", err)
	}

	stored, ok, reason := runtimebridge.DefaultEditorStore().FreshForSession("editor-1", time.Now().UTC())
	if !ok {
		t.Fatalf("expected fresh snapshot, reason=%s", reason)
	}
	if stored.Snapshot.RootSummary.ActiveScene != "res://Main.tscn" {
		t.Fatalf("expected active scene res://Main.tscn, got %q", stored.Snapshot.RootSummary.ActiveScene)
	}
}

func TestGetActiveGameSessionTool_ResolvesLatestFreshEditorSessionAcrossCallerSessions(t *testing.T) {
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-owner", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, now)
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_owner", "editor-owner", "res://Main.tscn", "launch-token", now)

	tool := &GetActiveGameSessionTool{}
	raw := json.RawMessage(`{
		"_mcp":{"session_id":"ai-session","session_initialized":true}
	}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.runtime.session.get_active: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["session_id"] != "game_owner" {
		t.Fatalf("expected session_id=game_owner, got %v", result["session_id"])
	}
	if result["editor_session_id"] != "editor-owner" {
		t.Fatalf("expected editor_session_id=editor-owner, got %v", result["editor_session_id"])
	}
	if result["running"] != true {
		t.Fatalf("expected running=true, got %v", result["running"])
	}
}

func TestGetActiveGameSessionTool_UsesExplicitEditorSessionOverride(t *testing.T) {
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-a", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://SceneA.tscn"},
	}, now.Add(2*time.Second))
	runtimebridge.DefaultEditorStore().Upsert("editor-b", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://SceneB.tscn"},
	}, now)
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_b", "editor-b", "res://SceneB.tscn", "launch-token-b", now)

	tool := &GetActiveGameSessionTool{}
	raw := json.RawMessage(`{
		"editor_session_id":"editor-b",
		"_mcp":{"session_id":"ai-session","session_initialized":true}
	}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.runtime.session.get_active: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["session_id"] != "game_b" {
		t.Fatalf("expected session_id=game_b, got %v", result["session_id"])
	}
	if result["editor_session_id"] != "editor-b" {
		t.Fatalf("expected editor_session_id=editor-b, got %v", result["editor_session_id"])
	}
}

func TestGetActiveGameSessionTool_ReturnsNotAvailableWhenNoHealthyEditorSession(t *testing.T) {
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()

	tool := &GetActiveGameSessionTool{}
	raw := json.RawMessage(`{
		"_mcp":{"session_id":"ai-session","session_initialized":true}
	}`)
	_, err := tool.Execute(raw)
	if err == nil {
		t.Fatal("expected semantic error")
	}
	semanticErr, ok := tooltypes.AsSemanticError(err)
	if !ok {
		t.Fatalf("expected semantic error, got %T", err)
	}
	if semanticErr.Kind != tooltypes.SemanticKindNotAvailable {
		t.Fatalf("expected not_available kind, got %s", semanticErr.Kind)
	}
	if semanticErr.Data["code"] != "runtime_snapshot_missing" {
		t.Fatalf("expected code runtime_snapshot_missing, got %v", semanticErr.Data["code"])
	}
}

func TestGetActiveGameSessionTool_RejectsInvalidEditorSessionOverrideType(t *testing.T) {
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()

	tool := &GetActiveGameSessionTool{}
	raw := json.RawMessage(`{
		"editor_session_id":123,
		"_mcp":{"session_id":"ai-session","session_initialized":true}
	}`)
	_, err := tool.Execute(raw)
	if err == nil {
		t.Fatal("expected semantic error")
	}
	semanticErr, ok := tooltypes.AsSemanticError(err)
	if !ok {
		t.Fatalf("expected semantic error, got %T", err)
	}
	if semanticErr.Kind != tooltypes.SemanticKindInvalidParams {
		t.Fatalf("expected invalid_params kind, got %s", semanticErr.Kind)
	}
}

func TestBridgeRuntimeRegisterTool_RejectsLaunchTokenMismatch(t *testing.T) {
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_1", "editor-1", "res://Main.tscn", "launch_ok", time.Now().UTC())

	tool := &BridgeRuntimeRegisterTool{}
	raw := json.RawMessage(`{
		"session_id":"game_1",
		"editor_session_id":"editor-1",
		"launch_token":"launch_bad",
		"_mcp":{"session_id":"runtime-1","session_initialized":true}
	}`)
	_, err := tool.Execute(raw)
	if err == nil {
		t.Fatal("expected semantic error")
	}
	semanticErr, ok := tooltypes.AsSemanticError(err)
	if !ok {
		t.Fatalf("expected semantic error, got %T", err)
	}
	if semanticErr.Data["code"] != "game_session_missing" {
		t.Fatalf("expected code game_session_missing, got %v", semanticErr.Data["code"])
	}
}

func TestBridgeRuntimeSnapshotPushTool_RejectsMismatchedRuntimeSession(t *testing.T) {
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)
	now := time.Now().UTC()
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_1", "editor-1", "res://Main.tscn", "launch_ok", now)
	runtimebridge.DefaultGameSessionRegistry().RegisterRuntimeTransport("game_1", "runtime-1", "editor-1", "res://Main.tscn", now, "launch_ok")

	tool := &BridgeRuntimeSnapshotPushTool{}
	raw := json.RawMessage(`{
		"session_id":"game_1",
		"snapshot":{"snapshot_id":"snap_1","frame":1,"running":true},
		"_mcp":{"session_id":"runtime-2","session_initialized":true}
	}`)
	_, err := tool.Execute(raw)
	if err == nil {
		t.Fatal("expected semantic error")
	}
	semanticErr, ok := tooltypes.AsSemanticError(err)
	if !ok {
		t.Fatalf("expected semantic error, got %T", err)
	}
	if semanticErr.Data["code"] != "game_session_missing" {
		t.Fatalf("expected code game_session_missing, got %v", semanticErr.Data["code"])
	}
}

func TestAwaitRuntimeSnapshotTool_ReturnsSnapshotMetadata(t *testing.T) {
	runtimebridge.ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)

	now := time.Now().UTC()
	runtimebridge.DefaultRuntimeSnapshotStore().Upsert("game_1", runtimebridge.RuntimeSnapshot{
		SessionID:     "game_1",
		SnapshotID:    "snap_1",
		Frame:         24,
		UpdatedAt:     now.Format(time.RFC3339Nano),
		RootScenePath: "res://scenes/mvp0/mvp0_root.tscn",
		RootNodeName:  "Main",
		NodeCount:     12,
		Running:       true,
	}, now)

	tool := &AwaitRuntimeSnapshotTool{}
	raw := json.RawMessage(`{
		"session_id":"game_1",
		"min_frame":10,
		"timeout_ms":200,
		"_mcp":{"session_id":"editor-1","session_initialized":true}
	}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.runtime.await_snapshot: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["snapshot_id"] != "snap_1" {
		t.Fatalf("expected snapshot_id=snap_1, got %v", result["snapshot_id"])
	}
	if result["frame"] != float64(24) {
		t.Fatalf("expected frame=24, got %v", result["frame"])
	}
}

func TestAwaitRuntimeSnapshotTool_RejectsInvalidFreshness(t *testing.T) {
	tool := &AwaitRuntimeSnapshotTool{}
	raw := json.RawMessage(`{
		"session_id":"game_1",
		"freshness":"newest",
		"_mcp":{"session_id":"editor-1","session_initialized":true}
	}`)
	_, err := tool.Execute(raw)
	if err == nil {
		t.Fatal("expected semantic error")
	}
	semanticErr, ok := tooltypes.AsSemanticError(err)
	if !ok {
		t.Fatalf("expected semantic error, got %T", err)
	}
	if semanticErr.Kind != tooltypes.SemanticKindInvalidParams {
		t.Fatalf("expected invalid_params kind, got %s", semanticErr.Kind)
	}
}

func TestRuntimeSceneTreeGetTool_ReturnsRuntimeSnapshotMissingCode(t *testing.T) {
	runtimebridge.ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)

	tool := &RuntimeSceneTreeGetTool{}
	raw := json.RawMessage(`{
		"session_id":"game_missing",
		"_mcp":{"session_id":"editor-1","session_initialized":true}
	}`)
	_, err := tool.Execute(raw)
	if err == nil {
		t.Fatal("expected semantic error")
	}
	semanticErr, ok := tooltypes.AsSemanticError(err)
	if !ok {
		t.Fatalf("expected semantic error, got %T", err)
	}
	if semanticErr.Data["code"] != "runtime_snapshot_missing" {
		t.Fatalf("expected code runtime_snapshot_missing, got %v", semanticErr.Data["code"])
	}
}

func TestRuntimeInputPressTool_ReturnsGameSessionMissingWithoutRuntimeTransport(t *testing.T) {
	runtimebridge.ResetDefaultGameSessionRegistryForTests()

	tool := &RuntimeInputPressTool{}
	raw := json.RawMessage(`{
		"session_id":"game_1",
		"input":"ui_right",
		"_mcp":{"session_id":"editor-1","session_initialized":true}
	}`)
	_, err := tool.Execute(raw)
	if err == nil {
		t.Fatal("expected semantic error")
	}
	semanticErr, ok := tooltypes.AsSemanticError(err)
	if !ok {
		t.Fatalf("expected semantic error, got %T", err)
	}
	if semanticErr.Data["code"] != "game_session_missing" {
		t.Fatalf("expected code game_session_missing, got %v", semanticErr.Data["code"])
	}
}

func TestRuntimeLogGetTool_RejectsMissingGameSession(t *testing.T) {
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultRuntimeLogStoreForTests(100)

	tool := &RuntimeLogGetTool{}
	raw := json.RawMessage(`{
		"session_id":"game_missing",
		"_mcp":{"session_id":"editor-1","session_initialized":true}
	}`)
	_, err := tool.Execute(raw)
	if err == nil {
		t.Fatal("expected semantic error")
	}
	semanticErr, ok := tooltypes.AsSemanticError(err)
	if !ok {
		t.Fatalf("expected semantic error, got %T", err)
	}
	if semanticErr.Data["code"] != "game_session_missing" {
		t.Fatalf("expected code game_session_missing, got %v", semanticErr.Data["code"])
	}
}

func TestRuntimeLogGetTool_RejectsStoppedGameSession(t *testing.T) {
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultRuntimeLogStoreForTests(100)

	now := time.Now().UTC()
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_1", "editor-1", "res://Main.tscn", "launch-token", now)
	runtimebridge.DefaultGameSessionRegistry().StopSession("game_1", now.Add(time.Second))

	tool := &RuntimeLogGetTool{}
	raw := json.RawMessage(`{
		"session_id":"game_1",
		"_mcp":{"session_id":"editor-1","session_initialized":true}
	}`)
	_, err := tool.Execute(raw)
	if err == nil {
		t.Fatal("expected semantic error")
	}
	semanticErr, ok := tooltypes.AsSemanticError(err)
	if !ok {
		t.Fatalf("expected semantic error, got %T", err)
	}
	if semanticErr.Data["code"] != "game_not_running" {
		t.Fatalf("expected code game_not_running, got %v", semanticErr.Data["code"])
	}
}

func TestRuntimeLogGetTool_FiltersByErrorLevelAndSinceSequence(t *testing.T) {
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultRuntimeLogStoreForTests(100)

	now := time.Now().UTC()
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_1", "editor-1", "res://Main.tscn", "launch-token", now)
	runtimebridge.DefaultGameSessionRegistry().RegisterRuntimeTransport("game_1", "runtime-1", "editor-1", "res://Main.tscn", now, "launch-token")

	appended := runtimebridge.DefaultRuntimeLogStore().Append("game_1", []runtimebridge.RuntimeLogAppendEntry{
		{Level: "info", Message: "boot", Source: "runtime_companion"},
		{Level: "error", Message: "boom-1", Source: "runtime_command:godot.runtime.input.tap"},
		{Level: "warning", Message: "warn", Source: "runtime_companion"},
		{Level: "error", Message: "boom-2", Source: "runtime_command:godot.runtime.screenshot.get"},
	}, now)
	if len(appended) != 4 {
		t.Fatalf("expected 4 appended entries, got %d", len(appended))
	}

	tool := &RuntimeLogGetTool{}
	raw := json.RawMessage(`{
		"session_id":"game_1",
		"level":"error",
		"since_sequence":2,
		"limit":10,
		"_mcp":{"session_id":"editor-1","session_initialized":true}
	}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.runtime.log.get: %v", err)
	}

	var result struct {
		Source    string                          `json:"source"`
		SessionID string                          `json:"session_id"`
		Entries   []runtimebridge.RuntimeLogEntry `json:"entries"`
	}
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Source != "runtime" {
		t.Fatalf("expected source runtime, got %q", result.Source)
	}
	if result.SessionID != "game_1" {
		t.Fatalf("expected session_id game_1, got %q", result.SessionID)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 filtered entry, got %d", len(result.Entries))
	}
	if result.Entries[0].Message != "boom-2" {
		t.Fatalf("expected message boom-2, got %q", result.Entries[0].Message)
	}
	if result.Entries[0].Level != "error" {
		t.Fatalf("expected level error, got %q", result.Entries[0].Level)
	}
	if result.Entries[0].Sequence <= appended[1].Sequence {
		t.Fatalf("expected sequence after %d, got %d", appended[1].Sequence, result.Entries[0].Sequence)
	}
}

func TestRuntimeLogGetTool_ReturnsEmptyEntriesForCleanRunningSession(t *testing.T) {
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultRuntimeLogStoreForTests(100)

	now := time.Now().UTC()
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_1", "editor-1", "res://Main.tscn", "launch-token", now)
	runtimebridge.DefaultGameSessionRegistry().RegisterRuntimeTransport("game_1", "runtime-1", "editor-1", "res://Main.tscn", now, "launch-token")

	tool := &RuntimeLogGetTool{}
	raw := json.RawMessage(`{
		"session_id":"game_1",
		"level":"error",
		"limit":10,
		"_mcp":{"session_id":"editor-1","session_initialized":true}
	}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.runtime.log.get: %v", err)
	}

	var result struct {
		Entries []runtimebridge.RuntimeLogEntry `json:"entries"`
	}
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Entries) != 0 {
		t.Fatalf("expected empty entries for clean run, got %d", len(result.Entries))
	}
}

func TestRuntimeLogClearTool_DispatchesAndClearsBuffer(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultRuntimeLogStoreForTests(100)

	now := time.Now().UTC()
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_1", "editor-1", "res://Main.tscn", "launch-token", now)
	runtimebridge.DefaultGameSessionRegistry().RegisterRuntimeTransport("game_1", "runtime-1", "editor-1", "res://Main.tscn", now, "launch-token")
	runtimebridge.DefaultRuntimeLogStore().Append("game_1", []runtimebridge.RuntimeLogAppendEntry{
		{Level: "error", Message: "boom"},
		{Level: "warning", Message: "warn"},
	}, now)

	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		go func() {
			runtimebridge.DefaultCommandBroker().Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result:    map[string]any{"cleared": true},
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	tool := &RuntimeLogClearTool{}
	raw := json.RawMessage(`{
		"session_id":"game_1",
		"_mcp":{"session_id":"editor-1","session_initialized":true}
	}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.runtime.log.clear: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["cleared"] != float64(2) {
		t.Fatalf("expected cleared=2, got %v", result["cleared"])
	}

	entries := runtimebridge.DefaultRuntimeLogStore().Get("game_1", "all", 50, 0)
	if len(entries) != 0 {
		t.Fatalf("expected cleared runtime log buffer, got %d entries", len(entries))
	}
}
