package runtimebridge

import (
	"sync"
	"sync/atomic"
	"time"
)

var defaultRuntimeSnapshotStore atomic.Pointer[RuntimeSnapshotStore]

func init() {
	defaultRuntimeSnapshotStore.Store(NewRuntimeSnapshotStore(defaultStaleAfter, defaultStaleGrace))
}

// RuntimeSnapshotStore tracks live runtime snapshots by game session.
type RuntimeSnapshotStore struct {
	mu          sync.RWMutex
	cond        *sync.Cond
	staleAfter  time.Duration
	staleGrace  time.Duration
	latestID    string
	bySessionID map[string]StoredRuntimeSnapshot
	lastStates  map[string]string
	transitions map[string]uint64
}

func NewRuntimeSnapshotStore(staleAfter time.Duration, staleGrace time.Duration) *RuntimeSnapshotStore {
	if staleAfter <= 0 {
		staleAfter = defaultStaleAfter
	}
	if staleGrace < 0 {
		staleGrace = 0
	}
	store := &RuntimeSnapshotStore{
		staleAfter:  staleAfter,
		staleGrace:  staleGrace,
		bySessionID: make(map[string]StoredRuntimeSnapshot),
		lastStates:  make(map[string]string),
		transitions: make(map[string]uint64),
	}
	store.cond = sync.NewCond(&store.mu)
	return store
}

func DefaultRuntimeSnapshotStore() *RuntimeSnapshotStore {
	if store := defaultRuntimeSnapshotStore.Load(); store != nil {
		return store
	}
	store := NewRuntimeSnapshotStore(defaultStaleAfter, defaultStaleGrace)
	if defaultRuntimeSnapshotStore.CompareAndSwap(nil, store) {
		return store
	}
	return defaultRuntimeSnapshotStore.Load()
}

func ResetDefaultRuntimeSnapshotStoreForTests(staleAfter time.Duration, staleGrace time.Duration) {
	defaultRuntimeSnapshotStore.Store(NewRuntimeSnapshotStore(staleAfter, staleGrace))
}

