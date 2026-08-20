package worker

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestWaitForDelayStopsWhenContextTimesOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()

		result := make(chan bool, 1)
		go func() {
			result <- waitForDelay(ctx, 10*time.Second)
		}()
		synctest.Sleep(2 * time.Second)

		if ctx.Err() == nil {
			t.Fatal("expected timeout")
		}
		if <-result {
			t.Fatal("waitForDelay returned true after context timeout")
		}
	})
}

func TestWaitForDelayCompletes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		if !waitForDelay(context.Background(), time.Second) {
			t.Fatal("waitForDelay returned false after timer fired")
		}
	})
}
