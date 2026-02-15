package http

import (
	"sync"
	"time"
)

// SessionManager manages MCP sessions for Streamable HTTP
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// Session represents an MCP session
type Session struct {
	ID          string
	Created     time.Time
	LastSeen    time.Time
	Initialized bool
	Transport   *StreamableHTTPTransport
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

// CreateSession creates a new session
func (sm *SessionManager) CreateSession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session, exists := sm.sessions[sessionID]; exists {
		session.LastSeen = time.Now()
		return
	}

	sm.sessions[sessionID] = &Session{
		ID:       sessionID,
		Created:  time.Now(),
		LastSeen: time.Now(),
	}
}

// GetSession retrieves a session by ID
func (sm *SessionManager) GetSession(sessionID string) (*Session, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if exists {
		session.LastSeen = time.Now()
	}
	return session, exists
}

// HasSession checks whether a session exists
func (sm *SessionManager) HasSession(sessionID string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	_, exists := sm.sessions[sessionID]
	return exists
}

// TouchSession updates LastSeen for an existing session
func (sm *SessionManager) TouchSession(sessionID string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return false
	}
	session.LastSeen = time.Now()
	return true
}

// SetTransport sets or updates the stream transport for an existing session
func (sm *SessionManager) SetTransport(sessionID string, transport *StreamableHTTPTransport) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return false
	}
	if session.Transport != nil && session.Transport != transport {
		session.Transport.Close()
	}
	session.Transport = transport
	session.LastSeen = time.Now()
	return true
}

// ClearTransport removes the stream transport from a session.
func (sm *SessionManager) ClearTransport(sessionID string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return false
	}
	if session.Transport != nil {
		session.Transport.Close()
		session.Transport = nil
	}
	session.LastSeen = time.Now()
	return true
}

// MarkInitialized marks a session as initialized.
func (sm *SessionManager) MarkInitialized(sessionID string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return false
	}
	session.Initialized = true
	session.LastSeen = time.Now()
	return true
}

// RemoveSession removes a session
func (sm *SessionManager) RemoveSession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session, exists := sm.sessions[sessionID]; exists {
		if session.Transport != nil {
			session.Transport.Close()
		}
		delete(sm.sessions, sessionID)
	}
}

// CleanupSessions removes expired sessions
func (sm *SessionManager) CleanupSessions(timeout time.Duration) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	for sessionID, session := range sm.sessions {
		if now.Sub(session.LastSeen) > timeout {
			if session.Transport != nil {
				session.Transport.Close()
			}
			delete(sm.sessions, sessionID)
		}
	}
}
