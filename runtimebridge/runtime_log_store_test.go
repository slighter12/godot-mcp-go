package runtimebridge

import (
	"sync"
	"testing"
	"time"
)

func TestRuntimeLogStoreGet_FiltersByLevelAndSequence(t *testing.T) {
	store := NewRuntimeLogStore(10)
	now := time.Now().UTC()

	appended := store.Append("game_1", []RuntimeLogAppendEntry{
		{Level: "info", Message: "boot"},
		{Level: "error", Message: "boom-1"},
		{Level: "warning", Message: "warn"},
		{Level: "error", Message: "boom-2"},
	}, now)
	if len(appended) != 4 {
		t.Fatalf("expected 4 appended entries, got %d", len(appended))
	}

	entries := store.Get("game_1", "error", 10, appended[1].Sequence)
	if len(entries) != 1 {
		t.Fatalf("expected 1 filtered entry, got %d", len(entries))
	}
	if entries[0].Message != "boom-2" {
		t.Fatalf("expected boom-2, got %q", entries[0].Message)
	}
	if entries[0].Sequence <= appended[1].Sequence {
		t.Fatalf("expected sequence greater than %d, got %d", appended[1].Sequence, entries[0].Sequence)
	}
}

func TestRuntimeLogStoreGet_ReturnsOldestMatchingEntriesWithinLimit(t *testing.T) {
	store := NewRuntimeLogStore(10)
	now := time.Now().UTC()

	store.Append("game_1", []RuntimeLogAppendEntry{
		{Level: "error", Message: "boom-1"},
		{Level: "error", Message: "boom-2"},
		{Level: "error", Message: "boom-3"},
	}, now)

	entries := store.Get("game_1", "error", 2, 0)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Message != "boom-1" || entries[1].Message != "boom-2" {
		t.Fatalf("expected oldest matching entries, got %#v", entries)
	}
}

func TestDefaultRuntimeLogStoreReset_ReplacesInstance(t *testing.T) {
	ResetDefaultRuntimeLogStoreForTests(32)
	first := DefaultRuntimeLogStore()
	if first == nil {
		t.Fatal("expected default runtime log store")
	}
	first.Append("game_1", []RuntimeLogAppendEntry{{Level: "error", Message: "boom"}}, time.Now().UTC())

	ResetDefaultRuntimeLogStoreForTests(32)
	second := DefaultRuntimeLogStore()
	if second == nil {
		t.Fatal("expected default runtime log store after reset")
	}
	if first == second {
		t.Fatal("expected reset to replace default runtime log store instance")
	}
	if entries := second.Get("game_1", "all", 50, 0); len(entries) != 0 {
		t.Fatalf("expected reset store to have empty entries, got %d", len(entries))
	}
}

func TestDefaultRuntimeLogStoreReset_ConcurrentLoadAndReset(t *testing.T) {
	ResetDefaultRuntimeLogStoreForTests(32)
	var wg sync.WaitGroup

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				ResetDefaultRuntimeLogStoreForTests(32)
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				store := DefaultRuntimeLogStore()
				if store == nil {
					t.Error("expected non-nil default runtime log store")
					return
				}
				_ = store.Health()
			}
		}()
	}

	wg.Wait()
}
