package mcp

import (
	"sync"
	"time"
)

// ServerInfo represents information about a registered server
type ServerInfo struct {
	ID         string
	Tools      []Tool
	CreatedAt  time.Time
	LastSeen   time.Time
	Persistent bool
}

// ClientInfo represents information about a connected client
type ClientInfo struct {
	ID          string
	ServerID    string
	CreatedAt   time.Time
	LastSeen    time.Time
	Initialized bool
}

// Registry manages server and client registrations
type Registry struct {
	servers map[string]*ServerInfo
	clients map[string]*ClientInfo
	mu      sync.RWMutex
}

// NewRegistry creates a new registry
func NewRegistry() *Registry {
	return &Registry{
		servers: make(map[string]*ServerInfo),
		clients: make(map[string]*ClientInfo),
	}
}

// RegisterServer registers a new server
func (r *Registry) RegisterServer(id string, tools []Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.servers[id]; exists {
		return ErrServerAlreadyRegistered
	}

	r.servers[id] = &ServerInfo{
		ID:        id,
		Tools:     tools,
		CreatedAt: time.Now(),
		LastSeen:  time.Now(),
	}
	return nil
}

// RegisterClient registers a new client
func (r *Registry) RegisterClient(id string, serverID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.clients[id]; exists {
		return ErrClientAlreadyRegistered
	}

	if _, exists := r.servers[serverID]; !exists {
		return ErrServerNotFound
	}

	r.clients[id] = &ClientInfo{
		ID:          id,
		ServerID:    serverID,
		CreatedAt:   time.Now(),
		LastSeen:    time.Now(),
		Initialized: false,
	}
	return nil
}

// InitializeClient marks a client as initialized
func (r *Registry) InitializeClient(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	client, exists := r.clients[id]
	if !exists {
		return ErrClientNotFound
	}

	client.Initialized = true
	client.LastSeen = time.Now()
	return nil
}

// UpdateLastSeen updates the last seen timestamp for a server or client
func (r *Registry) UpdateLastSeen(id string, isServer bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if isServer {
		server, exists := r.servers[id]
		if !exists {
			return ErrServerNotFound
		}
		server.LastSeen = time.Now()
	} else {
		client, exists := r.clients[id]
		if !exists {
			return ErrClientNotFound
		}
		client.LastSeen = time.Now()
	}
	return nil
}

// SetPersistence sets the persistent flag for a server
func (r *Registry) SetPersistence(id string, persistent bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	server, exists := r.servers[id]
	if !exists {
		return ErrServerNotFound
	}
	server.Persistent = persistent
	return nil
}

// IsClientInitialized checks if a client is initialized
func (r *Registry) IsClientInitialized(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	client, exists := r.clients[id]
	if !exists {
		return false
	}
	return client.Initialized
}

// IsServerRegistered checks if a server is registered
func (r *Registry) IsServerRegistered(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.servers[id]
	return exists
}

// GetServerTools returns the tools registered for a server
func (r *Registry) GetServerTools(id string) ([]Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	server, exists := r.servers[id]
	if !exists {
		return nil, ErrServerNotFound
	}
	return server.Tools, nil
}

// Cleanup removes inactive servers and clients
func (r *Registry) Cleanup(timeout time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	// Cleanup servers
	for id, server := range r.servers {
		if !server.Persistent && now.Sub(server.LastSeen) > timeout {
			delete(r.servers, id)
		}
	}

	// Cleanup clients
	for id, client := range r.clients {
		if now.Sub(client.LastSeen) > timeout {
			delete(r.clients, id)
		}
	}
}

// Errors
var (
	ErrServerAlreadyRegistered = &Error{Code: 400, Message: "server already registered"}
	ErrClientAlreadyRegistered = &Error{Code: 400, Message: "client already registered"}
	ErrServerNotFound          = &Error{Code: 404, Message: "server not found"}
	ErrClientNotFound          = &Error{Code: 404, Message: "client not found"}
)

// Error represents a registry error
type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string {
	return e.Message
}
