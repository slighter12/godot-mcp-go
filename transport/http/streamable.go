package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// StreamableHTTPTransport handles MCP communication over Streamable HTTP
type StreamableHTTPTransport struct {
	writer  http.ResponseWriter
	flusher http.Flusher
	mu      sync.Mutex
	closed  bool
}

// NewStreamableHTTPTransport creates a new Streamable HTTP transport
func NewStreamableHTTPTransport(w http.ResponseWriter, f http.Flusher) *StreamableHTTPTransport {
	return &StreamableHTTPTransport{
		writer:  w,
		flusher: f,
	}
}

// Send sends a JSON-RPC message through the Streamable HTTP transport
func (t *StreamableHTTPTransport) Send(message any) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("transport is closed")
	}

	// Marshal the message to JSON
	dataJSON, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Write the JSON-RPC message as a single JSON object
	_, err = t.writer.Write(dataJSON)
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	// Add newline delimiter as per JSON-RPC spec
	_, err = t.writer.Write([]byte("\n"))
	if err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	t.flusher.Flush()
	return nil
}

// SendSSE sends a message through SSE stream (for server-to-client communication)
func (t *StreamableHTTPTransport) SendSSE(event string, data any) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("transport is closed")
	}

	// Marshal the data to JSON
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal SSE data: %w", err)
	}

	// Format as SSE event
	sseMessage := fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(dataJSON))
	_, err = t.writer.Write([]byte(sseMessage))
	if err != nil {
		return fmt.Errorf("failed to write SSE message: %w", err)
	}

	t.flusher.Flush()
	return nil
}

// Close closes the Streamable HTTP transport
func (t *StreamableHTTPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.closed {
		t.closed = true
	}
	return nil
}

// IsClosed returns true if the transport is closed
func (t *StreamableHTTPTransport) IsClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}
