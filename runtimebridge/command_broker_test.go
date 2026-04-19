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
		commandID, _ := params["command_id"].(string)
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

	ack, ok, reason := broker.DispatchAndWait("session-1", "godot.project.run", map[string]any{}, 2*time.Second)
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

	_, ok, reason := DefaultCommandBroker().DispatchAndWait("session-1", "godot.project.run", map[string]any{}, 2*time.Second)
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
		commandID, _ := params["command_id"].(string)
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
			_, ok, reason := broker.DispatchAndWait("bench-session", "godot.project.run", map[string]any{}, 500*time.Millisecond)
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
		commandID, _ := params["command_id"].(string)
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
			_, ok, reason := broker.DispatchAndWait(sessionID, "godot.project.run", map[string]any{}, 500*time.Millisecond)
			if !ok {
				b.Fatalf("expected ack in benchmark, reason=%s", reason)
			}
		}
	})
}

func TestCommandBrokerAck_RejectsWrongSession(t *testing.T) {
	ResetDefaultCommandBrokerForTests(2 * time.Second)
	broker := DefaultCommandBroker()

	var capturedCommandID string
	SetNotificationSender(func(sessionID string, message map[string]any) bool {
		params, _ := message["params"].(map[string]any)
		capturedCommandID, _ = params["command_id"].(string)
		// Do NOT auto-ack here — we'll ack manually to test rejection.
		return true
	})
	defer SetNotificationSender(nil)

	// Dispatch in background so we can attempt acks while it's pending.
	done := make(chan struct{})
	go func() {
		defer close(done)
		broker.DispatchAndWait("editor-1", "godot.node.create", map[string]any{}, 2*time.Second)
	}()

	// Wait for the command to be dispatched and capturedCommandID to be set.
	for i := 0; i < 50; i++ {
		if capturedCommandID != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if capturedCommandID == "" {
		t.Fatal("command was not dispatched")
	}

	// Ack from wrong session should be rejected.
	accepted := broker.Ack("wrong-session", CommandAck{
		CommandID: capturedCommandID,
		Success:   true,
	})
	if accepted {
		t.Fatal("expected ack from wrong session to be rejected")
	}

	// Ack from correct session should be accepted.
	accepted = broker.Ack("editor-1", CommandAck{
		CommandID: capturedCommandID,
		Success:   true,
	})
	if !accepted {
		t.Fatal("expected ack from correct session to be accepted")
	}

	<-done
}

func TestDefaultCommandBrokerReset_ReplacesInstance(t *testing.T) {
	ResetDefaultCommandBrokerForTests(2 * time.Second)
	first := DefaultCommandBroker()
	if first == nil {
		t.Fatal("expected default command broker")
	}

	ResetDefaultCommandBrokerForTests(3 * time.Second)
	second := DefaultCommandBroker()
	if second == nil {
		t.Fatal("expected default command broker after reset")
	}
	if first == second {
		t.Fatal("expected reset to replace default command broker instance")
	}
}

func TestDefaultCommandBrokerReset_ConcurrentLoadAndReset(t *testing.T) {
	ResetDefaultCommandBrokerForTests(500 * time.Millisecond)
	var wg sync.WaitGroup

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				ResetDefaultCommandBrokerForTests(500 * time.Millisecond)
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				broker := DefaultCommandBroker()
				if broker == nil {
					t.Error("expected non-nil default command broker")
					return
				}
				_ = broker.Metrics()
			}
		}()
	}

	wg.Wait()
}
