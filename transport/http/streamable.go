package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// StreamableHTTPTransport provides optional SSE writer utilities.
// The current request handling path in router.go is still response-based.
type StreamableHTTPTransport struct {
	writer  http.ResponseWriter
	flusher http.Flusher
	mu      sync.Mutex
	closed  bool
	onClose func()
	once    sync.Once
}

// NewStreamableHTTPTransport creates a new Streamable HTTP transport
func NewStreamableHTTPTransport(w http.ResponseWriter, f http.Flusher, onClose ...func()) *StreamableHTTPTransport {
	var closeHook func()
	if len(onClose) > 0 {
		closeHook = onClose[0]
	}
	return &StreamableHTTPTransport{
		writer:  w,
		flusher: f,
		onClose: closeHook,
	}
}

// SendSSE sends a message through SSE stream (for server-to-client communication)
func (t *StreamableHTTPTransport) SendSSE(event string, data any) error {
	return t.SendSSEWithTimeout(event, data, 0)
}

// SendSSEWithTimeout sends SSE data with an optional per-write timeout.
func (t *StreamableHTTPTransport) SendSSEWithTimeout(event string, data any, timeout time.Duration) error {
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

	sseMessage := fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(dataJSON))
	if err := t.writeLocked(sseMessage, timeout); err != nil {
		return fmt.Errorf("failed to write SSE message: %w", err)
	}
	return nil
}

// SendComment writes one SSE comment frame (":" prefixed lines).
func (t *StreamableHTTPTransport) SendComment(comment string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("transport is closed")
	}

	comment = strings.ReplaceAll(comment, "\r\n", "\n")
	comment = strings.ReplaceAll(comment, "\r", "\n")
	comment = strings.ReplaceAll(comment, "\n", "\n: ")
	frame := fmt.Sprintf(": %s\n\n", comment)
	if err := t.writeLocked(frame, 0); err != nil {
		return fmt.Errorf("failed to write SSE comment: %w", err)
	}
	return nil
}

func (t *StreamableHTTPTransport) writeLocked(payload string, timeout time.Duration) error {
	if timeout > 0 {
		controller := http.NewResponseController(t.writer)
		if err := controller.SetWriteDeadline(time.Now().Add(timeout)); err == nil {
			defer func() {
				_ = controller.SetWriteDeadline(time.Time{})
			}()
		}
	}

	_, err := t.writer.Write([]byte(payload))
	if err != nil {
		return err
	}
	t.flusher.Flush()
	return nil
}

// Close closes the Streamable HTTP transport
func (t *StreamableHTTPTransport) Close() error {
	t.mu.Lock()
	wasOpen := !t.closed
	if wasOpen {
		t.closed = true
	}
	t.mu.Unlock()

	if wasOpen && t.onClose != nil {
		t.once.Do(t.onClose)
	}
	return nil
}

// IsClosed returns true if the transport is closed
func (t *StreamableHTTPTransport) IsClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}
