package realtime

import (
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestBrokerCloseClosesAllSubscriptionsAndRejectsNewWork(t *testing.T) {
	broker := NewBroker()
	siteA, siteB := uuid.New(), uuid.New()
	subA, _, _ := broker.Subscribe(siteA, "")
	subB, _, _ := broker.Subscribe(siteB, "")

	broker.Close()
	broker.Close()

	for _, sub := range []*Subscription{subA, subB} {
		select {
		case _, ok := <-sub.Events():
			if ok {
				t.Fatal("subscription remains open after broker close")
			}
		default:
			t.Fatal("broker close did not close subscription")
		}
		sub.Close()
	}
	if sub, _, _ := broker.Subscribe(siteA, ""); sub != nil {
		t.Fatal("closed broker accepted subscription")
	}
	broker.Publish(Event{SiteID: siteA})
	if got := broker.SubscriberCount(siteA); got != 0 {
		t.Fatalf("subscriber count after close = %d, want 0", got)
	}
}

func TestBrokerCloseRacesWithSubscribeAndPublish(t *testing.T) {
	broker := NewBroker()
	siteID := uuid.New()
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			if sub, _, _ := broker.Subscribe(siteID, ""); sub != nil {
				defer sub.Close()
			}
			broker.Publish(Event{SiteID: siteID})
			broker.Close()
		})
	}
	wg.Wait()
	if sub, _, _ := broker.Subscribe(siteID, ""); sub != nil {
		t.Fatal("closed broker accepted subscription")
	}
}

func TestBrokerReplayResyncsForInvalidAndFutureIDs(t *testing.T) {
	broker := NewBroker()
	siteID := uuid.New()
	active, _, _ := broker.Subscribe(siteID, "")
	defer active.Close()
	broker.Publish(Event{SiteID: siteID})

	for _, lastEventID := range []string{"invalid", "2"} {
		sub, replay, missed := broker.Subscribe(siteID, lastEventID)
		if sub == nil || !missed || len(replay) != 0 {
			t.Fatalf("Subscribe(%q) = sub %v, replay %d, missed %v; want resync", lastEventID, sub != nil, len(replay), missed)
		}
		sub.Close()
	}
}

func TestBrokerDoesNotRetainHistoryWithoutSubscribers(t *testing.T) {
	broker := NewBroker()
	siteID := uuid.New()
	broker.Publish(Event{SiteID: siteID})
	if len(broker.history[siteID]) != 0 {
		t.Fatal("publish without subscribers retained replay history")
	}

	sub, _, _ := broker.Subscribe(siteID, "")
	broker.Publish(Event{SiteID: siteID})
	sub.Close()
	if len(broker.history[siteID]) != 0 {
		t.Fatal("last subscription close retained replay history")
	}
	reconnect, replay, missed := broker.Subscribe(siteID, "1")
	if reconnect == nil || !missed || len(replay) != 0 {
		t.Fatalf("reconnect = sub %v, replay %d, missed %v; want resync", reconnect != nil, len(replay), missed)
	}
	reconnect.Close()
}
