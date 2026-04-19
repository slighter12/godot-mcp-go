package runtimebridge

import (
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const defaultRuntimeLogCapacity = 2000

var defaultRuntimeLogStore atomic.Pointer[RuntimeLogStore]

func init() {
	defaultRuntimeLogStore.Store(NewRuntimeLogStore(defaultRuntimeLogCapacity))
}

// RuntimeLogStore tracks session-scoped runtime log streams.
type RuntimeLogStore struct {
	mu       sync.RWMutex
	capacity int
	bySess   map[string][]RuntimeLogEntry
	nextSeq  map[string]int64
}

func NewRuntimeLogStore(capacity int) *RuntimeLogStore {
	if capacity <= 0 {
		capacity = defaultRuntimeLogCapacity
	}
	return &RuntimeLogStore{
		capacity: capacity,
		bySess:   make(map[string][]RuntimeLogEntry),
		nextSeq:  make(map[string]int64),
	}
}

func DefaultRuntimeLogStore() *RuntimeLogStore {
	if store := defaultRuntimeLogStore.Load(); store != nil {
		return store
	}
	store := NewRuntimeLogStore(defaultRuntimeLogCapacity)
	if defaultRuntimeLogStore.CompareAndSwap(nil, store) {
		return store
	}
	return defaultRuntimeLogStore.Load()
}

func ResetDefaultRuntimeLogStoreForTests(capacity int) {
	defaultRuntimeLogStore.Store(NewRuntimeLogStore(capacity))
}

func (s *RuntimeLogStore) Append(sessionID string, entries []RuntimeLogAppendEntry, now time.Time) []RuntimeLogEntry {
	if s == nil || strings.TrimSpace(sessionID) == "" || len(entries) == 0 {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	sessionID = strings.TrimSpace(sessionID)

	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.bySess[sessionID]
	seq := s.nextSeq[sessionID]
	if seq <= 0 {
		seq = 1
	}

	out := make([]RuntimeLogEntry, 0, len(entries))
	for _, in := range entries {
		level := normalizeLogLevel(in.Level)
		if level == "" {
			level = "info"
		}
		ts := strings.TrimSpace(in.Time)
		if ts == "" {
			ts = now.Format(time.RFC3339Nano)
		}
		entry := RuntimeLogEntry{
			Sequence:   seq,
			Time:       ts,
			Level:      level,
			Message:    strings.TrimSpace(in.Message),
			Source:     strings.TrimSpace(in.Source),
			StackTrace: strings.TrimSpace(in.StackTrace),
		}
		seq++
		current = append(current, entry)
		out = append(out, entry)
	}
	if len(current) > s.capacity {
		current = current[len(current)-s.capacity:]
	}
	s.bySess[sessionID] = current
	s.nextSeq[sessionID] = seq
	return out
}

func (s *RuntimeLogStore) Get(sessionID string, level string, limit int, sinceSequence int64) []RuntimeLogEntry {
	if s == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	level = normalizeLogLevel(level)
	if level == "" {
		level = "all"
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	items := s.bySess[strings.TrimSpace(sessionID)]
	if len(items) == 0 {
		return []RuntimeLogEntry{}
	}

	filtered := make([]RuntimeLogEntry, 0, min(limit, len(items)))
	for _, entry := range items {
		if entry.Sequence <= sinceSequence {
			continue
		}
		if level != "all" && normalizeLogLevel(entry.Level) != level {
			continue
		}
		filtered = append(filtered, entry)
	}
	if len(filtered) == 0 {
		return []RuntimeLogEntry{}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Sequence < filtered[j].Sequence
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered
}

func (s *RuntimeLogStore) Clear(sessionID string) int {
	if s == nil || strings.TrimSpace(sessionID) == "" {
		return 0
	}
	sessionID = strings.TrimSpace(sessionID)

	s.mu.Lock()
	defer s.mu.Unlock()
	cleared := len(s.bySess[sessionID])
	delete(s.bySess, sessionID)
	delete(s.nextSeq, sessionID)
	return cleared
}

func (s *RuntimeLogStore) RemoveSession(sessionID string) {
	_ = s.Clear(sessionID)
}

func (s *RuntimeLogStore) Health() map[string]any {
	if s == nil {
		return map[string]any{
			"sessions": 0,
			"entries":  0,
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := 0
	for _, items := range s.bySess {
		total += len(items)
	}
	return map[string]any{
		"sessions": len(s.bySess),
		"entries":  total,
	}
}

func normalizeLogLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return "debug"
	case "info":
		return "info"
	case "warning", "warn":
		return "warning"
	case "error":
		return "error"
	case "all":
		return "all"
	default:
		return ""
	}
}
