package webhookdispatcher

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hitkeep/internal/api"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/webhooks"
)

func TestDispatcherSignsExactBodyAndMarksTwoHundredSuccess(t *testing.T) {
	var receivedBody []byte
	var timestamp, signature, eventHeader, deliveryHeader string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		timestamp = r.Header.Get(HeaderTimestamp)
		signature = r.Header.Get(HeaderSignature)
		eventHeader = r.Header.Get(HeaderEventID)
		deliveryHeader = r.Header.Get(HeaderDeliveryID)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()

	store, _, site := setupDispatcherStore(t)
	defer store.Close()
	webhook, secret, err := store.CreateWebhook(context.Background(), &site.ID, api.WebhookInput{
		Name: "Signed receiver", URL: receiver.URL, Enabled: true, Events: []string{webhooks.EventGoalCreated},
	})
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	jobs, err := store.EnqueueWebhookEvent(context.Background(), database.WebhookEventInput{
		SiteID: &site.ID, EventType: webhooks.EventGoalCreated, APIVersion: "2.10", Data: map[string]any{"site_id": site.ID.String()},
	})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("enqueue delivery: jobs=%+v err=%v", jobs, err)
	}

	dispatcher := NewDispatcher(store, config.Config{
		WebhookAllowDevelopmentTargets: true,
		WebhookDeliveryTimeoutSeconds:  2,
		WebhookMaxAttempts:             3,
	})
	if err := dispatcher.Dispatch(context.Background(), jobs[0].DeliveryID); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	delivery, err := store.GetWebhookDelivery(context.Background(), jobs[0].DeliveryID)
	if err != nil {
		t.Fatalf("get delivery: %v", err)
	}
	if delivery == nil || delivery.Status != database.WebhookDeliverySucceeded || delivery.AttemptCount != 1 || delivery.WebhookID != webhook.ID {
		t.Fatalf("unexpected successful delivery: %+v", delivery)
	}
	if string(receivedBody) != string(jobs[0].Payload) {
		t.Fatalf("dispatcher changed signed payload\nreceived=%s\nstored=%s", receivedBody, jobs[0].Payload)
	}
	if eventHeader != jobs[0].EventID.String() || deliveryHeader != jobs[0].DeliveryID.String() {
		t.Fatalf("unexpected stable ID headers event=%q delivery=%q", eventHeader, deliveryHeader)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(receivedBody)
	wantSignature := "v1=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(signature), []byte(wantSignature)) {
		t.Fatalf("signature=%q want=%q", signature, wantSignature)
	}
}

func TestDispatcherDoesNotFollowRedirectAndSchedulesBoundedRetry(t *testing.T) {
	redirectTargetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectTargetCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()

	store, _, site := setupDispatcherStore(t)
	defer store.Close()
	if _, _, err := store.CreateWebhook(context.Background(), &site.ID, api.WebhookInput{
		Name: "Redirector", URL: redirector.URL, Enabled: true, Events: []string{webhooks.EventImportFailed},
	}); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	jobs, err := store.EnqueueWebhookEvent(context.Background(), database.WebhookEventInput{
		SiteID: &site.ID, EventType: webhooks.EventImportFailed, APIVersion: "2.10", Data: map[string]any{"site_id": site.ID.String()},
	})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("enqueue delivery: jobs=%+v err=%v", jobs, err)
	}

	dispatcher := NewDispatcher(store, config.Config{
		WebhookAllowDevelopmentTargets: true,
		WebhookDeliveryTimeoutSeconds:  2,
		WebhookMaxAttempts:             3,
	})
	before := time.Now().UTC()
	if err := dispatcher.Dispatch(context.Background(), jobs[0].DeliveryID); err != nil {
		t.Fatalf("dispatch redirect: %v", err)
	}
	if redirectTargetCalls != 0 {
		t.Fatalf("dispatcher followed redirect %d time(s)", redirectTargetCalls)
	}
	delivery, err := store.GetWebhookDelivery(context.Background(), jobs[0].DeliveryID)
	if err != nil {
		t.Fatalf("get delivery: %v", err)
	}
	if delivery == nil || delivery.Status != database.WebhookDeliveryRetrying || delivery.ResponseStatus != http.StatusFound || delivery.NextAttemptAt == nil {
		t.Fatalf("unexpected retrying delivery: %+v", delivery)
	}
	if delivery.NextAttemptAt.Before(before.Add(25*time.Second)) || delivery.NextAttemptAt.After(before.Add(35*time.Second)) {
		t.Fatalf("unexpected first retry time: %s", delivery.NextAttemptAt)
	}
}

