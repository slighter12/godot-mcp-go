package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

func TestGetProjectSettingsTool_ReturnsParsedEntries(t *testing.T) {
	projectRoot := t.TempDir()
	projectFile := filepath.Join(projectRoot, "project.godot")
	content := strings.Join([]string{
		"; comment",
		"[application]",
		"config/name=\"Demo\"",
		"run/main_scene=\"res://scenes/Main.tscn\"",
		"[physics]",
		"common/physics_ticks_per_second=60",
		"",
	}, "\n")
	if err := os.WriteFile(projectFile, []byte(content), 0644); err != nil {
		t.Fatalf("write project.godot: %v", err)
	}

	t.Setenv("GODOT_PROJECT_ROOT", projectRoot)
	tool := &GetProjectSettingsTool{}
	resultRaw, err := tool.Execute(json.RawMessage(`{"section_prefix":"application"}`))
	if err != nil {
		t.Fatalf("execute godot.project.settings.get: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	settingsRaw, ok := result["settings"].([]any)
	if !ok || len(settingsRaw) == 0 {
		t.Fatalf("expected non-empty settings list, got %T %v", result["settings"], result["settings"])
	}
}

func TestListProjectResourcesTool_FiltersByExtension(t *testing.T) {
	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectRoot, "project.godot"), []byte("[application]\n"), 0644); err != nil {
		t.Fatalf("write project.godot: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, "scenes"), 0755); err != nil {
		t.Fatalf("mkdir scenes: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, "scripts"), 0755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "scenes", "Main.tscn"), []byte("[gd_scene]\n"), 0644); err != nil {
		t.Fatalf("write scene: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "scripts", "Main.gd"), []byte("extends Node\n"), 0644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	t.Setenv("GODOT_PROJECT_ROOT", projectRoot)
	tool := &ListProjectResourcesTool{}
	resultRaw, err := tool.Execute(json.RawMessage(`{"extensions":[".gd"]}`))
	if err != nil {
		t.Fatalf("execute godot.project.resources.list: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	resourcesRaw, ok := result["resources"].([]any)
	if !ok || len(resourcesRaw) != 1 {
		t.Fatalf("expected one filtered resource, got %T %v", result["resources"], result["resources"])
	}
	entry, ok := resourcesRaw[0].(map[string]any)
	if !ok {
		t.Fatalf("expected resource entry map, got %T", resourcesRaw[0])
	}
	if entry["extension"] != ".gd" {
		t.Fatalf("expected extension .gd, got %v", entry["extension"])
	}
}

func TestGetEditorStateTool_UsesRuntimeSnapshot(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	runtimebridge.DefaultEditorStore().Upsert("session-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{
			ActiveScene:  "res://Main.tscn",
			ActiveScript: "res://scripts/Main.gd",
		},
	}, time.Now().UTC())

	tool := &GetEditorStateTool{}
	resultRaw, err := tool.Execute(json.RawMessage(`{"_mcp":{"session_id":"session-1","session_initialized":true}}`))
	if err != nil {
		t.Fatalf("execute godot.editor.state.get: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["active_scene"] != "res://Main.tscn" {
		t.Fatalf("expected active scene res://Main.tscn, got %v", result["active_scene"])
	}
}

func TestGetEditorStateTool_IsSessionScoped(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("session-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{
			ActiveScene: "res://SceneA.tscn",
		},
	}, now)
	runtimebridge.DefaultEditorStore().Upsert("session-2", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{
			ActiveScene: "res://SceneB.tscn",
		},
	}, now.Add(1*time.Second))

	tool := &GetEditorStateTool{}
	resultRaw, err := tool.Execute(json.RawMessage(`{"_mcp":{"session_id":"session-1","session_initialized":true}}`))
	if err != nil {
		t.Fatalf("execute godot.editor.state.get: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["active_scene"] != "res://SceneA.tscn" {
		t.Fatalf("expected active scene res://SceneA.tscn, got %v", result["active_scene"])
	}
}

func TestGetEditorStateTool_UsesLatestFreshEditorSessionWhenCallerHasNoSnapshot(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-a", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{
			ActiveScene: "res://SceneA.tscn",
		},
	}, now)

	tool := &GetEditorStateTool{}
	resultRaw, err := tool.Execute(json.RawMessage(`{"_mcp":{"session_id":"ai-session-b","session_initialized":true}}`))
	if err != nil {
		t.Fatalf("execute godot.editor.state.get: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["active_scene"] != "res://SceneA.tscn" {
		t.Fatalf("expected active scene res://SceneA.tscn, got %v", result["active_scene"])
	}
	if result["session_id"] != "editor-a" {
		t.Fatalf("expected session_id=editor-a, got %v", result["session_id"])
	}
}

func TestGetEditorStateTool_UsesExplicitEditorSessionOverride(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-a", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://SceneA.tscn"},
	}, now.Add(2*time.Second))
	runtimebridge.DefaultEditorStore().Upsert("editor-b", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://SceneB.tscn"},
	}, now)

	tool := &GetEditorStateTool{}
	raw := json.RawMessage(`{"editor_session_id":"editor-b","_mcp":{"session_id":"ai-session","session_initialized":true}}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.editor.state.get: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["active_scene"] != "res://SceneB.tscn" {
		t.Fatalf("expected active scene res://SceneB.tscn, got %v", result["active_scene"])
	}
	if result["session_id"] != "editor-b" {
		t.Fatalf("expected session_id=editor-b, got %v", result["session_id"])
	}
}

func TestGetEditorStateTool_ReturnsErrorWithoutHealthyEditorSession(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)

	tool := &GetEditorStateTool{}
	_, err := tool.Execute(json.RawMessage(`{"_mcp":{"session_id":"ai-session","session_initialized":true}}`))
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

func TestRunProject_RequiresInitializedSession(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.SetNotificationSender(nil)
	defer runtimebridge.SetNotificationSender(nil)

	tool := &RunProjectTool{}
	_, err := tool.Execute(json.RawMessage(`{}`))
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
}

func TestRunProject_DispatchesRuntimeCommand(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.DefaultEditorStore().Upsert("session-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, time.Now().UTC())
	broker := runtimebridge.DefaultCommandBroker()
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		arguments, _ := params["arguments"].(map[string]any)
		gameSessionID, _ := arguments["session_id"].(string)
		go func() {
			if gameSessionID != "" {
				now := time.Now().UTC()
				runtimebridge.DefaultGameSessionRegistry().RegisterRuntimeTransport(gameSessionID, sessionID, "session-1", "res://Main.tscn", now, "test-launch")
				runtimebridge.DefaultRuntimeSnapshotStore().Upsert(gameSessionID, runtimebridge.RuntimeSnapshot{
					SessionID:     gameSessionID,
					SnapshotID:    "snap_1",
					Frame:         1,
					UpdatedAt:     now.Format(time.RFC3339Nano),
					RootScenePath: "res://Main.tscn",
					RootNodeName:  "Main",
					NodeCount:     1,
					Running:       true,
				}, now)
			}
			broker.Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result: map[string]any{
					"running":    true,
					"session_id": gameSessionID,
					"scene_path": "res://Main.tscn",
				},
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	tool := &RunProjectTool{}
	payload := map[string]any{
		"_mcp": map[string]any{
			"session_id":          "session-1",
			"session_initialized": true,
		},
	}
	raw, _ := json.Marshal(payload)

	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.project.run: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["success"] != true {
		t.Fatalf("expected success=true, got %v", result["success"])
	}
	if result["running"] != true {
		t.Fatalf("expected running=true, got %v", result["running"])
	}
	if result["scene_path"] != "res://Main.tscn" {
		t.Fatalf("expected scene_path res://Main.tscn, got %v", result["scene_path"])
	}

	sessionID, ok := result["session_id"].(string)
	if !ok || sessionID == "" {
		t.Fatalf("expected non-empty session_id, got %v", result["session_id"])
	}
	session, exists := runtimebridge.DefaultGameSessionRegistry().Session(sessionID)
	if !exists {
		t.Fatalf("expected session %q in registry", sessionID)
	}
	if session.ScenePath != "res://Main.tscn" {
		t.Fatalf("expected registry scene path res://Main.tscn, got %q", session.ScenePath)
	}
}

func TestRunProject_AttachKeepsExistingLaunchTokenFromAck(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()

	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, now)
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_existing", "editor-1", "res://Main.tscn", "launch_existing", now)
	runtimebridge.DefaultGameSessionRegistry().RegisterRuntimeTransport("game_existing", "runtime-1", "editor-1", "res://Main.tscn", now, "launch_existing")
	runtimebridge.DefaultRuntimeSnapshotStore().Upsert("game_existing", runtimebridge.RuntimeSnapshot{
		SessionID:     "game_existing",
		SnapshotID:    "snap_existing",
		Frame:         12,
		UpdatedAt:     now.Format(time.RFC3339Nano),
		RootScenePath: "res://Main.tscn",
		RootNodeName:  "Main",
		NodeCount:     1,
		Running:       true,
	}, now)

	broker := runtimebridge.DefaultCommandBroker()
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		if sessionID != "editor-1" {
			return false
		}
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		go func() {
			broker.Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result: map[string]any{
					"running":      true,
					"session_id":   "game_existing",
					"scene_path":   "res://Main.tscn",
					"launch_token": "launch_existing",
				},
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	tool := &RunProjectTool{}
	raw := json.RawMessage(`{"_mcp":{"session_id":"ai-session","session_initialized":true}}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.project.run: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["session_id"] != "game_existing" {
		t.Fatalf("expected attached session_id game_existing, got %v", result["session_id"])
	}

	session, exists := runtimebridge.DefaultGameSessionRegistry().Session("game_existing")
	if !exists {
		t.Fatal("expected game_existing in registry")
	}
	if session.LaunchToken != "launch_existing" {
		t.Fatalf("expected launch token launch_existing, got %q", session.LaunchToken)
	}
}

