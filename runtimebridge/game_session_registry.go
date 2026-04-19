package runtimebridge

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var defaultGameSessionRegistry atomic.Pointer[GameSessionRegistry]

func init() {
	defaultGameSessionRegistry.Store(NewGameSessionRegistry())
}

// GameSessionRegistry tracks editor session <-> game session <-> runtime transport mapping.
type GameSessionRegistry struct {
	mu                sync.RWMutex
	bySessionID       map[string]GameSession
	byEditorSessionID map[string]string
	byRuntimeSession  map[string]string
}

func NewGameSessionRegistry() *GameSessionRegistry {
	return &GameSessionRegistry{
		bySessionID:       make(map[string]GameSession),
		byEditorSessionID: make(map[string]string),
		byRuntimeSession:  make(map[string]string),
	}
}

func DefaultGameSessionRegistry() *GameSessionRegistry {
	if store := defaultGameSessionRegistry.Load(); store != nil {
		return store
	}
	store := NewGameSessionRegistry()
	if defaultGameSessionRegistry.CompareAndSwap(nil, store) {
		return store
	}
	return defaultGameSessionRegistry.Load()
}

func ResetDefaultGameSessionRegistryForTests() {
	defaultGameSessionRegistry.Store(NewGameSessionRegistry())
}

func (r *GameSessionRegistry) UpsertFromRun(sessionID string, editorSessionID string, scenePath string, launchToken string, startedAt time.Time) {
	if r == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	sessionID = strings.TrimSpace(sessionID)
	editorSessionID = strings.TrimSpace(editorSessionID)

	r.mu.Lock()
	defer r.mu.Unlock()
	session := r.bySessionID[sessionID]
	session.SessionID = sessionID
	if editorSessionID != "" {
		if oldEditorSessionID := strings.TrimSpace(session.EditorSessionID); oldEditorSessionID != "" &&
			oldEditorSessionID != editorSessionID &&
			r.byEditorSessionID[oldEditorSessionID] == sessionID {
			delete(r.byEditorSessionID, oldEditorSessionID)
		}
		session.EditorSessionID = editorSessionID
		r.byEditorSessionID[editorSessionID] = sessionID
	}
	if strings.TrimSpace(scenePath) != "" {
		session.ScenePath = strings.TrimSpace(scenePath)
	}
	if strings.TrimSpace(launchToken) != "" {
		session.LaunchToken = strings.TrimSpace(launchToken)
	}
	session.Running = true
	session.StoppedAt = ""
	session.StartedAt = startedAt.UTC().Format(time.RFC3339Nano)
	r.bySessionID[sessionID] = session
}

func (r *GameSessionRegistry) RegisterRuntimeTransport(sessionID string, runtimeSessionID string, editorSessionID string, scenePath string, startedAt time.Time, launchToken string) {
	if r == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	sessionID = strings.TrimSpace(sessionID)
	runtimeSessionID = strings.TrimSpace(runtimeSessionID)
	editorSessionID = strings.TrimSpace(editorSessionID)

	r.mu.Lock()
	defer r.mu.Unlock()
	session := r.bySessionID[sessionID]
	session.SessionID = sessionID
	if editorSessionID != "" {
		if oldEditorSessionID := strings.TrimSpace(session.EditorSessionID); oldEditorSessionID != "" &&
			oldEditorSessionID != editorSessionID &&
			r.byEditorSessionID[oldEditorSessionID] == sessionID {
			delete(r.byEditorSessionID, oldEditorSessionID)
		}
		session.EditorSessionID = editorSessionID
		r.byEditorSessionID[editorSessionID] = sessionID
	}
	if scenePath = strings.TrimSpace(scenePath); scenePath != "" {
		session.ScenePath = scenePath
	}
	if launchToken = strings.TrimSpace(launchToken); launchToken != "" {
		session.LaunchToken = launchToken
	}
	if runtimeSessionID != "" {
		if previousOwnerSessionID := strings.TrimSpace(r.byRuntimeSession[runtimeSessionID]); previousOwnerSessionID != "" &&
			previousOwnerSessionID != sessionID {
			if previousOwnerSession, ok := r.bySessionID[previousOwnerSessionID]; ok {
				// Transfer ownership to the new game session and clear stale ownership
				// from the previous one so stop/remove paths do not delete the new index.
				if strings.TrimSpace(previousOwnerSession.RuntimeSessionID) == runtimeSessionID {
					previousOwnerSession.RuntimeSessionID = ""
					r.bySessionID[previousOwnerSessionID] = previousOwnerSession
				}
			}
		}
		if old := strings.TrimSpace(session.RuntimeSessionID); old != "" && old != runtimeSessionID &&
			r.byRuntimeSession[old] == sessionID {
			delete(r.byRuntimeSession, old)
		}
		session.RuntimeSessionID = runtimeSessionID
		r.byRuntimeSession[runtimeSessionID] = sessionID
	}
	session.Running = true
	session.StoppedAt = ""
	if session.StartedAt == "" {
		session.StartedAt = startedAt.UTC().Format(time.RFC3339Nano)
	}
	r.bySessionID[sessionID] = session
}

