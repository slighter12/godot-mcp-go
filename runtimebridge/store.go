package runtimebridge

import (
	"sync"
	"time"
)

const defaultStaleAfter = 10 * time.Second

var defaultStore = NewStore(defaultStaleAfter)

// Store tracks runtime snapshots by MCP session.
type Store struct {
	mu          sync.RWMutex
	staleAfter  time.Duration
	latestID    string
	bySessionID map[string]StoredSnapshot
}

func NewStore(staleAfter time.Duration) *Store {
	if staleAfter <= 0 {
		staleAfter = defaultStaleAfter
	}
	return &Store{
		staleAfter:  staleAfter,
		bySessionID: make(map[string]StoredSnapshot),
	}
}

func DefaultStore() *Store {
	return defaultStore
}

func (s *Store) Upsert(sessionID string, snapshot Snapshot, now time.Time) {
	if s == nil || sessionID == "" {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if snapshot.NodeDetails == nil {
		snapshot.NodeDetails = map[string]NodeDetail{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.bySessionID[sessionID] = StoredSnapshot{
		SessionID: sessionID,
		Snapshot:  snapshot,
		UpdatedAt: now.UTC(),
	}
	s.latestID = sessionID
}

// Touch updates only the freshness timestamp for an existing session snapshot.
func (s *Store) Touch(sessionID string, now time.Time) bool {
	if s == nil || sessionID == "" {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.bySessionID[sessionID]
	if !ok {
		return false
	}
	stored.UpdatedAt = now.UTC()
	s.bySessionID[sessionID] = stored
	s.latestID = sessionID
	return true
}

func (s *Store) RemoveSession(sessionID string) {
	if s == nil || sessionID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.bySessionID, sessionID)
	if s.latestID == sessionID {
		s.latestID = ""
		var latest StoredSnapshot
		for id, candidate := range s.bySessionID {
			if latest.SessionID == "" || candidate.UpdatedAt.After(latest.UpdatedAt) {
				latest = candidate
				s.latestID = id
			}
		}
	}
}

func (s *Store) LatestFresh(now time.Time) (StoredSnapshot, bool, string) {
	if s == nil {
		return StoredSnapshot{}, false, "runtime_bridge_store_unavailable"
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.latestID == "" {
		return StoredSnapshot{}, false, "runtime_snapshot_missing"
	}
	stored, ok := s.bySessionID[s.latestID]
	if !ok {
		return StoredSnapshot{}, false, "runtime_snapshot_missing"
	}
	return s.validateFreshLocked(stored, now)
}

// FreshForSession returns the latest fresh snapshot for one MCP session.
func (s *Store) FreshForSession(sessionID string, now time.Time) (StoredSnapshot, bool, string) {
	if s == nil {
		return StoredSnapshot{}, false, "runtime_bridge_store_unavailable"
	}
	if sessionID == "" {
		return StoredSnapshot{}, false, "session_missing"
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	stored, ok := s.bySessionID[sessionID]
	if !ok {
		return StoredSnapshot{}, false, "runtime_snapshot_missing"
	}
	return s.validateFreshLocked(stored, now)
}

func (s *Store) StaleAfter() time.Duration {
	if s == nil {
		return defaultStaleAfter
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.staleAfter
}

// ResetDefaultStoreForTests resets the package singleton for deterministic tests.
func ResetDefaultStoreForTests(staleAfter time.Duration) {
	defaultStore = NewStore(staleAfter)
}

func (s *Store) validateFreshLocked(stored StoredSnapshot, now time.Time) (StoredSnapshot, bool, string) {
	if now.Sub(stored.UpdatedAt) > s.staleAfter {
		return StoredSnapshot{}, false, "runtime_snapshot_stale"
	}
	return stored, true, ""
}
