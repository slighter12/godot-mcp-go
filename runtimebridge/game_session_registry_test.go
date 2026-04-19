package runtimebridge

import (
	"sync"
	"testing"
	"time"
)

func TestGameSessionRegistryStopByEditorSession_StopsMappedSession(t *testing.T) {
	registry := NewGameSessionRegistry()
	now := time.Now().UTC()

	registry.UpsertFromRun("game_1", "editor_1", "res://Main.tscn", "launch_token", now)
	registry.RegisterRuntimeTransport("game_1", "runtime_1", "editor_1", "res://Main.tscn", now, "launch_token")

	sessionID, stopped := registry.StopByEditorSession("editor_1", now.Add(time.Second))
	if !stopped {
		t.Fatal("expected stop by editor session to succeed")
	}
	if sessionID != "game_1" {
		t.Fatalf("expected session_id game_1, got %q", sessionID)
	}

	session, ok := registry.Session("game_1")
	if !ok {
		t.Fatal("expected game session to exist")
	}
	if session.Running {
		t.Fatal("expected session running=false after stop")
	}
	if session.RuntimeSessionID != "" {
		t.Fatalf("expected runtime session cleared, got %q", session.RuntimeSessionID)
	}
	if session.StoppedAt == "" {
		t.Fatal("expected stopped_at to be populated")
	}
	if _, ok := registry.GameSessionIDByRuntimeSession("runtime_1"); ok {
		t.Fatal("expected runtime index entry to be removed")
	}
}

func TestGameSessionRegistryStopByEditorSession_RejectsStaleMappingWithoutSession(t *testing.T) {
	registry := NewGameSessionRegistry()
	registry.byEditorSessionID["editor_orphan"] = "game_missing"

	if sessionID, stopped := registry.StopByEditorSession("editor_orphan", time.Now().UTC()); stopped {
		t.Fatalf("expected stop to fail for stale mapping, got session=%q", sessionID)
	}
	if _, ok := registry.byEditorSessionID["editor_orphan"]; ok {
		t.Fatal("expected stale editor mapping to self-heal after failed stop")
	}
}

func TestGameSessionRegistryRemoveByEditorSession_RemovesSessionAndIndexes(t *testing.T) {
	registry := NewGameSessionRegistry()
	now := time.Now().UTC()

	registry.UpsertFromRun("game_1", "editor_1", "res://Main.tscn", "launch_token", now)
	registry.RegisterRuntimeTransport("game_1", "runtime_1", "editor_1", "res://Main.tscn", now, "launch_token")

	registry.RemoveByEditorSession("editor_1")

	if _, ok := registry.Session("game_1"); ok {
		t.Fatal("expected game session to be removed")
	}
	if _, ok := registry.ActiveForEditor("editor_1"); ok {
		t.Fatal("expected editor index entry to be removed")
	}
	if _, ok := registry.GameSessionIDByRuntimeSession("runtime_1"); ok {
		t.Fatal("expected runtime index entry to be removed")
	}
}

func TestGameSessionRegistryRemoveByEditorSession_HealsStaleMappingWithoutSession(t *testing.T) {
	registry := NewGameSessionRegistry()
	registry.byEditorSessionID["editor_orphan"] = "game_missing"

	registry.RemoveByEditorSession("editor_orphan")

	if _, ok := registry.byEditorSessionID["editor_orphan"]; ok {
		t.Fatal("expected stale editor mapping to self-heal after remove")
	}
}

func TestGameSessionRegistryUpsertFromRun_RebindEditorSessionRemovesOldMapping(t *testing.T) {
	registry := NewGameSessionRegistry()
	now := time.Now().UTC()

	registry.UpsertFromRun("game_1", "editor_old", "res://Main.tscn", "launch_token", now)
	registry.UpsertFromRun("game_1", "editor_new", "res://Main.tscn", "launch_token", now.Add(time.Second))

	if _, ok := registry.byEditorSessionID["editor_old"]; ok {
		t.Fatal("expected old editor mapping to be removed during rebind")
	}
	if mapped := registry.byEditorSessionID["editor_new"]; mapped != "game_1" {
		t.Fatalf("expected editor_new to map game_1, got %q", mapped)
	}
}

