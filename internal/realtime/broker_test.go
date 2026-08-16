package realtime

import (
	"testing"
	"testing/synctest"

	"github.com/google/uuid"
)

func TestBrokerPublishesOnlyToSiteSubscribers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		broker := NewBroker()
		siteID := uuid.New()
		otherSiteID := uuid.New()

		sub, _, missed := broker.Subscribe(siteID, "")
		if missed {
			t.Fatal("new subscription should not report a missed replay")
		}
		defer sub.Close()
		other, _, _ := broker.Subscribe(otherSiteID, "")
		defer other.Close()

		go broker.Publish(Event{SiteID: siteID, Kinds: []string{KindHits}, Counts: map[string]int{KindHits: 1}})
		synctest.Wait()

		event := <-sub.Events()
		if event.Name != EventAnalyticsChanged {
			t.Fatalf("expected analytics changed event, got %q", event.Name)
		}
		if event.SiteID != siteID {
			t.Fatalf("expected site %s, got %s", siteID, event.SiteID)
		}

		select {
		case event := <-other.Events():
			t.Fatalf("other site subscriber received unexpected event %+v", event)
		default:
		}
	})
}

func TestBrokerReplaysEventsAfterLastEventID(t *testing.T) {
	broker := NewBroker()
	siteID := uuid.New()

	broker.Publish(Event{SiteID: siteID, Kinds: []string{KindHits}, Counts: map[string]int{KindHits: 1}})
	broker.Publish(Event{SiteID: siteID, Kinds: []string{KindEvents}, Counts: map[string]int{KindEvents: 1}})

	sub, replay, missed := broker.Subscribe(siteID, "1")
	defer sub.Close()
	if missed {
		t.Fatal("expected buffered replay, got missed replay")
	}
	if len(replay) != 1 {
		t.Fatalf("expected one replay event, got %d", len(replay))
	}
	if replay[0].ID != 2 || replay[0].Kinds[0] != KindEvents {
		t.Fatalf("unexpected replay event %+v", replay[0])
	}
}

func TestBrokerReportsMissedReplayWhenHistoryExpired(t *testing.T) {
	broker := NewBroker()
	broker.historyLimit = 1
	siteID := uuid.New()

	broker.Publish(Event{SiteID: siteID, Kinds: []string{KindHits}})
	broker.Publish(Event{SiteID: siteID, Kinds: []string{KindEvents}})

	sub, replay, missed := broker.Subscribe(siteID, "1")
	defer sub.Close()
	if !missed {
		t.Fatal("expected missed replay when last event is older than history")
	}
	if len(replay) != 0 {
		t.Fatalf("expected no replay after missed history, got %d", len(replay))
	}
}

func TestSubscriptionCloseRemovesSubscriber(t *testing.T) {
	broker := NewBroker()
	siteID := uuid.New()

	sub, _, _ := broker.Subscribe(siteID, "")
	if got := broker.SubscriberCount(siteID); got != 1 {
		t.Fatalf("expected one subscriber, got %d", got)
	}
	sub.Close()
	if got := broker.SubscriberCount(siteID); got != 0 {
		t.Fatalf("expected no subscribers after close, got %d", got)
	}
}

func TestSlowSubscriberReceivesResync(t *testing.T) {
	broker := NewBroker()
	broker.bufferSize = 1
	siteID := uuid.New()

	sub, _, _ := broker.Subscribe(siteID, "")
	defer sub.Close()

	broker.Publish(Event{SiteID: siteID, Kinds: []string{KindHits}})
	broker.Publish(Event{SiteID: siteID, Kinds: []string{KindEvents}})

	event := <-sub.Events()
	if event.Name != EventAnalyticsResync {
		t.Fatalf("expected resync event for slow subscriber, got %q", event.Name)
	}
}
