package webhookdispatcher

import (
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/config"
)

func TestWorkerEndpointAdmissionDoesNotBlock(t *testing.T) {
	worker := NewWorker(nil, nil, config.Config{WebhookPerEndpointConcurrency: 1}, testLogger(), 0)
	webhookID := uuid.New()
	release, ok := worker.tryAcquire(webhookID)
	if !ok {
		t.Fatal("expected first endpoint admission")
	}
	if _, ok := worker.tryAcquire(webhookID); ok {
		t.Fatal("expected saturated endpoint admission to fail without blocking")
	}
	release()
	if release, ok = worker.tryAcquire(webhookID); !ok {
		t.Fatal("expected endpoint admission after release")
	}
	release()
}