func TestRunProject_AttachKeepsExistingLaunchTokenWhenAckOmitsIt(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()

	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, now)
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_existing", "editor-1", "res://Main.tscn", "launch_existing", now)
	runtimebridge.DefaultGameSessionRegistry().RegisterRuntimeTransport("game_existing", "runtime-1", "editor-1", "res://Main.tscn", now, "launch_existing")
	runtimebridge.DefaultRuntimeSnapshotStore().Upsert("game_existing", runtimebridge.RuntimeSnapshot{
		SessionID:     "game_existing",
		SnapshotID:    "snap_existing",
		Frame:         13,
		UpdatedAt:     now.Format(time.RFC3339Nano),
		RootScenePath: "res://Main.tscn",
		RootNodeName:  "Main",
		NodeCount:     1,
		Running:       true,
	}, now)

	broker := runtimebridge.DefaultCommandBroker()
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		if sessionID != "editor-1" {
			return false
		}
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		go func() {
			broker.Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result: map[string]any{
					"running":    true,
					"session_id": "game_existing",
					"scene_path": "res://Main.tscn",
				},
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	tool := &RunProjectTool{}
	raw := json.RawMessage(`{"_mcp":{"session_id":"ai-session","session_initialized":true}}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.project.run: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["session_id"] != "game_existing" {
		t.Fatalf("expected attached session_id game_existing, got %v", result["session_id"])
	}

	session, exists := runtimebridge.DefaultGameSessionRegistry().Session("game_existing")
	if !exists {
		t.Fatal("expected game_existing in registry")
	}
	if session.LaunchToken != "launch_existing" {
		t.Fatalf("expected launch token launch_existing, got %q", session.LaunchToken)
	}
}

func TestRunProject_CleansUpSessionWhenTransportUnavailable(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(200 * time.Millisecond)
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)
	runtimebridge.ResetDefaultRuntimeLogStoreForTests(2000)
	runtimebridge.DefaultEditorStore().Upsert("session-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, time.Now().UTC())
	runtimebridge.SetNotificationSender(nil)
	defer runtimebridge.SetNotificationSender(nil)

	tool := &RunProjectTool{}
	raw := json.RawMessage(`{"_mcp":{"session_id":"session-1","session_initialized":true}}`)

	_, err := tool.Execute(raw)
	if err == nil {
		t.Fatal("expected semantic error")
	}
	if _, ok := runtimebridge.DefaultGameSessionRegistry().ActiveForEditor("session-1"); ok {
		t.Fatal("expected no active game session after failed run")
	}
}

func TestRunProject_PreservesSessionWhenSnapshotNeverArrives(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)
	runtimebridge.ResetDefaultRuntimeLogStoreForTests(2000)
	runtimebridge.DefaultEditorStore().Upsert("session-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, time.Now().UTC())
	broker := runtimebridge.DefaultCommandBroker()
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		arguments, _ := params["arguments"].(map[string]any)
		gameSessionID, _ := arguments["session_id"].(string)
		go func() {
			broker.Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result: map[string]any{
					"running":    true,
					"session_id": gameSessionID,
					"scene_path": "res://Main.tscn",
				},
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	tool := &RunProjectTool{}
	raw := json.RawMessage(`{"_mcp":{"session_id":"session-1","session_initialized":true}}`)

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
	code := semanticErr.Data["code"]
	if code != "runtime_snapshot_missing" && code != "command_timeout" {
		t.Fatalf("expected code runtime_snapshot_missing/command_timeout, got %v", code)
	}
	active, exists := runtimebridge.DefaultGameSessionRegistry().ActiveForEditor("session-1")
	if !exists {
		t.Fatal("expected active game session to remain for late runtime register")
	}
	if !active.Running {
		t.Fatal("expected active session to remain running after snapshot timeout")
	}
}

func TestStopProject_ReturnsNotAvailableWhenTransportMissing(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(200 * time.Millisecond)
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.DefaultEditorStore().Upsert("session-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, time.Now().UTC())
	runtimebridge.SetNotificationSender(nil)
	defer runtimebridge.SetNotificationSender(nil)

	tool := &StopProjectTool{}
	payload := map[string]any{
		"_mcp": map[string]any{
			"session_id":          "session-1",
			"session_initialized": true,
		},
	}
	raw, _ := json.Marshal(payload)

	_, err := tool.Execute(raw)
	if err == nil {
		t.Fatal("expected semantic error")
	}
	semanticErr, ok := tooltypes.AsSemanticError(err)
	if !ok {
		t.Fatalf("expected semantic error, got %T", err)
	}
	if semanticErr.Data["reason"] != "command_transport_unavailable" {
		t.Fatalf("expected command_transport_unavailable, got %v", semanticErr.Data["reason"])
	}
}

func TestRunProject_UsesLatestFreshEditorSessionWhenCallerDiffers(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, now)

	broker := runtimebridge.DefaultCommandBroker()
	dispatchedTo := ""
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		dispatchedTo = sessionID
		if sessionID != "editor-1" {
			return false
		}
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		arguments, _ := params["arguments"].(map[string]any)
		gameSessionID, _ := arguments["session_id"].(string)
		go func() {
			snapshotAt := time.Now().UTC()
			runtimebridge.DefaultGameSessionRegistry().RegisterRuntimeTransport(gameSessionID, "runtime-1", "editor-1", "res://Main.tscn", snapshotAt, "test-launch")
			runtimebridge.DefaultRuntimeSnapshotStore().Upsert(gameSessionID, runtimebridge.RuntimeSnapshot{
				SessionID:     gameSessionID,
				SnapshotID:    "snap_1",
				Frame:         1,
				UpdatedAt:     snapshotAt.Format(time.RFC3339Nano),
				RootScenePath: "res://Main.tscn",
				RootNodeName:  "Main",
				NodeCount:     1,
				Running:       true,
			}, snapshotAt)
			broker.Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result: map[string]any{
					"running":    true,
					"session_id": gameSessionID,
					"scene_path": "res://Main.tscn",
				},
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	tool := &RunProjectTool{}
	raw := json.RawMessage(`{"_mcp":{"session_id":"caller-1","session_initialized":true}}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.project.run: %v", err)
	}
	if dispatchedTo != "editor-1" {
		t.Fatalf("expected dispatch to editor-1, got %q", dispatchedTo)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	sessionID, ok := result["session_id"].(string)
	if !ok || sessionID == "" {
		t.Fatalf("expected non-empty session_id, got %v", result["session_id"])
	}
	session, exists := runtimebridge.DefaultGameSessionRegistry().Session(sessionID)
	if !exists {
		t.Fatalf("expected session %q in registry", sessionID)
	}
	if session.EditorSessionID != "editor-1" {
		t.Fatalf("expected registry editor session editor-1, got %q", session.EditorSessionID)
	}
}

func TestRunProject_UsesExplicitEditorSessionOverride(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()

	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-a", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://SceneA.tscn"},
	}, now.Add(2*time.Second))
	runtimebridge.DefaultEditorStore().Upsert("editor-b", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://SceneB.tscn"},
	}, now)

	broker := runtimebridge.DefaultCommandBroker()
	dispatchedTo := ""
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		dispatchedTo = sessionID
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		arguments, _ := params["arguments"].(map[string]any)
		gameSessionID, _ := arguments["session_id"].(string)
		go func() {
			snapshotAt := time.Now().UTC()
			runtimebridge.DefaultGameSessionRegistry().RegisterRuntimeTransport(gameSessionID, "runtime-1", sessionID, "res://SceneB.tscn", snapshotAt, "test-launch")
			runtimebridge.DefaultRuntimeSnapshotStore().Upsert(gameSessionID, runtimebridge.RuntimeSnapshot{
				SessionID:     gameSessionID,
				SnapshotID:    "snap_1",
				Frame:         1,
				UpdatedAt:     snapshotAt.Format(time.RFC3339Nano),
				RootScenePath: "res://SceneB.tscn",
				RootNodeName:  "Main",
				NodeCount:     1,
				Running:       true,
			}, snapshotAt)
			broker.Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result: map[string]any{
					"running":    true,
					"session_id": gameSessionID,
					"scene_path": "res://SceneB.tscn",
				},
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	tool := &RunProjectTool{}
	raw := json.RawMessage(`{"editor_session_id":"editor-b","_mcp":{"session_id":"ai-session","session_initialized":true}}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.project.run: %v", err)
	}
	if dispatchedTo != "editor-b" {
		t.Fatalf("expected dispatch to editor-b, got %q", dispatchedTo)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["editor_session_id"] != "editor-b" {
		t.Fatalf("expected editor_session_id=editor-b, got %v", result["editor_session_id"])
	}
}

func TestStopProject_DispatchesToTargetGameOwnerEditorSession(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)
	runtimebridge.ResetDefaultRuntimeLogStoreForTests(100)
	now := time.Now().UTC()
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_test", "editor-1", "res://Main.tscn", "launch-token", now)
	runtimebridge.DefaultGameSessionRegistry().RegisterRuntimeTransport("game_test", "runtime-1", "editor-1", "res://Main.tscn", now, "launch-token")
	runtimebridge.DefaultRuntimeSnapshotStore().Upsert("game_test", runtimebridge.RuntimeSnapshot{
		SessionID:     "game_test",
		SnapshotID:    "snap_1",
		Frame:         1,
		UpdatedAt:     now.Format(time.RFC3339Nano),
		RootScenePath: "res://Main.tscn",
		RootNodeName:  "Main",
		NodeCount:     1,
		Running:       true,
	}, now)
	runtimebridge.DefaultRuntimeLogStore().Append("game_test", []runtimebridge.RuntimeLogAppendEntry{
		{Level: "error", Message: "boom"},
	}, now)

	dispatchedTo := ""
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		dispatchedTo = sessionID
		if sessionID != "editor-1" {
			return false
		}
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		go func() {
			runtimebridge.DefaultCommandBroker().Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result: map[string]any{
					"running":    false,
					"session_id": "game_test",
				},
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	tool := &StopProjectTool{}
	raw := json.RawMessage(`{"session_id":"game_test","_mcp":{"session_id":"caller-2","session_initialized":true}}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.project.stop: %v", err)
	}
	if dispatchedTo != "editor-1" {
		t.Fatalf("expected dispatch to editor-1, got %q", dispatchedTo)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["running"] != false {
		t.Fatalf("expected running=false, got %v", result["running"])
	}
	session, exists := runtimebridge.DefaultGameSessionRegistry().Session("game_test")
	if !exists {
		t.Fatalf("expected session game_test to remain in registry")
	}
	if session.Running {
		t.Fatal("expected game_test to be stopped in registry")
	}
	if _, ok, _ := runtimebridge.DefaultRuntimeSnapshotStore().FreshForSession("game_test", time.Now().UTC()); ok {
		t.Fatal("expected runtime snapshot to be removed after stop")
	}
	if entries := runtimebridge.DefaultRuntimeLogStore().Get("game_test", "all", 10, 0); len(entries) != 0 {
		t.Fatalf("expected runtime logs to be removed after stop, got %d entries", len(entries))
	}
}

func TestIsProjectRunning_UsesGameSessionRegistry(t *testing.T) {
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_test", "session-1", "res://Main.tscn", "token", time.Now().UTC())
	runtimebridge.DefaultEditorStore().Upsert("session-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, time.Now().UTC())

	tool := &IsProjectRunningTool{}
	raw := json.RawMessage(`{"_mcp":{"session_id":"session-1","session_initialized":true}}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.project.is_running: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["running"] != true {
		t.Fatalf("expected running=true, got %v", result["running"])
	}
	if result["session_id"] != "game_test" {
		t.Fatalf("expected session_id=game_test, got %v", result["session_id"])
	}
}

func TestIsProjectRunning_ResolvesEditorOwnerAcrossSessions(t *testing.T) {
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-a", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, now)
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_test", "editor-a", "res://Main.tscn", "token", now)

	tool := &IsProjectRunningTool{}
	raw := json.RawMessage(`{"_mcp":{"session_id":"ai-session-b","session_initialized":true}}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.project.is_running: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["running"] != true {
		t.Fatalf("expected running=true, got %v", result["running"])
	}
	if result["session_id"] != "game_test" {
		t.Fatalf("expected session_id=game_test, got %v", result["session_id"])
	}
	if result["editor_session_id"] != "editor-a" {
		t.Fatalf("expected editor_session_id=editor-a, got %v", result["editor_session_id"])
	}
}

func TestIsProjectRunning_UsesExplicitEditorSessionOverride(t *testing.T) {
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-a", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://SceneA.tscn"},
	}, now.Add(2*time.Second))
	runtimebridge.DefaultEditorStore().Upsert("editor-b", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://SceneB.tscn"},
	}, now)
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_b", "editor-b", "res://SceneB.tscn", "token", now)

	tool := &IsProjectRunningTool{}
	raw := json.RawMessage(`{"editor_session_id":"editor-b","_mcp":{"session_id":"ai-session","session_initialized":true}}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.project.is_running: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["running"] != true {
		t.Fatalf("expected running=true, got %v", result["running"])
	}
	if result["session_id"] != "game_b" {
		t.Fatalf("expected session_id=game_b, got %v", result["session_id"])
	}
}

func TestIsProjectRunning_ReturnsErrorWithoutHealthyEditorSession(t *testing.T) {
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)

	tool := &IsProjectRunningTool{}
	raw := json.RawMessage(`{"_mcp":{"session_id":"ai-session","session_initialized":true}}`)
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