func (r *GameSessionRegistry) MarkSnapshotReceived(sessionID string, at time.Time) {
	if r == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.bySessionID[strings.TrimSpace(sessionID)]
	if !ok {
		return
	}
	session.HasSnapshot = true
	session.LastSnapshotAt = at.UTC().Format(time.RFC3339Nano)
	session.Running = true
	r.bySessionID[session.SessionID] = session
}

func (r *GameSessionRegistry) StopSession(sessionID string, stoppedAt time.Time) {
	if r == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	if stoppedAt.IsZero() {
		stoppedAt = time.Now().UTC()
	}
	sessionID = strings.TrimSpace(sessionID)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopSessionLocked(sessionID, stoppedAt)
}

func (r *GameSessionRegistry) stopSessionLocked(sessionID string, stoppedAt time.Time) bool {
	session, ok := r.bySessionID[sessionID]
	if !ok {
		return false
	}
	session.Running = false
	session.StoppedAt = stoppedAt.UTC().Format(time.RFC3339Nano)
	if strings.TrimSpace(session.RuntimeSessionID) != "" {
		delete(r.byRuntimeSession, session.RuntimeSessionID)
		session.RuntimeSessionID = ""
	}
	r.bySessionID[session.SessionID] = session
	return true
}

func (r *GameSessionRegistry) StopByEditorSession(editorSessionID string, stoppedAt time.Time) (string, bool) {
	if r == nil || strings.TrimSpace(editorSessionID) == "" {
		return "", false
	}
	if stoppedAt.IsZero() {
		stoppedAt = time.Now().UTC()
	}
	editorSessionID = strings.TrimSpace(editorSessionID)

	r.mu.Lock()
	defer r.mu.Unlock()
	sessionID := strings.TrimSpace(r.byEditorSessionID[editorSessionID])
	if strings.TrimSpace(sessionID) == "" {
		return "", false
	}
	if !r.stopSessionLocked(sessionID, stoppedAt) {
		delete(r.byEditorSessionID, editorSessionID)
		return "", false
	}
	return sessionID, true
}

