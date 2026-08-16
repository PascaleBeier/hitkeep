package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/webhookdispatcher"
	webhookcore "hitkeep/internal/webhooks"
)

func TestSiteWebhookHandlerLifecycleAndAudit(t *testing.T) {
	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	ownerID, err := store.CreateUser(context.Background(), "webhook-owner@example.test", "hash")
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	site, err := store.CreateSite(context.Background(), ownerID, "webhook.example.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	h := &handler{ctx: &shared.Context{
		Store:    store,
		Config:   &config.Config{WebhookAllowDevelopmentTargets: true},
		Webhooks: webhookdispatcher.NewEmitter(store, &failingTestProducer{}, "2.10.2", testWebhookLogger()),
	}}

	createBody := `{"name":"Operations","description":"CRM notifications","url":"http://localhost:9900/hook","enabled":true,"events":["goal.created","import.completed"]}`
	createReq := webhookRequest(http.MethodPost, "/api/sites/"+site.ID.String()+"/webhooks", site.ID, uuid.Nil, ownerID, createBody)
	createW := httptest.NewRecorder()
	h.handleCreate(&site.ID).ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%q", createW.Code, createW.Body.String())
	}
	var created api.WebhookSecretResponse
	if err := json.NewDecoder(createW.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Webhook.ID == uuid.Nil || !strings.HasPrefix(created.Secret, "whsec_") {
		t.Fatalf("unexpected create response: %+v", created)
	}

	listReq := webhookRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/webhooks", site.ID, uuid.Nil, ownerID, "")
	listW := httptest.NewRecorder()
	h.handleList(&site.ID).ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK || !strings.Contains(listW.Body.String(), "Operations") {
		t.Fatalf("unexpected list response status=%d body=%q", listW.Code, listW.Body.String())
	}

	updateBody := `{"name":"Operations v2","description":"","url":"http://localhost:9901/hook","enabled":true,"events":["goal.deleted"]}`
	updateReq := webhookRequest(http.MethodPut, "/api/sites/"+site.ID.String()+"/webhooks/"+created.Webhook.ID.String(), site.ID, created.Webhook.ID, ownerID, updateBody)
	updateW := httptest.NewRecorder()
	h.handleUpdate(&site.ID).ServeHTTP(updateW, updateReq)
	if updateW.Code != http.StatusOK || !strings.Contains(updateW.Body.String(), "Operations v2") || !strings.Contains(updateW.Body.String(), `"enabled":true`) {
		t.Fatalf("unexpected update response status=%d body=%q", updateW.Code, updateW.Body.String())
	}

	rotateReq := webhookRequest(http.MethodPost, "/api/sites/"+site.ID.String()+"/webhooks/"+created.Webhook.ID.String()+"/rotate", site.ID, created.Webhook.ID, ownerID, "")
	rotateW := httptest.NewRecorder()
	h.handleRotate(&site.ID).ServeHTTP(rotateW, rotateReq)
	if rotateW.Code != http.StatusOK {
		t.Fatalf("rotate status=%d body=%q", rotateW.Code, rotateW.Body.String())
	}
	var rotated api.WebhookSecretResponse
	if err := json.NewDecoder(rotateW.Body).Decode(&rotated); err != nil {
		t.Fatalf("decode rotate: %v", err)
	}
	if rotated.Secret == created.Secret || !strings.HasPrefix(rotated.Secret, "whsec_") {
		t.Fatalf("expected new one-time secret, got %q", rotated.Secret)
	}

	testReq := webhookRequest(http.MethodPost, "/api/sites/"+site.ID.String()+"/webhooks/"+created.Webhook.ID.String()+"/test", site.ID, created.Webhook.ID, ownerID, "")
	testW := httptest.NewRecorder()
	h.handleTest(&site.ID).ServeHTTP(testW, testReq)
	if testW.Code != http.StatusAccepted {
		t.Fatalf("test event status=%d body=%q", testW.Code, testW.Body.String())
	}
	deliveriesReq := webhookRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/webhooks/"+created.Webhook.ID.String()+"/deliveries", site.ID, created.Webhook.ID, ownerID, "")
	deliveriesW := httptest.NewRecorder()
	h.handleDeliveries(&site.ID).ServeHTTP(deliveriesW, deliveriesReq)
	if deliveriesW.Code != http.StatusOK || !strings.Contains(deliveriesW.Body.String(), webhookcore.EventWebhookTest) {
		t.Fatalf("delivery log status=%d body=%q", deliveriesW.Code, deliveriesW.Body.String())
	}

	deleteReq := webhookRequest(http.MethodDelete, "/api/sites/"+site.ID.String()+"/webhooks/"+created.Webhook.ID.String(), site.ID, created.Webhook.ID, ownerID, "")
	deleteW := httptest.NewRecorder()
	h.handleDelete(&site.ID).ServeHTTP(deleteW, deleteReq)
	if deleteW.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%q", deleteW.Code, deleteW.Body.String())
	}

	audit, _, err := store.ListInstanceAuditEntries(context.Background(), database.InstanceAuditFilter{Limit: 20})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	actions := make(map[string]bool)
	for _, entry := range audit {
		actions[entry.Action] = true
	}
	for _, action := range []string{"webhook.created", "webhook.updated", "webhook.secret_rotated", "webhook.deleted"} {
		if !actions[action] {
			t.Errorf("expected audit action %q in %+v", action, actions)
		}
	}
}

func testWebhookLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type failingTestProducer struct{}

func (*failingTestProducer) Publish(string, []byte) error { return context.DeadlineExceeded }

func TestWebhookHandlerRejectsInvalidDestinationAndEventScope(t *testing.T) {
	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	ownerID, _ := store.CreateUser(context.Background(), "invalid-webhook@example.test", "hash")
	site, _ := store.CreateSite(context.Background(), ownerID, "invalid-webhook.example.test")
	h := &handler{ctx: &shared.Context{Store: store, Config: &config.Config{}}}

	for _, body := range []string{
		`{"name":"Private","url":"https://127.0.0.1/hook","enabled":true,"events":["goal.created"]}`,
		`{"name":"Pageviews","url":"https://example.com/hook","enabled":true,"events":["pageview.created"]}`,
		`{"name":"Wrong scope","url":"https://example.com/hook","enabled":true,"events":["site.created"]}`,
	} {
		req := webhookRequest(http.MethodPost, "/api/sites/"+site.ID.String()+"/webhooks", site.ID, uuid.Nil, ownerID, body)
		w := httptest.NewRecorder()
		h.handleCreate(&site.ID).ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body=%s status=%d response=%q", body, w.Code, w.Body.String())
		}
	}
}

func webhookRequest(method, target string, siteID, webhookID, userID uuid.UUID, body string) *http.Request {
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	if siteID != uuid.Nil {
		req.SetPathValue("id", siteID.String())
	}
	if webhookID != uuid.Nil {
		req.SetPathValue("webhookID", webhookID.String())
	}
	return req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, userID))
}
