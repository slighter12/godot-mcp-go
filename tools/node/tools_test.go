package node

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

func TestNodeWriteTools_ReturnNotAvailable(t *testing.T) {
	tools := []interface {
		Execute(args json.RawMessage) ([]byte, error)
	}{
		&CreateNodeTool{},
		&DeleteNodeTool{},
		&ModifyNodeTool{},
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

func TestNodeCreateTool_DispatchesToRuntimeCommandSessionID(t *testing.T) {
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

	tool := &CreateNodeTool{}
	raw := json.RawMessage(`{
		"type": "Node2D",
		"parent": "/root",
		"name": "TestNode",
		"_mcp": {
			"session_id": "ai-session",
			"session_initialized": true,
			"runtime_command_session_id": "editor-1"
		}
	}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.node.create: %v", err)
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

func TestNodeCreateTool_FailsWhenRuntimeCommandSessionMissing(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(500 * time.Millisecond)
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	runtimebridge.SetNotificationSender(nil)

	tool := &CreateNodeTool{}
	raw := json.RawMessage(`{
		"type": "Node2D",
		"parent": "/root",
		"name": "TestNode",
		"_mcp": {
			"session_id": "ai-session",
			"session_initialized": true,
			"runtime_command_session_id": ""
		}
	}`)
	_, err := tool.Execute(raw)
	if err == nil {
		t.Fatal("expected error when runtime command session is missing")
	}
	semanticErr, ok := tooltypes.AsSemanticError(err)
	if !ok {
		t.Fatalf("expected semantic error, got %T", err)
	}
	if semanticErr.Kind != tooltypes.SemanticKindNotAvailable {
		t.Fatalf("expected not_available kind, got %s", semanticErr.Kind)
	}
}
