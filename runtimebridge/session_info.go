package runtimebridge

import "sync"

// SessionInfoProvider returns MCP session diagnostic information.
type SessionInfoProvider interface {
	SessionSummaries() []map[string]any
	SessionCounts() map[string]any
}

var (
	sessionInfoMu       sync.RWMutex
	sessionInfoProvider  SessionInfoProvider
)

// SetSessionInfoProvider registers a provider for MCP session diagnostics.
func SetSessionInfoProvider(provider SessionInfoProvider) {
	sessionInfoMu.Lock()
	defer sessionInfoMu.Unlock()
	sessionInfoProvider = provider
}

// GetSessionSummaries returns session summaries from the registered provider.
func GetSessionSummaries() []map[string]any {
	sessionInfoMu.RLock()
	provider := sessionInfoProvider
	sessionInfoMu.RUnlock()
	if provider == nil {
		return nil
	}
	return provider.SessionSummaries()
}

// GetSessionCounts returns session counts from the registered provider.
func GetSessionCounts() map[string]any {
	sessionInfoMu.RLock()
	provider := sessionInfoProvider
	sessionInfoMu.RUnlock()
	if provider == nil {
		return map[string]any{
			"total":             0,
			"fully_initialized": 0,
			"with_transport":    0,
		}
	}
	return provider.SessionCounts()
}
