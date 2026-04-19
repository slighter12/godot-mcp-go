package scene

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

func TestReadSceneTool_ReadsContentAndNodeSummary(t *testing.T) {
	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectRoot, "project.godot"), []byte("[application]"), 0o644); err != nil {
		t.Fatalf("write project.godot: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, "scenes"), 0o755); err != nil {
		t.Fatalf("mkdir scenes: %v", err)
	}
	sceneContent := "[gd_scene format=3]\n[node name=\"Root\" type=\"Node2D\"]\n[node name=\"Child\" type=\"Sprite2D\" parent=\".\"]\n"
	if err := os.WriteFile(filepath.Join(projectRoot, "scenes", "Main.tscn"), []byte(sceneContent), 0o644); err != nil {
		t.Fatalf("write scene: %v", err)
	}
	t.Setenv("GODOT_PROJECT_ROOT", projectRoot)

	tool := &ReadSceneTool{}
	rawArgs, _ := json.Marshal(map[string]any{"path": "res://scenes/Main.tscn"})
	resultRaw, err := tool.Execute(rawArgs)
	if err != nil {
		t.Fatalf("execute godot.scene.read: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["path"] != "res://scenes/Main.tscn" {
		t.Fatalf("unexpected path: %v", result["path"])
	}
	if result["content"] != sceneContent {
		t.Fatalf("unexpected content: %v", result["content"])
	}
	nodes, ok := result["nodes"].([]any)
	if !ok || len(nodes) != 2 {
		t.Fatalf("expected two parsed nodes, got %T %v", result["nodes"], result["nodes"])
	}
}

func TestSceneWriteTools_ReturnNotAvailable(t *testing.T) {
	tools := []interface {
		Execute(args json.RawMessage) ([]byte, error)
	}{
		&CreateSceneTool{},
		&SaveSceneTool{},
		&ApplySceneTool{},
	}
	for _, tool := range tools {
		_, err := tool.Execute(json.RawMessage(`{}`))
		if err == nil {
			t.Fatal("expected semantic not available error")
		}
		semanticErr, ok := tooltypes.AsSemanticError(err)
		if !ok {
			t.Fatalf("expected semantic error, got %T", err)
		}
		if semanticErr.Kind != tooltypes.SemanticKindNotAvailable {
			t.Fatalf("expected not_available kind, got %s", semanticErr.Kind)
		}
	}
}

func TestApplySceneTool_ResolvesLatestFreshEditorSessionWhenCallerDiffers(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, now)

	dispatchedTo := ""
	broker := runtimebridge.DefaultCommandBroker()
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		dispatchedTo = sessionID
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		go func() {
			_ = broker.Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result: map[string]any{
					"applied": true,
				},
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	tool := &ApplySceneTool{}
	raw := json.RawMessage(`{
		"path":"res://scenes/Main.tscn",
		"_mcp":{"session_id":"ai-session","session_initialized":true}
	}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.editor.scene.apply: %v", err)
	}
	if dispatchedTo != "editor-1" {
		t.Fatalf("expected dispatch to editor-1, got %q", dispatchedTo)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["success"] != true {
		t.Fatalf("expected success=true, got %v", result["success"])
	}
}

func TestApplySceneTool_UsesExplicitEditorSessionOverride(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-a", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://A.tscn"},
	}, now.Add(2*time.Second))
	runtimebridge.DefaultEditorStore().Upsert("editor-b", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://B.tscn"},
	}, now)

	dispatchedTo := ""
	broker := runtimebridge.DefaultCommandBroker()
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		dispatchedTo = sessionID
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		go func() {
			_ = broker.Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result: map[string]any{
					"applied": true,
				},
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	tool := &ApplySceneTool{}
	raw := json.RawMessage(`{
		"path":"res://scenes/Main.tscn",
		"editor_session_id":"editor-b",
		"_mcp":{"session_id":"ai-session","session_initialized":true}
	}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.editor.scene.apply: %v", err)
	}
	if dispatchedTo != "editor-b" {
		t.Fatalf("expected dispatch to editor-b, got %q", dispatchedTo)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["success"] != true {
		t.Fatalf("expected success=true, got %v", result["success"])
	}
}

func TestSceneCreateTool_DispatchesToRuntimeCommandSessionID(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, now)

	dispatchedTo := ""
	broker := runtimebridge.DefaultCommandBroker()
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		dispatchedTo = sessionID
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		go func() {
			_ = broker.Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result:    map[string]any{"created": true},
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	tool := &CreateSceneTool{}
	raw := json.RawMessage(`{
		"path": "res://scenes/New.tscn",
		"_mcp": {
			"session_id": "ai-session",
			"session_initialized": true,
			"runtime_command_session_id": "editor-1"
		}
	}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.scene.create: %v", err)
	}
	if dispatchedTo != "editor-1" {
		t.Fatalf("expected dispatch to editor-1, got %q", dispatchedTo)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["success"] != true {
		t.Fatalf("expected success=true, got %v", result["success"])
	}
}

func TestSceneSaveTool_DispatchesToRuntimeCommandSessionID(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, now)

	dispatchedTo := ""
	broker := runtimebridge.DefaultCommandBroker()
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		dispatchedTo = sessionID
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		go func() {
			_ = broker.Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result:    map[string]any{"saved": true},
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	tool := &SaveSceneTool{}
	raw := json.RawMessage(`{
		"_mcp": {
			"session_id": "ai-session",
			"session_initialized": true,
			"runtime_command_session_id": "editor-1"
		}
	}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.scene.save: %v", err)
	}
	if dispatchedTo != "editor-1" {
		t.Fatalf("expected dispatch to editor-1, got %q", dispatchedTo)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["success"] != true {
		t.Fatalf("expected success=true, got %v", result["success"])
	}
}