func TestGameSessionRegistryRegisterRuntimeTransport_RebindEditorSessionRemovesOldMapping(t *testing.T) {
	registry := NewGameSessionRegistry()
	now := time.Now().UTC()

	registry.UpsertFromRun("game_1", "editor_old", "res://Main.tscn", "launch_token", now)
	registry.RegisterRuntimeTransport("game_1", "runtime_1", "editor_new", "res://Main.tscn", now.Add(time.Second), "launch_token")

	if _, ok := registry.byEditorSessionID["editor_old"]; ok {
		t.Fatal("expected old editor mapping to be removed during runtime transport rebind")
	}
	if mapped := registry.byEditorSessionID["editor_new"]; mapped != "game_1" {
		t.Fatalf("expected editor_new to map game_1, got %q", mapped)
	}
}

func TestGameSessionRegistryRegisterRuntimeTransport_RebindRuntimeSessionEvictsPreviousOwner(t *testing.T) {
	registry := NewGameSessionRegistry()
	now := time.Now().UTC()

	registry.UpsertFromRun("game_1", "editor_1", "res://Main1.tscn", "launch_token_1", now)
	registry.RegisterRuntimeTransport("game_1", "runtime_shared", "editor_1", "res://Main1.tscn", now, "launch_token_1")
	registry.UpsertFromRun("game_2", "editor_2", "res://Main2.tscn", "launch_token_2", now.Add(time.Second))
	registry.RegisterRuntimeTransport("game_2", "runtime_shared", "editor_2", "res://Main2.tscn", now.Add(2*time.Second), "launch_token_2")

	session1, ok := registry.Session("game_1")
	if !ok {
		t.Fatal("expected game_1 to exist")
	}
	if session1.RuntimeSessionID != "" {
		t.Fatalf("expected game_1 runtime session to be cleared after ownership transfer, got %q", session1.RuntimeSessionID)
	}

	if mapped, ok := registry.GameSessionIDByRuntimeSession("runtime_shared"); !ok || mapped != "game_2" {
		t.Fatalf("expected runtime_shared to map game_2, got mapped=%q ok=%v", mapped, ok)
	}

	registry.StopSession("game_1", now.Add(3*time.Second))
	if mapped, ok := registry.GameSessionIDByRuntimeSession("runtime_shared"); !ok || mapped != "game_2" {
		t.Fatalf("expected runtime_shared mapping to remain on game_2 after stopping old owner, got mapped=%q ok=%v", mapped, ok)
	}
}

func TestDefaultGameSessionRegistryReset_ReplacesInstance(t *testing.T) {
	ResetDefaultGameSessionRegistryForTests()
	first := DefaultGameSessionRegistry()
	if first == nil {
		t.Fatal("expected default game session registry")
	}
	first.UpsertFromRun("game_1", "editor_1", "res://Main.tscn", "token", time.Now().UTC())

	ResetDefaultGameSessionRegistryForTests()
	second := DefaultGameSessionRegistry()
	if second == nil {
		t.Fatal("expected default game session registry after reset")
	}
	if first == second {
		t.Fatal("expected reset to replace default game session registry instance")
	}
	if _, ok := second.Session("game_1"); ok {
		t.Fatal("expected reset registry to have empty session state")
	}
}

func TestDefaultGameSessionRegistryReset_ConcurrentLoadAndReset(t *testing.T) {
	ResetDefaultGameSessionRegistryForTests()
	var wg sync.WaitGroup

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				ResetDefaultGameSessionRegistryForTests()
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				registry := DefaultGameSessionRegistry()
				if registry == nil {
					t.Error("expected non-nil default game session registry")
					return
				}
				_ = registry.Health()
			}
		}()
	}

	wg.Wait()
}
