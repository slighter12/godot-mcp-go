package types

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/runtimebridge"
)

func TestDispatchRuntimeCommand_EmitsProgressWhenEnabled(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(500 * time.Millisecond)
	broker := runtimebridge.DefaultCommandBroker()
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		go func() {
			_ = broker.Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	var mu sync.Mutex
	events := make([]RuntimeCommandProgressEvent, 0)
	SetRuntimeCommandProgressNotifier(func(event RuntimeCommandProgressEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	})
	defer SetRuntimeCommandProgressNotifier(nil)

	rawArgs, err := json.Marshal(map[string]any{
		"_mcp": map[string]any{
			"session_id":                  "session-1",
			"session_initialized":         true,
			"emit_progress_notifications": true,
			"progress_token":              "req-123",
		},
	})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	_, err = DispatchRuntimeCommand(RuntimeCommandDispatchOptions{
		RawArgs:                  rawArgs,
		CommandName:              "godot.project.run",
		Timeout:                  500 * time.Millisecond,
		SessionRequiredMessage:   "session required",
		BridgeUnavailableMessage: "bridge unavailable",
	})
	if err != nil {
		t.Fatalf("dispatch runtime command: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 2 {
		t.Fatalf("expected 2 progress events, got %d", len(events))
	}
	if events[0].Progress != 0.4 {
		t.Fatalf("expected first event progress 0.4, got %v", events[0].Progress)
	}
	if events[1].Progress != 1.0 {
		t.Fatalf("expected second event progress 1.0, got %v", events[1].Progress)
	}
}

func TestDispatchRuntimeCommand_DoesNotEmitProgressWhenDisabled(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(500 * time.Millisecond)
	broker := runtimebridge.DefaultCommandBroker()
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		go func() {
			_ = broker.Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	var mu sync.Mutex
	events := make([]RuntimeCommandProgressEvent, 0)
	SetRuntimeCommandProgressNotifier(func(event RuntimeCommandProgressEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	})
	defer SetRuntimeCommandProgressNotifier(nil)

	rawArgs, err := json.Marshal(map[string]any{
		"_mcp": map[string]any{
			"session_id":                  "session-1",
			"session_initialized":         true,
			"emit_progress_notifications": false,
		},
	})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	_, err = DispatchRuntimeCommand(RuntimeCommandDispatchOptions{
		RawArgs:                  rawArgs,
		CommandName:              "godot.project.run",
		Timeout:                  500 * time.Millisecond,
		SessionRequiredMessage:   "session required",
		BridgeUnavailableMessage: "bridge unavailable",
	})
	if err != nil {
		t.Fatalf("dispatch runtime command: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 0 {
		t.Fatalf("expected 0 progress events, got %d", len(events))
	}
}
