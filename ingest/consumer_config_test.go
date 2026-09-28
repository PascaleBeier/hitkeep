package ingest

import (
	"testing"
	"time"
)

func TestIngestConsumerConfigTunesDeliveryForBatching(t *testing.T) {
	cfg := newIngestConsumerConfig()

	if cfg.MaxInFlight != ingestConsumerConcurrency {
		t.Fatalf("expected MaxInFlight %d, got %d", ingestConsumerConcurrency, cfg.MaxInFlight)
	}
	// Handlers hold a message for at most one batch flush (200ms) plus the
	// 10s persist timeout; 30s redelivers wedged messages twice as fast as
	// the 60s default without risking premature requeues.
	if cfg.MsgTimeout != 30*time.Second {
		t.Fatalf("expected MsgTimeout 30s, got %s", cfg.MsgTimeout)
	}
	if cfg.MaxAttempts != 5 {
		t.Fatalf("expected MaxAttempts 5, got %d", cfg.MaxAttempts)
	}
	// Transient persist failures should retry promptly, not after the 90s
	// go-nsq default.
	if cfg.DefaultRequeueDelay != 5*time.Second {
		t.Fatalf("expected DefaultRequeueDelay 5s, got %s", cfg.DefaultRequeueDelay)
	}
}
