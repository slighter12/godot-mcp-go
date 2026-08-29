package http

import "time"

const progressWriteTimeout = 5 * time.Second

type progressStreamRecord struct {
	transport *StreamableHTTPTransport
	canceled  bool
}

// Request-scoped progress is carried on the response SSE stream for the
// tools/call that supplied the progress token. It is intentionally separate
// from subscriptions, which are reserved for long-lived event delivery.
// progressRouteKey must be server-generated rather than a client JSON-RPC id.
func (s *Server) registerProgressStream(progressRouteKey string, transport *StreamableHTTPTransport) {
	if s == nil || transport == nil || progressRouteKey == "" {
		return
	}
	s.progressMu.Lock()
	s.progressStreams[progressRouteKey] = &progressStreamRecord{transport: transport}
	s.progressMu.Unlock()
}

func (s *Server) unregisterProgressStream(progressRouteKey string, transport *StreamableHTTPTransport) {
	if s == nil || progressRouteKey == "" {
		return
	}
	s.progressMu.Lock()
	if current := s.progressStreams[progressRouteKey]; current != nil && current.transport == transport {
		delete(s.progressStreams, progressRouteKey)
	}
	s.progressMu.Unlock()
}

func (s *Server) sendRequestProgress(progressRouteKey string, notification map[string]any) bool {
	if s == nil || progressRouteKey == "" {
		return false
	}
	s.progressMu.RLock()
	record := s.progressStreams[progressRouteKey]
	if record == nil || record.canceled || record.transport == nil {
		s.progressMu.RUnlock()
		return false
	}
	transport := record.transport
	s.progressMu.RUnlock()

	err := transport.SendSSEWithTimeout("message", notification, progressWriteTimeout)
	if err != nil {
		s.unregisterProgressStream(progressRouteKey, transport)
		return false
	}
	return true
}

func (s *Server) markCanceledProgressRequest(progressRouteKey string) {
	if s == nil || progressRouteKey == "" {
		return
	}
	s.progressMu.Lock()
	if record := s.progressStreams[progressRouteKey]; record != nil {
		record.canceled = true
	}
	s.progressMu.Unlock()
}

func (s *Server) isCanceledProgressRequest(progressRouteKey string) bool {
	if s == nil || progressRouteKey == "" {
		return false
	}
	s.progressMu.RLock()
	record := s.progressStreams[progressRouteKey]
	canceled := record != nil && record.canceled
	s.progressMu.RUnlock()
	return canceled
}
