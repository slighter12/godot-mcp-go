package project

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

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
		t.Fatalf("execute get-editor-state: %v", err)
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
		t.Fatalf("execute get-editor-state: %v", err)
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
		commandID, _ := params["commandId"].(string)
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
		t.Fatalf("execute run-project: %v", err)
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
