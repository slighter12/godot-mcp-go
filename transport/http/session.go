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
	ID        string
	Created   time.Time
	LastSeen  time.Time
	Transport *StreamableHTTPTransport
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

// CreateSession creates a new session
func (sm *SessionManager) CreateSession(sessionID string, transport *StreamableHTTPTransport) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.sessions[sessionID] = &Session{
		ID:        sessionID,
		Created:   time.Now(),
		LastSeen:  time.Now(),
		Transport: transport,
	}
}

// GetSession retrieves a session by ID
func (sm *SessionManager) GetSession(sessionID string) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[sessionID]
	if exists {
		session.LastSeen = time.Now()
	}
	return session, exists
}

// RemoveSession removes a session
func (sm *SessionManager) RemoveSession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session, exists := sm.sessions[sessionID]; exists {
		session.Transport.Close()
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
			session.Transport.Close()
			delete(sm.sessions, sessionID)
		}
	}
}
