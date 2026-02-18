package utility

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

func TestAckEditorCommandTool_RequiresInitializedSession(t *testing.T) {
	tool := NewAckEditorCommandTool()
	raw, _ := json.Marshal(map[string]any{
		"command_id": "cmd-1",
		"success":    true,
	})
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
}

func TestAckEditorCommandTool_AcknowledgesPendingCommand(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	broker := runtimebridge.DefaultCommandBroker()

	capturedCommandID := ""
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		params, _ := message["params"].(map[string]any)
		if id, ok := params["commandId"].(string); ok {
			capturedCommandID = id
		}
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	type dispatchResult struct {
		ack    runtimebridge.CommandAck
		ok     bool
		reason string
	}
	resultCh := make(chan dispatchResult, 1)
	go func() {
		ack, ok, reason := broker.DispatchAndWait("session-1", "run-project", map[string]any{}, 2*time.Second)
		resultCh <- dispatchResult{ack: ack, ok: ok, reason: reason}
	}()

	deadline := time.Now().Add(300 * time.Millisecond)
	for capturedCommandID == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if capturedCommandID == "" {
		t.Fatal("expected command id to be captured from notification")
	}

	tool := NewAckEditorCommandTool()
	raw, _ := json.Marshal(map[string]any{
		"command_id": capturedCommandID,
		"success":    true,
		"result":     map[string]any{"running": true},
		"_mcp": map[string]any{
			"session_id":          "session-1",
			"session_initialized": true,
		},
	})
	if _, err := tool.Execute(raw); err != nil {
		t.Fatalf("execute ack-editor-command: %v", err)
	}

	select {
	case dispatch := <-resultCh:
		if !dispatch.ok {
			t.Fatalf("expected command dispatch success, reason=%s", dispatch.reason)
		}
		if !dispatch.ack.Success {
			t.Fatalf("expected ack success, got %+v", dispatch.ack)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for dispatch result")
	}
}
