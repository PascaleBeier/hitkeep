package shared

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/realtime"
)

type deadlineBlockingRealtimeWriter struct {
	header   http.Header
	deadline chan time.Time
	started  chan struct{}
}

func newDeadlineBlockingRealtimeWriter() *deadlineBlockingRealtimeWriter {
	return &deadlineBlockingRealtimeWriter{
		header:   make(http.Header),
		deadline: make(chan time.Time, 1),
		started:  make(chan struct{}, 1),
	}
}

func (w *deadlineBlockingRealtimeWriter) Header() http.Header { return w.header }

func (w *deadlineBlockingRealtimeWriter) WriteHeader(int) {}

func (w *deadlineBlockingRealtimeWriter) Write([]byte) (int, error) {
	select {
	case w.started <- struct{}{}:
	default:
	}
	deadline := <-w.deadline
	<-time.After(time.Until(deadline))
	return 0, context.DeadlineExceeded
}

func (w *deadlineBlockingRealtimeWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadline <- deadline
	return nil
}

func TestServeRealtimeStreamStopsAtLifetime(t *testing.T) {
	previous := realtimeStreamLifetime
	realtimeStreamLifetime = 0
	t.Cleanup(func() { realtimeStreamLifetime = previous })

	done := make(chan struct{})
	go func() {
		defer close(done)
		ServeRealtimeStream(httptest.NewRecorder(), httptest.NewRequest("GET", "/realtime", nil), realtime.NewBroker(), uuid.New())
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("realtime stream did not stop at its lifetime")
	}
}

func TestServeRealtimeStreamSetsDeadlineBeforePrelude(t *testing.T) {
	previous := realtimeStreamLifetime
	realtimeStreamLifetime = 10 * time.Millisecond
	t.Cleanup(func() { realtimeStreamLifetime = previous })

	writer := newDeadlineBlockingRealtimeWriter()
	done := make(chan struct{})
	go func() {
		defer close(done)
		ServeRealtimeStream(writer, httptest.NewRequest("GET", "/realtime", nil), realtime.NewBroker(), uuid.New())
	}()
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("realtime stream did not begin the prelude write")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("blocked prelude write outlived realtime stream lifetime")
	}
}
