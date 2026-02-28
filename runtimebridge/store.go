package runtimebridge

import (
	"strconv"
	"sync"
	"time"
)

const defaultStaleAfter = 10 * time.Second
const defaultStaleGrace = 0 * time.Millisecond

const (
	FreshnessStateMissing = "missing"
	FreshnessStateFresh   = "fresh"
	FreshnessStateGrace   = "grace"
	FreshnessStateStale   = "stale"
)

var defaultStore = NewStore(defaultStaleAfter, defaultStaleGrace)

// SessionFreshness stores one session freshness snapshot for observability endpoints.
type SessionFreshness struct {
	SessionID string `json:"session_id"`
	State     string `json:"state"`
	AgeMS     int64  `json:"age_ms"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// StoreHealth contains aggregate runtime freshness health.
type StoreHealth struct {
	StaleAfterMS  int64              `json:"stale_after_ms"`
	StaleGraceMS  int64              `json:"stale_grace_ms"`
	Sessions      int                `json:"sessions"`
	States        map[string]int     `json:"states"`
	Transitions   map[string]uint64  `json:"transitions"`
	SessionHealth []SessionFreshness `json:"session_health"`
}

// Store tracks runtime snapshots by MCP session.
type Store struct {
	mu          sync.RWMutex
	staleAfter  time.Duration
	staleGrace  time.Duration
	latestID    string
	bySessionID map[string]StoredSnapshot
	lastStates  map[string]string
	transitions map[string]uint64
}

func NewStore(staleAfter time.Duration, staleGrace time.Duration) *Store {
	if staleAfter <= 0 {
		staleAfter = defaultStaleAfter
	}
	if staleGrace < 0 {
		staleGrace = 0
	}
	return &Store{
		staleAfter:  staleAfter,
		staleGrace:  staleGrace,
		bySessionID: make(map[string]StoredSnapshot),
		lastStates:  make(map[string]string),
		transitions: make(map[string]uint64),
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
	s.observeSessionStateLocked(sessionID, s.bySessionID[sessionID], now)
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
	s.observeSessionStateLocked(sessionID, stored, now)
	return true
}

func (s *Store) RemoveSession(sessionID string) {
	if s == nil || sessionID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.bySessionID, sessionID)
	delete(s.lastStates, sessionID)
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

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.latestID == "" {
		return StoredSnapshot{}, false, "runtime_snapshot_missing"
	}
	stored, ok := s.bySessionID[s.latestID]
	if !ok {
		return StoredSnapshot{}, false, "runtime_snapshot_missing"
	}
	return s.validateFreshLocked(s.latestID, stored, now)
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

	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.bySessionID[sessionID]
	if !ok {
		return StoredSnapshot{}, false, "runtime_snapshot_missing"
	}
	return s.validateFreshLocked(sessionID, stored, now)
}

func (s *Store) StaleAfter() time.Duration {
	if s == nil {
		return defaultStaleAfter
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.staleAfter
}

func (s *Store) StaleGrace() time.Duration {
	if s == nil {
		return defaultStaleGrace
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.staleGrace
}

func (s *Store) ConfigureFreshness(staleAfter time.Duration, staleGrace time.Duration) {
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
	s.staleAfter = staleAfter
	s.staleGrace = staleGrace
	now := time.Now().UTC()
	for sessionID, stored := range s.bySessionID {
		s.observeSessionStateLocked(sessionID, stored, now)
	}
}

func (s *Store) Health(now time.Time) StoreHealth {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	out := StoreHealth{
		StaleAfterMS:  s.StaleAfter().Milliseconds(),
		StaleGraceMS:  s.StaleGrace().Milliseconds(),
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

	out.Sessions = len(s.bySessionID)
	out.SessionHealth = make([]SessionFreshness, 0, len(s.bySessionID))
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

// ResetDefaultStoreForTests resets the package singleton for deterministic tests.
func ResetDefaultStoreForTests(staleAfter time.Duration) {
	defaultStore = NewStore(staleAfter, defaultStaleGrace)
}

func (s *Store) validateFreshLocked(sessionID string, stored StoredSnapshot, now time.Time) (StoredSnapshot, bool, string) {
	state, _ := s.observeSessionStateLocked(sessionID, stored, now)
	if state == FreshnessStateStale {
		return StoredSnapshot{}, false, "runtime_snapshot_stale"
	}
	return stored, true, ""
}

func (s *Store) observeSessionStateLocked(sessionID string, stored StoredSnapshot, now time.Time) (string, time.Duration) {
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

func freshnessStateForAge(age time.Duration, staleAfter time.Duration, staleGrace time.Duration) string {
	if age <= staleAfter {
		return FreshnessStateFresh
	}
	if age <= staleAfter+staleGrace {
		return FreshnessStateGrace
	}
	return FreshnessStateStale
}

func (s StoreHealth) TransitionCount(from, to string) uint64 {
	if s.Transitions == nil {
		return 0
	}
	return s.Transitions[from+"->"+to]
}

func (s StoreHealth) HasState(state string) bool {
	count, ok := s.States[state]
	return ok && count > 0
}

func (s SessionFreshness) AgeString() string {
	return strconv.FormatInt(s.AgeMS, 10) + "ms"
}

// HealthSnapshot returns consolidated runtime bridge health and metrics.
func HealthSnapshot(now time.Time) map[string]any {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	storeHealth := DefaultStore().Health(now)
	commandMetrics := DefaultCommandBroker().Metrics()

	return map[string]any{
		"timestamp": now.Format(time.RFC3339Nano),
		"freshness": map[string]any{
			"stale_after_ms": storeHealth.StaleAfterMS,
			"stale_grace_ms": storeHealth.StaleGraceMS,
			"sessions":       storeHealth.Sessions,
			"states":         storeHealth.States,
			"transitions":    storeHealth.Transitions,
			"session_health": storeHealth.SessionHealth,
		},
		"command_broker": commandMetrics,
	}
}