func (r *GameSessionRegistry) Session(sessionID string) (GameSession, bool) {
	if r == nil || strings.TrimSpace(sessionID) == "" {
		return GameSession{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	session, ok := r.bySessionID[strings.TrimSpace(sessionID)]
	return session, ok
}

func (r *GameSessionRegistry) ActiveForEditor(editorSessionID string) (GameSession, bool) {
	if r == nil || strings.TrimSpace(editorSessionID) == "" {
		return GameSession{}, false
	}
	editorSessionID = strings.TrimSpace(editorSessionID)
	r.mu.RLock()
	defer r.mu.RUnlock()
	sessionID := strings.TrimSpace(r.byEditorSessionID[editorSessionID])
	if sessionID == "" {
		return GameSession{}, false
	}
	session, ok := r.bySessionID[sessionID]
	if !ok || !session.Running {
		return GameSession{}, false
	}
	return session, true
}

func (r *GameSessionRegistry) RuntimeSessionID(gameSessionID string) (string, bool) {
	if r == nil || strings.TrimSpace(gameSessionID) == "" {
		return "", false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	session, ok := r.bySessionID[strings.TrimSpace(gameSessionID)]
	if !ok || !session.Running || strings.TrimSpace(session.RuntimeSessionID) == "" {
		return "", false
	}
	return strings.TrimSpace(session.RuntimeSessionID), true
}

func (r *GameSessionRegistry) MatchesLaunchToken(gameSessionID string, launchToken string) bool {
	if r == nil || strings.TrimSpace(gameSessionID) == "" {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	session, ok := r.bySessionID[strings.TrimSpace(gameSessionID)]
	if !ok {
		return false
	}
	return strings.TrimSpace(session.LaunchToken) == strings.TrimSpace(launchToken)
}

func (r *GameSessionRegistry) RuntimeSessionMatches(gameSessionID string, runtimeSessionID string) bool {
	if r == nil || strings.TrimSpace(gameSessionID) == "" || strings.TrimSpace(runtimeSessionID) == "" {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	session, ok := r.bySessionID[strings.TrimSpace(gameSessionID)]
	if !ok {
		return false
	}
	return strings.TrimSpace(session.RuntimeSessionID) == strings.TrimSpace(runtimeSessionID)
}

func (r *GameSessionRegistry) GameSessionIDByRuntimeSession(runtimeSessionID string) (string, bool) {
	if r == nil || strings.TrimSpace(runtimeSessionID) == "" {
		return "", false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	sessionID := strings.TrimSpace(r.byRuntimeSession[strings.TrimSpace(runtimeSessionID)])
	if sessionID == "" {
		return "", false
	}
	return sessionID, true
}

func (r *GameSessionRegistry) IsRunning(sessionID string) bool {
	session, ok := r.Session(sessionID)
	return ok && session.Running
}

func (r *GameSessionRegistry) RemoveSession(sessionID string) {
	if r == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removeSessionLocked(sessionID)
}

func (r *GameSessionRegistry) removeSessionLocked(sessionID string) bool {
	session, ok := r.bySessionID[sessionID]
	if !ok {
		return false
	}
	delete(r.bySessionID, sessionID)
	if strings.TrimSpace(session.EditorSessionID) != "" && r.byEditorSessionID[session.EditorSessionID] == sessionID {
		delete(r.byEditorSessionID, session.EditorSessionID)
	}
	if strings.TrimSpace(session.RuntimeSessionID) != "" && r.byRuntimeSession[session.RuntimeSessionID] == sessionID {
		delete(r.byRuntimeSession, session.RuntimeSessionID)
	}
	return true
}

func (r *GameSessionRegistry) RemoveByEditorSession(editorSessionID string) {
	if r == nil || strings.TrimSpace(editorSessionID) == "" {
		return
	}
	editorSessionID = strings.TrimSpace(editorSessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	sessionID := strings.TrimSpace(r.byEditorSessionID[editorSessionID])
	if sessionID == "" {
		return
	}
	if !r.removeSessionLocked(sessionID) {
		delete(r.byEditorSessionID, editorSessionID)
	}
}

// StopStaleForEditor stops all running game sessions for one editor session
// that have no runtime transport registered.  These are zombie sessions left
// behind by project.run calls that timed out waiting for the first snapshot.
// The provided excludeSessionID (the session about to be created) is skipped.
func (r *GameSessionRegistry) StopStaleForEditor(editorSessionID string, excludeSessionID string, stoppedAt time.Time) int {
	if r == nil || strings.TrimSpace(editorSessionID) == "" {
		return 0
	}
	if stoppedAt.IsZero() {
		stoppedAt = time.Now().UTC()
	}
	editorSessionID = strings.TrimSpace(editorSessionID)
	excludeSessionID = strings.TrimSpace(excludeSessionID)

	r.mu.Lock()
	defer r.mu.Unlock()

	stopped := 0
	for id, session := range r.bySessionID {
		if !session.Running {
			continue
		}
		if id == excludeSessionID {
			continue
		}
		if strings.TrimSpace(session.EditorSessionID) != editorSessionID {
			continue
		}
		if strings.TrimSpace(session.RuntimeSessionID) != "" {
			continue
		}
		r.stopSessionLocked(id, stoppedAt)
		stopped++
	}
	return stopped
}

// LatestRunning returns the most recently started running game session.
func (r *GameSessionRegistry) LatestRunning() (GameSession, bool) {
	if r == nil {
		return GameSession{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var best GameSession
	found := false
	for _, session := range r.bySessionID {
		if !session.Running {
			continue
		}
		if !found || session.StartedAt > best.StartedAt {
			best = session
			found = true
		}
	}
	return best, found
}

func (r *GameSessionRegistry) Health() map[string]any {
	if r == nil {
		return map[string]any{
			"sessions": 0,
		}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	running := 0
	for _, session := range r.bySessionID {
		if session.Running {
			running++
		}
	}
	return map[string]any{
		"sessions": len(r.bySessionID),
		"running":  running,
	}
}
