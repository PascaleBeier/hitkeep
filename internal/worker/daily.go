package worker

import (
	"context"
	"log/slog"
	"time"
)

// runDailyAtUTC waits until the next hour:00 UTC, invokes run, then keeps
// invoking it every 24 hours until ctx is cancelled. Panics are logged under
// name.
func runDailyAtUTC(ctx context.Context, name string, hour int, run func(context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error(name+" panicked", "error", r)
		}
	}()

	now := time.Now().UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}

	timer := time.NewTimer(time.Until(next))
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}

	run(ctx)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run(ctx)
		}
	}
}
