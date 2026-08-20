package shared

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	json "hitkeep/internal/jsonapi"
	"hitkeep/internal/realtime"
)

const realtimeHeartbeatInterval = 15 * time.Second

func ServeRealtimeStream(w http.ResponseWriter, r *http.Request, broker *realtime.Broker, siteID uuid.UUID) {
	controller := http.NewResponseController(w)
	subscription, replay, missed := broker.Subscribe(siteID, r.Header.Get("Last-Event-ID"))
	if subscription == nil {
		http.Error(w, "Service not available", http.StatusServiceUnavailable)
		return
	}
	defer subscription.Close()

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")

	if _, err := fmt.Fprint(w, ": connected\nretry: 3000\n\n"); err != nil {
		return
	}
	if err := controller.Flush(); err != nil {
		LoggerFromContext(r.Context()).Debug("Failed to flush realtime stream prelude", "error", err, "site_id", siteID)
		return
	}

	if missed {
		now := time.Now().UTC()
		resync := realtime.Event{
			Name:        realtime.EventAnalyticsResync,
			SiteID:      siteID,
			Kinds:       []string{realtime.KindHits, realtime.KindEvents, realtime.KindEcommerce, realtime.KindWebVitals},
			ChangedAt:   now,
			BucketStart: now.Truncate(time.Minute),
			Counts:      map[string]int{},
		}
		if !writeRealtimeEvent(w, controller, r.Context(), resync) {
			return
		}
	}
	for _, event := range replay {
		if !writeRealtimeEvent(w, controller, r.Context(), event) {
			return
		}
	}

	heartbeat := time.NewTicker(realtimeHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-subscription.Events():
			if !ok {
				return
			}
			if !writeRealtimeEvent(w, controller, r.Context(), event) {
				return
			}
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		}
	}
}

func writeRealtimeEvent(w http.ResponseWriter, controller *http.ResponseController, ctx context.Context, event realtime.Event) bool {
	name := event.Name
	if name == "" {
		name = realtime.EventAnalyticsChanged
	}
	if event.ID > 0 {
		if _, err := fmt.Fprintf(w, "id: %s\n", strconv.FormatUint(event.ID, 10)); err != nil {
			return false
		}
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", name); err != nil {
		return false
	}

	data, err := json.Marshal(event)
	if err != nil {
		LoggerFromContext(ctx).Error("Failed to encode realtime event", "error", err, "site_id", event.SiteID)
		return false
	}
	if _, err := io.WriteString(w, "data: "); err != nil {
		return false
	}
	// json.Marshal escapes JSON control characters, so data cannot inject SSE fields.
	if _, err := w.Write(data); err != nil { //nolint:gosec // G705 is a false positive for encoded JSON.
		return false
	}
	if _, err := io.WriteString(w, "\n\n"); err != nil {
		return false
	}
	return controller.Flush() == nil
}
