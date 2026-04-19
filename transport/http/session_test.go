package http

import (
	"testing"
)

func TestSessionManager_SessionSummaries(t *testing.T) {
	sm := NewSessionManager()
	sm.CreateSession("s1")
	sm.MarkInitializeAccepted("s1")
	sm.MarkInitialized("s1")
	sm.SetMutatingAllowed("s1", true)

	sm.CreateSession("s2")
	sm.MarkInitializeAccepted("s2")
	// s2 not yet fully initialized

	summaries := sm.SessionSummaries()
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}

	byID := map[string]map[string]any{}
	for _, s := range summaries {
		byID[s["session_id"].(string)] = s
	}

	s1 := byID["s1"]
	if s1["initialize_accepted"] != true || s1["initialized"] != true || s1["mutating"] != true {
		t.Fatalf("unexpected s1 summary: %v", s1)
	}
	s2 := byID["s2"]
	if s2["initialize_accepted"] != true || s2["initialized"] != false {
		t.Fatalf("unexpected s2 summary: %v", s2)
	}
}

func TestSessionManager_SessionCounts(t *testing.T) {
	sm := NewSessionManager()
	sm.CreateSession("s1")
	sm.MarkInitializeAccepted("s1")
	sm.MarkInitialized("s1")

	sm.CreateSession("s2")
	sm.MarkInitializeAccepted("s2")
	sm.MarkInitialized("s2")

	sm.CreateSession("s3")
	sm.MarkInitializeAccepted("s3")
	// s3 not fully initialized

	counts := sm.SessionCounts()
	if counts["total"] != 3 {
		t.Fatalf("expected total=3, got %v", counts["total"])
	}
	if counts["fully_initialized"] != 2 {
		t.Fatalf("expected fully_initialized=2, got %v", counts["fully_initialized"])
	}
	if counts["with_transport"] != 0 {
		t.Fatalf("expected with_transport=0, got %v", counts["with_transport"])
	}
}
