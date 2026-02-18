package runtimebridge

import (
	"testing"
	"time"
)

func TestStoreLatestFreshAndStale(t *testing.T) {
	store := NewStore(10 * time.Second)
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
	store := NewStore(10 * time.Second)
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
	store := NewStore(10 * time.Second)
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
	store := NewStore(10 * time.Second)
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
