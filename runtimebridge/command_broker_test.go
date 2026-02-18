package runtimebridge

import (
	"testing"
	"time"
)

func TestCommandBrokerDispatchAndAck(t *testing.T) {
	ResetDefaultCommandBrokerForTests(2 * time.Second)
	broker := DefaultCommandBroker()
	SetNotificationSender(func(sessionID string, message map[string]any) bool {
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["commandId"].(string)
		go func() {
			broker.Ack(sessionID, CommandAck{
				CommandID: commandID,
				Success:   true,
				Result:    map[string]any{"ok": true},
			})
		}()
		return true
	})
	defer SetNotificationSender(nil)

	ack, ok, reason := broker.DispatchAndWait("session-1", "run-project", map[string]any{}, 2*time.Second)
	if !ok {
		t.Fatalf("expected command ack, reason=%s", reason)
	}
	if !ack.Success {
		t.Fatalf("expected success ack, got %+v", ack)
	}
}

func TestCommandBrokerDispatchWithoutSender(t *testing.T) {
	ResetDefaultCommandBrokerForTests(2 * time.Second)
	SetNotificationSender(nil)

	_, ok, reason := DefaultCommandBroker().DispatchAndWait("session-1", "run-project", map[string]any{}, 2*time.Second)
	if ok {
		t.Fatal("expected dispatch failure without notification sender")
	}
	if reason != "command_transport_unavailable" {
		t.Fatalf("expected command_transport_unavailable, got %s", reason)
	}
}