func TestDispatcherEventuallyFailsWithoutDisablingWebhook(t *testing.T) {
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusServiceUnavailable)
	}))
	defer receiver.Close()

	store, _, site := setupDispatcherStore(t)
	defer store.Close()
	webhook, _, err := store.CreateWebhook(context.Background(), &site.ID, api.WebhookInput{
		Name: "Failing", URL: receiver.URL, Enabled: true, Events: []string{webhooks.EventImportFailed},
	})
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	jobs, _ := store.EnqueueWebhookEvent(context.Background(), database.WebhookEventInput{
		SiteID: &site.ID, EventType: webhooks.EventImportFailed, APIVersion: "2.10", Data: map[string]any{},
	})
	dispatcher := NewDispatcher(store, config.Config{
		WebhookAllowDevelopmentTargets: true,
		WebhookDeliveryTimeoutSeconds:  2,
		WebhookMaxAttempts:             1,
	})
	if err := dispatcher.Dispatch(context.Background(), jobs[0].DeliveryID); err != nil {
		t.Fatalf("dispatch failure: %v", err)
	}
	delivery, _ := store.GetWebhookDelivery(context.Background(), jobs[0].DeliveryID)
	if delivery == nil || delivery.Status != database.WebhookDeliveryFailed || delivery.NextAttemptAt != nil {
		t.Fatalf("unexpected failed delivery: %+v", delivery)
	}
	configured, err := store.GetWebhook(context.Background(), webhook.ID, &site.ID)
	if err != nil || configured == nil || !configured.Enabled {
		t.Fatalf("delivery failure must not disable webhook: webhook=%+v err=%v", configured, err)
	}
}

func TestDispatcherDoesNotPersistDestinationOrTransportDetails(t *testing.T) {
	receiver := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	destination := receiver.URL + "/hook?token=must-not-appear"
	receiver.Close()

	store, _, site := setupDispatcherStore(t)
	defer store.Close()
	if _, _, err := store.CreateWebhook(context.Background(), &site.ID, api.WebhookInput{
		Name: "Unavailable", URL: destination, Enabled: true, Events: []string{webhooks.EventWebhookTest},
	}); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	configured, err := store.ListWebhooks(context.Background(), &site.ID)
	if err != nil || len(configured) != 1 {
		t.Fatalf("list webhook: %+v err=%v", configured, err)
	}
	jobs, err := store.EnqueueWebhookEvent(context.Background(), database.WebhookEventInput{
		SiteID: &site.ID, TargetWebhookID: &configured[0].ID, EventType: webhooks.EventWebhookTest, APIVersion: "2.10", Data: map[string]any{},
	})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("enqueue: %+v err=%v", jobs, err)
	}
	dispatcher := NewDispatcher(store, config.Config{WebhookAllowDevelopmentTargets: true, WebhookDeliveryTimeoutSeconds: 1})
	if err := dispatcher.Dispatch(context.Background(), jobs[0].DeliveryID); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	delivery, err := store.GetWebhookDelivery(context.Background(), jobs[0].DeliveryID)
	if err != nil || delivery == nil {
		t.Fatalf("get delivery: %+v err=%v", delivery, err)
	}
	if delivery.LastErrorMessage != "delivery request failed" || strings.Contains(delivery.LastErrorMessage, "token=") || strings.Contains(delivery.LastErrorMessage, receiver.URL) {
		t.Fatalf("unsafe delivery diagnostic %q", delivery.LastErrorMessage)
	}
}

func TestRetryDelayIsExponentialAndBounded(t *testing.T) {
	t.Parallel()
	wants := map[int]time.Duration{
		1:  30 * time.Second,
		2:  time.Minute,
		3:  2 * time.Minute,
		8:  64 * time.Minute,
		20: 6 * time.Hour,
	}
	for attempt, want := range wants {
		if got := RetryDelay(attempt); got != want {
			t.Errorf("RetryDelay(%d)=%s want %s", attempt, got, want)
		}
	}
}

func TestDispatcherRetryDelayUsesConfiguredBounds(t *testing.T) {
	dispatcher := NewDispatcher(nil, config.Config{WebhookRetryBaseSeconds: 2, WebhookRetryMaxSeconds: 5})
	for attempt, want := range map[int]time.Duration{1: 2 * time.Second, 2: 4 * time.Second, 3: 5 * time.Second, 20: 5 * time.Second} {
		if got := dispatcher.retryDelay(attempt); got != want {
			t.Errorf("retryDelay(%d)=%s want %s", attempt, got, want)
		}
	}
}
