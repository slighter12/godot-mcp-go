package runtimebridge

import (
	"sync"
	"testing"
	"time"
)

func TestStoreLatestFreshAndStale(t *testing.T) {
	store := NewStore(10*time.Second, 0)
	now := time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC)
	store.Upsert("session-a", Snapshot{RootSummary: RootSummary{ActiveScene: "res://Main.tscn"}}, now)

	stored, ok, reason := store.LatestFresh(now.Add(9 * time.Second))
	if !ok {
		t.Fatalf("expected fresh snapshot, reason=%s", reason)
	}
	if stored.SessionID != "session-a" {
		t.Fatalf("expected session-a, got %s", stored.SessionID)
	}

	_, ok, reason = store.LatestFresh(now.Add(11 * time.Second))
	if ok {
		t.Fatal("expected stale snapshot")
	}
	if reason != "runtime_snapshot_stale" {
		t.Fatalf("expected runtime_snapshot_stale, got %s", reason)
	}
}

func TestStoreRemoveSessionUpdatesLatest(t *testing.T) {
	store := NewStore(10*time.Second, 0)
	now := time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC)
	store.Upsert("session-a", Snapshot{}, now)
	store.Upsert("session-b", Snapshot{}, now.Add(1*time.Second))

	store.RemoveSession("session-b")
	stored, ok, reason := store.LatestFresh(now.Add(2 * time.Second))
	if !ok {
		t.Fatalf("expected fallback latest snapshot, reason=%s", reason)
	}
	if stored.SessionID != "session-a" {
		t.Fatalf("expected session-a, got %s", stored.SessionID)
	}
}

func TestStoreTouchRefreshesSnapshot(t *testing.T) {
	store := NewStore(10*time.Second, 0)
	now := time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC)
	store.Upsert("session-a", Snapshot{
		RootSummary: RootSummary{ActiveScene: "res://Main.tscn"},
	}, now)

	if touched := store.Touch("session-a", now.Add(9*time.Second)); !touched {
		t.Fatal("expected touch success for existing session")
	}

	stored, ok, reason := store.LatestFresh(now.Add(18 * time.Second))
	if !ok {
		t.Fatalf("expected fresh snapshot after touch, reason=%s", reason)
	}
	if stored.Snapshot.RootSummary.ActiveScene != "res://Main.tscn" {
		t.Fatalf("expected snapshot payload unchanged, got %s", stored.Snapshot.RootSummary.ActiveScene)
	}

	if touched := store.Touch("missing-session", now); touched {
		t.Fatal("expected touch failure for missing session")
	}
}

func TestStoreFreshForSession_IsSessionScoped(t *testing.T) {
	store := NewStore(10*time.Second, 0)
	now := time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC)

	store.Upsert("session-a", Snapshot{RootSummary: RootSummary{ActiveScene: "res://A.tscn"}}, now)
	store.Upsert("session-b", Snapshot{RootSummary: RootSummary{ActiveScene: "res://B.tscn"}}, now.Add(1*time.Second))

	storedA, ok, reason := store.FreshForSession("session-a", now.Add(2*time.Second))
	if !ok {
		t.Fatalf("expected session-a snapshot, reason=%s", reason)
	}
	if storedA.SessionID != "session-a" {
		t.Fatalf("expected session-a, got %s", storedA.SessionID)
	}
	if storedA.Snapshot.RootSummary.ActiveScene != "res://A.tscn" {
		t.Fatalf("expected res://A.tscn, got %s", storedA.Snapshot.RootSummary.ActiveScene)
	}

	storedB, ok, reason := store.FreshForSession("session-b", now.Add(2*time.Second))
	if !ok {
		t.Fatalf("expected session-b snapshot, reason=%s", reason)
	}
	if storedB.SessionID != "session-b" {
		t.Fatalf("expected session-b, got %s", storedB.SessionID)
	}
	if storedB.Snapshot.RootSummary.ActiveScene != "res://B.tscn" {
		t.Fatalf("expected res://B.tscn, got %s", storedB.Snapshot.RootSummary.ActiveScene)
	}
}

func TestHealthSnapshot_UsesCanonicalFreshnessKeys(t *testing.T) {
	ResetDefaultStoreForTests(10 * time.Second)
	ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)
	ResetDefaultGameSessionRegistryForTests()
	ResetDefaultRuntimeLogStoreForTests(50)
	ResetDefaultCommandBrokerForTests(500 * time.Millisecond)

	now := time.Date(2026, 4, 6, 15, 0, 0, 0, time.UTC)
	DefaultEditorStore().Upsert("editor-1", Snapshot{
		RootSummary: RootSummary{ActiveScene: "res://Editor.tscn"},
	}, now)
	DefaultRuntimeSnapshotStore().Upsert("game-1", RuntimeSnapshot{
		SessionID:    "game-1",
		SnapshotID:   "snap_1",
		Frame:        1,
		UpdatedAt:    now.Format(time.RFC3339Nano),
		Running:      true,
		NodeCount:    1,
		RootNodeName: "Main",
	}, now)

	health := HealthSnapshot(now.Add(time.Second))
	if _, ok := health["freshness"]; ok {
		t.Fatalf("expected legacy key freshness to be removed, got %v", health["freshness"])
	}
	if _, ok := health["editor_freshness"]; !ok {
		t.Fatal("expected editor_freshness key")
	}
	if _, ok := health["runtime_freshness"]; !ok {
		t.Fatal("expected runtime_freshness key")
	}
}

func TestDefaultStore_DeprecatedAliasCompatibility(t *testing.T) {
	ResetDefaultEditorStoreForTests(10 * time.Second)

	legacy := DefaultStore()
	editor := DefaultEditorStore()
	if legacy == nil || editor == nil {
		t.Fatal("expected default stores to be initialized")
	}
	if legacy != editor {
		t.Fatal("expected DefaultStore and DefaultEditorStore to point to the same instance")
	}

	now := time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC)
	legacy.Upsert("editor-session", Snapshot{
		RootSummary: RootSummary{ActiveScene: "res://Main.tscn"},
	}, now)

	stored, ok, reason := editor.FreshForSession("editor-session", now.Add(time.Second))
	if !ok {
		t.Fatalf("expected snapshot from alias path, reason=%s", reason)
	}
	if stored.SessionID != "editor-session" {
		t.Fatalf("expected editor-session, got %s", stored.SessionID)
	}
}

func TestDefaultEditorStore_ResetAndLoadConcurrent(t *testing.T) {
	ResetDefaultEditorStoreForTests(10 * time.Second)

	const workers = 8
	const iterations = 100

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if DefaultEditorStore() == nil {
					t.Error("DefaultEditorStore returned nil")
					return
				}
				if DefaultStore() == nil {
					t.Error("DefaultStore returned nil")
					return
				}
			}
		}()
	}

	for i := 0; i < iterations; i++ {
		if i%2 == 0 {
			ResetDefaultStoreForTests(10 * time.Second)
			continue
		}
		ResetDefaultEditorStoreForTests(20 * time.Second)
	}

	wg.Wait()
	if got := DefaultEditorStore().StaleAfter(); got != 10*time.Second && got != 20*time.Second {
		t.Fatalf("expected stale_after to be one of test reset values, got %s", got)
	}
}
