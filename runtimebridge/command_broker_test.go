package runtimebridge

import (
	"fmt"
	"sync"
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

	ack, ok, reason := broker.DispatchAndWait("session-1", "godot-project-run", map[string]any{}, 2*time.Second)
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

	_, ok, reason := DefaultCommandBroker().DispatchAndWait("session-1", "godot-project-run", map[string]any{}, 2*time.Second)
	if ok {
		t.Fatal("expected dispatch failure without notification sender")
	}
	if reason != "command_transport_unavailable" {
		t.Fatalf("expected command_transport_unavailable, got %s", reason)
	}
}

func BenchmarkCommandBrokerDispatchAndAckParallel(b *testing.B) {
	ResetDefaultCommandBrokerForTests(500 * time.Millisecond)
	broker := DefaultCommandBroker()
	SetNotificationSender(func(sessionID string, message map[string]any) bool {
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["commandId"].(string)
		go func() {
			_ = broker.Ack(sessionID, CommandAck{
				CommandID: commandID,
				Success:   true,
				Result:    map[string]any{"ok": true},
			})
		}()
		return true
	})
	defer SetNotificationSender(nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, ok, reason := broker.DispatchAndWait("bench-session", "godot-project-run", map[string]any{}, 500*time.Millisecond)
			if !ok {
				b.Fatalf("expected ack in benchmark, reason=%s", reason)
			}
		}
	})
}

func BenchmarkCommandBrokerConcurrentSessions(b *testing.B) {
	ResetDefaultCommandBrokerForTests(500 * time.Millisecond)
	broker := DefaultCommandBroker()
	SetNotificationSender(func(sessionID string, message map[string]any) bool {
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["commandId"].(string)
		go func() {
			_ = broker.Ack(sessionID, CommandAck{
				CommandID: commandID,
				Success:   true,
			})
		}()
		return true
	})
	defer SetNotificationSender(nil)

	const sessions = 16
	sessionIDs := make([]string, 0, sessions)
	for i := 0; i < sessions; i++ {
		sessionIDs = append(sessionIDs, fmt.Sprintf("bench-session-%02d", i))
	}

	var indexMu sync.Mutex
	index := 0
	nextSessionID := func() string {
		indexMu.Lock()
		defer indexMu.Unlock()
		id := sessionIDs[index%len(sessionIDs)]
		index++
		return id
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sessionID := nextSessionID()
			_, ok, reason := broker.DispatchAndWait(sessionID, "godot-project-run", map[string]any{}, 500*time.Millisecond)
			if !ok {
				b.Fatalf("expected ack in benchmark, reason=%s", reason)
			}
		}
	})
}
