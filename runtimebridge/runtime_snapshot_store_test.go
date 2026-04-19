package runtimebridge

import (
	"sync"
	"testing"
	"time"
)

func TestRuntimeSnapshotStoreAwait_WakesOnUpsert(t *testing.T) {
	store := NewRuntimeSnapshotStore(2*time.Second, 0)
	sessionID := "game_await_wakeup"

	type result struct {
		stored StoredRuntimeSnapshot
		reason string
		ok     bool
	}
	done := make(chan result, 1)
	started := time.Now()

	go func() {
		stored, reason, ok := store.Await(sessionID, 10, 500*time.Millisecond, FreshnessStateFresh)
		done <- result{stored: stored, reason: reason, ok: ok}
	}()

	time.Sleep(30 * time.Millisecond)
	store.Upsert(sessionID, RuntimeSnapshot{
		SnapshotID: "snap-1",
		Frame:      12,
	}, time.Now().UTC())

	select {
	case out := <-done:
		if !out.ok {
			t.Fatalf("expected await to succeed, got reason=%q", out.reason)
		}
		if out.stored.Snapshot.Frame != 12 {
			t.Fatalf("expected frame 12, got %d", out.stored.Snapshot.Frame)
		}
		if elapsed := time.Since(started); elapsed >= 450*time.Millisecond {
			t.Fatalf("expected await to wake before timeout, took %s", elapsed)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for await wakeup")
	}
}

func TestRuntimeSnapshotStoreAwait_ReturnsMissingOnTimeout(t *testing.T) {
	store := NewRuntimeSnapshotStore(2*time.Second, 0)

	_, reason, ok := store.Await("missing_session", 0, 60*time.Millisecond, FreshnessStateFresh)
	if ok {
		t.Fatal("expected await to fail for missing snapshot")
	}
	if reason != "runtime_snapshot_missing" {
		t.Fatalf("expected runtime_snapshot_missing, got %q", reason)
	}
}

func TestRuntimeSnapshotStoreAwait_ReturnsStaleOnTimeout(t *testing.T) {
	store := NewRuntimeSnapshotStore(10*time.Millisecond, 0)
	sessionID := "game_stale_timeout"
	old := time.Now().UTC().Add(-200 * time.Millisecond)

	store.Upsert(sessionID, RuntimeSnapshot{
		SnapshotID: "snap-stale",
		Frame:      1,
	}, old)

	_, reason, ok := store.Await(sessionID, 2, 50*time.Millisecond, FreshnessStateFresh)
	if ok {
		t.Fatal("expected await to fail for stale snapshot")
	}
	if reason != "runtime_snapshot_stale" {
		t.Fatalf("expected runtime_snapshot_stale, got %q", reason)
	}
}

func TestRuntimeSnapshotStoreAwait_ReturnsCommandTimeoutWhenNotStale(t *testing.T) {
	store := NewRuntimeSnapshotStore(2*time.Second, 0)
	sessionID := "game_command_timeout"

	store.Upsert(sessionID, RuntimeSnapshot{
		SnapshotID: "snap-timeout",
		Frame:      1,
	}, time.Now().UTC())

	_, reason, ok := store.Await(sessionID, 10, 60*time.Millisecond, FreshnessStateFresh)
	if ok {
		t.Fatal("expected await to timeout when frame condition is not met")
	}
	if reason != "command_timeout" {
		t.Fatalf("expected command_timeout, got %q", reason)
	}
}

func TestDefaultRuntimeSnapshotStoreReset_ReplacesInstance(t *testing.T) {
	ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)

	first := DefaultRuntimeSnapshotStore()
	if first == nil {
		t.Fatal("expected first default runtime snapshot store to be initialized")
	}

	ResetDefaultRuntimeSnapshotStoreForTests(20*time.Second, 0)
	second := DefaultRuntimeSnapshotStore()
	if second == nil {
		t.Fatal("expected second default runtime snapshot store to be initialized")
	}
	if first == second {
		t.Fatal("expected reset to replace default runtime snapshot store instance")
	}
	if got := second.staleAfter; got != 20*time.Second {
		t.Fatalf("expected stale_after to be 20s after reset, got %s", got)
	}
}

func TestDefaultRuntimeSnapshotStoreReset_ConcurrentLoadAndReset(t *testing.T) {
	ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)

	const workers = 8
	const iterations = 100

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if DefaultRuntimeSnapshotStore() == nil {
					t.Error("DefaultRuntimeSnapshotStore returned nil")
					return
				}
			}
		}()
	}

	for i := 0; i < iterations; i++ {
		if i%2 == 0 {
			ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)
			continue
		}
		ResetDefaultRuntimeSnapshotStoreForTests(20*time.Second, 0)
	}

	wg.Wait()
	current := DefaultRuntimeSnapshotStore()
	if current == nil {
		t.Fatal("expected runtime snapshot store after concurrent reset/load")
	}
	if got := current.staleAfter; got != 10*time.Second && got != 20*time.Second {
		t.Fatalf("expected stale_after to be one of test reset values, got %s", got)
	}
}