func (s *RuntimeSnapshotStore) Upsert(sessionID string, snapshot RuntimeSnapshot, now time.Time) {
	if s == nil || sessionID == "" {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if snapshot.NodeDetails == nil {
		snapshot.NodeDetails = map[string]NodeDetail{}
	}
	snapshot.SessionID = sessionID
	if snapshot.UpdatedAt == "" {
		snapshot.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCondLocked()
	s.bySessionID[sessionID] = StoredRuntimeSnapshot{
		SessionID: sessionID,
		Snapshot:  snapshot,
		UpdatedAt: now.UTC(),
	}
	s.latestID = sessionID
	s.observeSessionStateLocked(sessionID, s.bySessionID[sessionID], now)
	s.cond.Broadcast()
}

func (s *RuntimeSnapshotStore) RemoveSession(sessionID string) {
	if s == nil || sessionID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCondLocked()
	delete(s.bySessionID, sessionID)
	delete(s.lastStates, sessionID)
	if s.latestID == sessionID {
		s.latestID = ""
		var latest StoredRuntimeSnapshot
		for id, candidate := range s.bySessionID {
			if latest.SessionID == "" || candidate.UpdatedAt.After(latest.UpdatedAt) {
				latest = candidate
				s.latestID = id
			}
		}
	}
	s.cond.Broadcast()
}

func (s *RuntimeSnapshotStore) FreshForSession(sessionID string, now time.Time) (StoredRuntimeSnapshot, bool, string) {
	if s == nil {
		return StoredRuntimeSnapshot{}, false, "runtime_snapshot_store_unavailable"
	}
	if sessionID == "" {
		return StoredRuntimeSnapshot{}, false, "game_session_missing"
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.bySessionID[sessionID]
	if !ok {
		return StoredRuntimeSnapshot{}, false, "runtime_snapshot_missing"
	}
	return s.validateFreshLocked(sessionID, stored, now)
}

func (s *RuntimeSnapshotStore) StateForSession(sessionID string, now time.Time) (StoredRuntimeSnapshot, string, bool) {
	if s == nil || sessionID == "" {
		return StoredRuntimeSnapshot{}, FreshnessStateMissing, false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.bySessionID[sessionID]
	if !ok {
		return StoredRuntimeSnapshot{}, FreshnessStateMissing, false
	}
	state, _ := s.observeSessionStateLocked(sessionID, stored, now)
	return stored, state, true
}

func (s *RuntimeSnapshotStore) LatestFresh(now time.Time) (StoredRuntimeSnapshot, bool, string) {
	if s == nil {
		return StoredRuntimeSnapshot{}, false, "runtime_snapshot_store_unavailable"
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.latestID == "" {
		return StoredRuntimeSnapshot{}, false, "runtime_snapshot_missing"
	}
	stored, ok := s.bySessionID[s.latestID]
	if !ok {
		return StoredRuntimeSnapshot{}, false, "runtime_snapshot_missing"
	}
	return s.validateFreshLocked(s.latestID, stored, now)
}

func (s *RuntimeSnapshotStore) ConfigureFreshness(staleAfter time.Duration, staleGrace time.Duration) {
	if s == nil {
		return
	}
	if staleAfter <= 0 {
		staleAfter = defaultStaleAfter
	}
	if staleGrace < 0 {
		staleGrace = 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCondLocked()
	s.staleAfter = staleAfter
	s.staleGrace = staleGrace
	now := time.Now().UTC()
	for sessionID, stored := range s.bySessionID {
		s.observeSessionStateLocked(sessionID, stored, now)
	}
	s.cond.Broadcast()
}

func (s *RuntimeSnapshotStore) Await(sessionID string, minFrame int64, timeout time.Duration, minFreshness string) (StoredRuntimeSnapshot, string, bool) {
	if s == nil {
		return StoredRuntimeSnapshot{}, "runtime_snapshot_store_unavailable", false
	}
	if sessionID == "" {
		return StoredRuntimeSnapshot{}, "game_session_missing", false
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	if minFreshness == "" {
		minFreshness = FreshnessStateFresh
	}

	deadline := time.Now().UTC().Add(timeout)
	timer := time.AfterFunc(timeout, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.cond != nil {
			s.cond.Broadcast()
		}
	})
	defer timer.Stop()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCondLocked()

	for {
		now := time.Now().UTC()
		if stored, reason, ok := s.awaitResultLocked(sessionID, minFrame, minFreshness, now, deadline); reason != "" || ok {
			return stored, reason, ok
		}
		s.cond.Wait()
	}
}

func (s *RuntimeSnapshotStore) awaitResultLocked(sessionID string, minFrame int64, minFreshness string, now time.Time, deadline time.Time) (StoredRuntimeSnapshot, string, bool) {
	stored, exists := s.bySessionID[sessionID]
	if exists {
		state, _ := s.observeSessionStateLocked(sessionID, stored, now)
		if stored.Snapshot.Frame >= minFrame && freshnessAtLeast(state, minFreshness) {
			return stored, "", true
		}
		if now.After(deadline) {
			if state == FreshnessStateStale {
				return StoredRuntimeSnapshot{}, "runtime_snapshot_stale", false
			}
			return StoredRuntimeSnapshot{}, "command_timeout", false
		}
		return StoredRuntimeSnapshot{}, "", false
	}
	if now.After(deadline) {
		return StoredRuntimeSnapshot{}, "runtime_snapshot_missing", false
	}
	return StoredRuntimeSnapshot{}, "", false
}

func (s *RuntimeSnapshotStore) ensureCondLocked() {
	if s.cond == nil {
		s.cond = sync.NewCond(&s.mu)
	}
}

func (s *RuntimeSnapshotStore) Health(now time.Time) StoreHealth {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	out := StoreHealth{
		StaleAfterMS:  defaultStaleAfter.Milliseconds(),
		StaleGraceMS:  defaultStaleGrace.Milliseconds(),
		States:        map[string]int{FreshnessStateFresh: 0, FreshnessStateGrace: 0, FreshnessStateStale: 0, FreshnessStateMissing: 0},
		Transitions:   map[string]uint64{},
		SessionHealth: []SessionFreshness{},
	}
	if s == nil {
		out.States[FreshnessStateMissing] = 1
		return out
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	out.StaleAfterMS = s.staleAfter.Milliseconds()
	out.StaleGraceMS = s.staleGrace.Milliseconds()
	out.Sessions = len(s.bySessionID)
	for sessionID, stored := range s.bySessionID {
		state, age := s.observeSessionStateLocked(sessionID, stored, now)
		out.States[state] = out.States[state] + 1
		out.SessionHealth = append(out.SessionHealth, SessionFreshness{
			SessionID: sessionID,
			State:     state,
			AgeMS:     age.Milliseconds(),
			UpdatedAt: stored.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	for key, count := range s.transitions {
		out.Transitions[key] = count
	}
	return out
}

func (s *RuntimeSnapshotStore) validateFreshLocked(sessionID string, stored StoredRuntimeSnapshot, now time.Time) (StoredRuntimeSnapshot, bool, string) {
	state, _ := s.observeSessionStateLocked(sessionID, stored, now)
	if state == FreshnessStateStale {
		return StoredRuntimeSnapshot{}, false, "runtime_snapshot_stale"
	}
	return stored, true, ""
}

func (s *RuntimeSnapshotStore) observeSessionStateLocked(sessionID string, stored StoredRuntimeSnapshot, now time.Time) (string, time.Duration) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	age := now.Sub(stored.UpdatedAt)
	state := freshnessStateForAge(age, s.staleAfter, s.staleGrace)
	last := s.lastStates[sessionID]
	if last != "" && last != state {
		key := last + "->" + state
		s.transitions[key] = s.transitions[key] + 1
	}
	s.lastStates[sessionID] = state
	return state, age
}

func freshnessAtLeast(state string, minState string) bool {
	rank := map[string]int{
		FreshnessStateMissing: 0,
		FreshnessStateStale:   1,
		FreshnessStateGrace:   2,
		FreshnessStateFresh:   3,
	}
	return rank[state] >= rank[minState]
}
