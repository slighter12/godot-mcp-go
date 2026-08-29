package http

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestSubscriptionManagerCloseAllStartsWritesConcurrently(t *testing.T) {
	manager := NewSubscriptionManager()
	writerA := newBlockingSSEWriter()
	writerB := newBlockingSSEWriter()
	var releaseOnce sync.Once
	releaseWriters := func() {
		releaseOnce.Do(func() {
			close(writerA.release)
			close(writerB.release)
		})
	}
	defer releaseWriters()
	transportA := NewStreamableHTTPTransport(writerA, writerA)
	transportB := NewStreamableHTTPTransport(writerB, writerB)
	if err := manager.Open("route-a", 1, "", transportA, nil, false, map[string]any{}); err != nil {
		t.Fatalf("open subscription A: %v", err)
	}
	if err := manager.Open("route-b", 2, "", transportB, nil, false, map[string]any{}); err != nil {
		t.Fatalf("open subscription B: %v", err)
	}

	done := make(chan struct{})
	go func() {
		manager.CloseAll()
		close(done)
	}()

	select {
	case <-writerA.entered:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("subscription A was not written")
	}
	select {
	case <-writerB.entered:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("subscription B write did not start concurrently")
	}

	releaseWriters()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("CloseAll did not finish after writes were released")
	}
}

type deadlineSSEWriter struct {
	mu       sync.Mutex
	deadline time.Time
	started  chan struct{}
	once     sync.Once
	header   http.Header
}

func newDeadlineSSEWriter() *deadlineSSEWriter {
	return &deadlineSSEWriter{
		started: make(chan struct{}),
		header:  make(http.Header),
	}
}

func (w *deadlineSSEWriter) Header() http.Header {
	return w.header
}

func (w *deadlineSSEWriter) WriteHeader(int) {}

func (w *deadlineSSEWriter) Flush() {}

func (w *deadlineSSEWriter) SetWriteDeadline(deadline time.Time) error {
	w.mu.Lock()
	w.deadline = deadline
	w.mu.Unlock()
	return nil
}

func (w *deadlineSSEWriter) Write(payload []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	w.mu.Lock()
	deadline := w.deadline
	w.mu.Unlock()
	if deadline.IsZero() {
		return 0, errors.New("write deadline was not configured")
	}
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	<-timer.C
	return 0, errors.New("write deadline exceeded")
}

func TestSubscriptionManagerCloseAllHonorsContextDeadline(t *testing.T) {
	manager := NewSubscriptionManager()
	writer := newDeadlineSSEWriter()
	transport := NewStreamableHTTPTransport(writer, writer)
	if err := manager.Open("deadline-route", 1, "", transport, nil, false, map[string]any{}); err != nil {
		t.Fatalf("open subscription: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	startedAt := time.Now()
	manager.closeAll(ctx)
	if elapsed := time.Since(startedAt); elapsed > 500*time.Millisecond {
		t.Fatalf("CloseAll exceeded shutdown deadline bound: %v", elapsed)
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for !transport.IsClosed() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !transport.IsClosed() {
		t.Fatal("subscription transport was not closed after deadline cleanup")
	}
}
