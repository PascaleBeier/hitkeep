package realtime

import (
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	EventAnalyticsChanged = "analytics.changed"
	EventAnalyticsResync  = "analytics.resync"

	KindHits          = "hits"
	KindEvents        = "events"
	KindEcommerce     = "ecommerce"
	KindWebVitals     = "web_vitals"
	KindAIFetch       = "ai_fetch"
	KindImports       = "imports"
	KindGoals         = "goals"
	KindFunnels       = "funnels"
	KindOpportunities = "opportunities"

	defaultHistoryLimit = 256
	defaultBufferSize   = 16
)

type Event struct {
	ID          uint64         `json:"-"`
	Name        string         `json:"-"`
	SiteID      uuid.UUID      `json:"site_id"`
	Kinds       []string       `json:"kinds"`
	ChangedAt   time.Time      `json:"changed_at"`
	BucketStart time.Time      `json:"bucket_start"`
	Counts      map[string]int `json:"counts"`
}

type Broker struct {
	mu           sync.Mutex
	closed       bool
	nextID       uint64
	historyLimit int
	bufferSize   int
	history      map[uuid.UUID][]Event
	subscribers  map[uuid.UUID]map[*Subscription]struct{}
}

type Subscription struct {
	siteID uuid.UUID
	ch     chan Event
	broker *Broker
	closed bool
}

func NewBroker() *Broker {
	return &Broker{
		historyLimit: defaultHistoryLimit,
		bufferSize:   defaultBufferSize,
		history:      map[uuid.UUID][]Event{},
		subscribers:  map[uuid.UUID]map[*Subscription]struct{}{},
	}
}

func (b *Broker) Close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for siteID, subs := range b.subscribers {
		for sub := range subs {
			sub.closed = true
			close(sub.ch)
		}
		delete(b.subscribers, siteID)
		delete(b.history, siteID)
	}
}

func (b *Broker) Subscribe(siteID uuid.UUID, lastEventID string) (*Subscription, []Event, bool) {
	if b == nil {
		return nil, nil, false
	}

	lastID, hasLastID := parseEventID(lastEventID)

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, nil, false
	}

	sub := &Subscription{
		siteID: siteID,
		ch:     make(chan Event, b.bufferSize),
		broker: b,
	}
	if b.subscribers[siteID] == nil {
		b.subscribers[siteID] = map[*Subscription]struct{}{}
	}
	b.subscribers[siteID][sub] = struct{}{}

	if lastEventID != "" && !hasLastID {
		return sub, nil, true
	}
	if !hasLastID {
		return sub, nil, false
	}

	history := b.history[siteID]
	if len(history) == 0 {
		return sub, nil, true
	}
	oldest := history[0].ID
	newest := history[len(history)-1].ID
	if lastID < oldest || lastID > newest {
		return sub, nil, true
	}

	replay := make([]Event, 0, len(history))
	for _, event := range history {
		if event.ID > lastID {
			replay = append(replay, event)
		}
	}
	return sub, replay, false
}

func (b *Broker) Publish(event Event) {
	if b == nil || event.SiteID == uuid.Nil {
		return
	}
	if event.Name == "" {
		event.Name = EventAnalyticsChanged
	}
	if event.ChangedAt.IsZero() {
		event.ChangedAt = time.Now().UTC()
	}
	if event.BucketStart.IsZero() {
		event.BucketStart = event.ChangedAt.Truncate(time.Minute)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || len(b.subscribers[event.SiteID]) == 0 {
		return
	}
	b.nextID++
	event.ID = b.nextID
	b.appendHistoryLocked(event)
	for sub := range b.subscribers[event.SiteID] {
		sub.enqueue(event)
	}
}

func (b *Broker) SubscriberCount(siteID uuid.UUID) int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subscribers[siteID])
}

func (b *Broker) appendHistoryLocked(event Event) {
	history := append(b.history[event.SiteID], event)
	if len(history) > b.historyLimit {
		history = history[len(history)-b.historyLimit:]
	}
	b.history[event.SiteID] = history
}

func (s *Subscription) Events() <-chan Event {
	if s == nil {
		return nil
	}
	return s.ch
}

func (s *Subscription) Close() {
	if s == nil || s.broker == nil {
		return
	}
	s.broker.mu.Lock()
	defer s.broker.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if subs := s.broker.subscribers[s.siteID]; subs != nil {
		delete(subs, s)
		if len(subs) == 0 {
			delete(s.broker.subscribers, s.siteID)
			delete(s.broker.history, s.siteID)
		}
	}
	close(s.ch)
}

func (s *Subscription) enqueue(event Event) {
	select {
	case s.ch <- event:
		return
	default:
	}

	resync := Event{
		ID:          event.ID,
		Name:        EventAnalyticsResync,
		SiteID:      event.SiteID,
		Kinds:       event.Kinds,
		ChangedAt:   event.ChangedAt,
		BucketStart: event.BucketStart,
		Counts:      event.Counts,
	}

	select {
	case <-s.ch:
	default:
	}
	select {
	case s.ch <- resync:
	default:
	}
}

func parseEventID(raw string) (uint64, bool) {
	if raw == "" {
		return 0, false
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}
