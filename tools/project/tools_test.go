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
	runtimebridge.DefaultStore().Upsert("session-1", runtimebridge.Snapshot{
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
	runtimebridge.DefaultStore().Upsert("session-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{
			ActiveScene: "res://SceneA.tscn",
		},
	}, now)
	runtimebridge.DefaultStore().Upsert("session-2", runtimebridge.Snapshot{
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
	broker := runtimebridge.DefaultCommandBroker()
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		go func() {
			broker.Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result:    map[string]any{"running": true},
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
}

func TestStopProject_ReturnsNotAvailableWhenTransportMissing(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(200 * time.Millisecond)
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
