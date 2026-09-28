package imports

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestImportRunnerStopCancelsWorkers(t *testing.T) {
	runner := newImportRunner(&handler{})
	runner.Start(context.Background())

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runner.Stop(stopCtx); err != nil {
		t.Fatalf("stop runner: %v", err)
	}
	if err := runner.TryEnqueue(uuid.New(), uuid.New()); !errors.Is(err, errImportRunnerStopped) {
		t.Fatalf("TryEnqueue after stop error = %v, want %v", err, errImportRunnerStopped)
	}
}

func TestImportRunnerTryEnqueueRejectsFullQueue(t *testing.T) {
	runner := newQueueTestRunner(t)

	for range cap(runner.queue) {
		runner.queue <- importRequest{siteID: uuid.New(), importID: uuid.New()}
	}
	if err := runner.TryEnqueue(uuid.New(), uuid.New()); !errors.Is(err, errImportQueueFull) {
		t.Fatalf("TryEnqueue on full queue error = %v, want %v", err, errImportQueueFull)
	}
}

func TestImportRunnerEnqueueHonorsCallerCancellation(t *testing.T) {
	runner := newQueueTestRunner(t)

	for range cap(runner.queue) {
		runner.queue <- importRequest{siteID: uuid.New(), importID: uuid.New()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := runner.Enqueue(ctx, uuid.New(), uuid.New()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Enqueue cancellation error = %v, want %v", err, context.DeadlineExceeded)
	}
}

func newQueueTestRunner(t *testing.T) *importRunner {
	t.Helper()
	runner := newImportRunner(&handler{})
	runCtx, cancel := context.WithCancel(context.Background())
	runner.mu.Lock()
	runner.ctx = runCtx
	runner.cancel = cancel
	runner.mu.Unlock()
	t.Cleanup(cancel)
	return runner
}
