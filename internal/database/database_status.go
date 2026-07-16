package database

import (
	"strings"
	"sync"
	"time"
)

const (
	DatabaseStateHealthy        = "healthy"
	DatabaseStateRecovering     = "recovering"
	DatabaseStateNeedsAttention = "needs_attention"
	DatabaseStateFailed         = "failed"
)

// DatabaseStatus is a sanitized operational snapshot. It intentionally omits
// database paths, raw DuckDB errors, SQL, and recovery-bundle filenames.
type DatabaseStatus struct {
	State                       string     `json:"state"`
	Phase                       string     `json:"phase,omitempty"`
	Trigger                     string     `json:"trigger,omitempty"`
	RecoveryEnabled             bool       `json:"recovery_enabled"`
	AutomaticWALRecoveryEnabled bool       `json:"automatic_wal_recovery_enabled"`
	RecoveryBundleAvailable     bool       `json:"recovery_bundle_available"`
	RemovedUnsafeIndexes        int        `json:"removed_unsafe_indexes"`
	CheckpointIntervalMinutes   int        `json:"checkpoint_interval_min"`
	LastCheckpointAt            *time.Time `json:"last_checkpoint_at,omitempty"`
	LastCheckpointError         string     `json:"last_checkpoint_error,omitempty"`
	LastRecoveryAt              *time.Time `json:"last_recovery_at,omitempty"`
}

type databaseStatusTracker struct {
	mu     sync.RWMutex
	status DatabaseStatus
}

func newDatabaseStatusTracker(recoveryEnabled, automaticWALRecoveryEnabled bool, checkpointInterval time.Duration) *databaseStatusTracker {
	intervalMinutes := 0
	if checkpointInterval > 0 {
		intervalMinutes = max(1, int(checkpointInterval/time.Minute))
	}
	return &databaseStatusTracker{status: DatabaseStatus{
		State:                       DatabaseStateHealthy,
		RecoveryEnabled:             recoveryEnabled,
		AutomaticWALRecoveryEnabled: automaticWALRecoveryEnabled,
		CheckpointIntervalMinutes:   intervalMinutes,
	}}
}

func (t *databaseStatusTracker) snapshot() DatabaseStatus {
	if t == nil {
		return DatabaseStatus{State: DatabaseStateHealthy}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.status
}

func (t *databaseStatusTracker) recovering(phase, trigger string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.State = DatabaseStateRecovering
	t.status.Phase = strings.TrimSpace(phase)
	t.status.Trigger = strings.TrimSpace(trigger)
}

func (t *databaseStatusTracker) recovered(removedUnsafeIndexes int) {
	if t == nil {
		return
	}
	now := time.Now().UTC()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.State = DatabaseStateHealthy
	t.status.Phase = ""
	t.status.Trigger = ""
	t.status.LastRecoveryAt = &now
	t.status.RemovedUnsafeIndexes = removedUnsafeIndexes
}

func (t *databaseStatusTracker) healthy() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.State = DatabaseStateHealthy
	t.status.Phase = ""
	t.status.Trigger = ""
}

func (t *databaseStatusTracker) recoveryFailed(trigger, phase string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.State = DatabaseStateNeedsAttention
	t.status.Trigger = strings.TrimSpace(trigger)
	t.status.Phase = strings.TrimSpace(phase)
}

func (t *databaseStatusTracker) bundleAvailable() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.RecoveryBundleAvailable = true
}

func (t *databaseStatusTracker) restoreRecoveryHistory(bundleAvailable bool, completedAt *time.Time, removedUnsafeIndexes int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.RecoveryBundleAvailable = bundleAvailable
	if completedAt == nil {
		return
	}
	at := completedAt.UTC()
	t.status.LastRecoveryAt = &at
	t.status.RemovedUnsafeIndexes = removedUnsafeIndexes
}

func (t *databaseStatusTracker) checkpointSucceeded(at time.Time) {
	if t == nil {
		return
	}
	at = at.UTC()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.LastCheckpointAt = &at
	t.status.LastCheckpointError = ""
}

func (t *databaseStatusTracker) checkpointFailed() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.LastCheckpointError = "checkpoint_failed"
}
