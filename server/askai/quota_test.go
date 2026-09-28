package askai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/entitlements"
)

type quotaTestProvider string

func (p quotaTestProvider) ForTenant(context.Context, uuid.UUID) (*entitlements.Entitlements, error) {
	limit := 1
	switch p {
	case "pro":
		limit = 100
	case "business":
		limit = 500
	}
	return &entitlements.Entitlements{MaxAskAIAnswersPerDay: limit}, nil
}

func (p quotaTestProvider) DescribeTenant(context.Context, uuid.UUID) (*entitlements.PlanInfo, error) {
	return &entitlements.PlanInfo{Code: string(p)}, nil
}

func requestAskAIStatus(t *testing.T, mux *http.ServeMux, siteID uuid.UUID, cookie *http.Cookie) (int, api.AskAIStatus) {
	t.Helper()
	req := newAskAIHTTPTestRequest(t, http.MethodGet, "/api/sites/"+siteID.String()+"/ask-ai/status", cookie, "")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var status api.AskAIStatus
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
			t.Fatalf("decode Ask AI status: %v", err)
		}
	}
	return rec.Code, status
}

func TestAskAICloudDailyStatusPlans(t *testing.T) {
	for _, tc := range []struct {
		plan  string
		limit int
	}{{"free", 1}, {"pro", 100}, {"business", 500}} {
		t.Run(tc.plan, func(t *testing.T) {
			cfg := askAITestConfig()
			cfg.CloudHosted = true
			mux, _, siteID, userID, _ := setupAskAIHandlerTestEnv(t, cfg, quotaTestProvider(tc.plan))
			code, status := requestAskAIStatus(t, mux, siteID, askAISessionCookie(t, userID))
			if code != http.StatusOK || status.DailyLimit == nil || *status.DailyLimit != tc.limit || status.DailyUsed == nil || *status.DailyUsed != 0 || status.DailyRemaining == nil || *status.DailyRemaining != tc.limit {
				t.Fatalf("plan %s status: code=%d status=%+v", tc.plan, code, status)
			}
			if status.DailyResetAt == nil || !status.DailyResetAt.Equal(time.Now().UTC().Truncate(24*time.Hour).Add(24*time.Hour)) {
				t.Fatalf("expected next UTC midnight reset, got %+v", status.DailyResetAt)
			}
		})
	}
}

func TestAskAICloudDailyQuotaSharedAcrossTeamSites(t *testing.T) {
	cfg := askAITestConfig()
	cfg.CloudHosted = true
	mux, store, siteID, userID, ai := setupAskAIHandlerTestEnv(t, cfg, quotaTestProvider("free"))
	cookie := askAISessionCookie(t, userID)
	if got := requestAskAI(t, mux, siteID, cookie, "").Code; got != http.StatusOK {
		t.Fatalf("first answer: %d", got)
	}
	teamID := requireSiteTeamID(t, store, siteID)
	start := time.Now().UTC().Truncate(24 * time.Hour)
	for _, check := range []struct {
		name string
		team uuid.UUID
		from time.Time
		to   time.Time
		want int
	}{
		{"current UTC day", teamID, start, start.Add(24 * time.Hour), 1},
		{"next UTC day", teamID, start.Add(24 * time.Hour), start.Add(48 * time.Hour), 0},
		{"different team", uuid.New(), start, start.Add(24 * time.Hour), 0},
	} {
		used, err := store.CountSuccessfulAskAIResponses(context.Background(), check.team, check.from, check.to)
		if err != nil || used != check.want {
			t.Fatalf("%s count: used=%d want=%d err=%v", check.name, used, check.want, err)
		}
	}
	secondSite, err := store.CreateSite(context.Background(), userID, "ask-ai-second.example.test")
	if err != nil {
		t.Fatal(err)
	}
	firstTeam := requireSiteTeamID(t, store, siteID)
	if secondTeam := requireSiteTeamID(t, store, secondSite.ID); secondTeam != firstTeam {
		t.Fatalf("sites should share a team: %s vs %s", firstTeam, secondTeam)
	}
	rec := requestAskAI(t, mux, secondSite.ID, cookie, "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second site answer: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var status api.AskAIStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Status != "daily_limit_exhausted" || status.DailyUsed == nil || *status.DailyUsed != 1 || status.DailyRemaining == nil || *status.DailyRemaining != 0 {
		t.Fatalf("unexpected exhausted status: %+v", status)
	}
	ai.mu.Lock()
	defer ai.mu.Unlock()
	if ai.calls != 1 {
		t.Fatalf("provider called %d times, want 1", ai.calls)
	}
}

func TestAskAICloudDailyQuotaIgnoresFailure(t *testing.T) {
	cfg := askAITestConfig()
	cfg.CloudHosted = true
	mux, _, siteID, userID, ai := setupAskAIHandlerTestEnv(t, cfg, quotaTestProvider("free"))
	cookie := askAISessionCookie(t, userID)
	ai.err = errors.New("provider failed")
	if got := requestAskAI(t, mux, siteID, cookie, "").Code; got != http.StatusBadGateway {
		t.Fatalf("failed answer: %d", got)
	}
	ai.err = nil
	if got := requestAskAI(t, mux, siteID, cookie, "").Code; got != http.StatusOK {
		t.Fatalf("answer after failure: %d", got)
	}
	if got := requestAskAI(t, mux, siteID, cookie, "").Code; got != http.StatusTooManyRequests {
		t.Fatalf("answer after success: %d", got)
	}
}

func TestAskAICloudDailyQuotaSerializesConcurrentTeamRequests(t *testing.T) {
	cfg := askAITestConfig()
	cfg.CloudHosted = true
	mux, _, siteID, userID, ai := setupAskAIHandlerTestEnv(t, cfg, quotaTestProvider("free"))
	cookie := askAISessionCookie(t, userID)
	block := make(chan struct{})
	ai.block = block
	ai.called = make(chan struct{})
	firstReq := newAskAIHTTPTestRequest(t, http.MethodPost, "/api/sites/"+siteID.String()+"/ask-ai", cookie, "")
	firstRec := httptest.NewRecorder()
	var wg sync.WaitGroup
	wg.Go(func() { mux.ServeHTTP(firstRec, firstReq) })
	select {
	case <-ai.called:
	case <-time.After(5 * time.Second):
		close(block)
		wg.Wait()
		t.Fatal("first provider call did not start")
	}
	secondReq := newAskAIHTTPTestRequest(t, http.MethodPost, "/api/sites/"+siteID.String()+"/ask-ai", cookie, "")
	secondRec := httptest.NewRecorder()
	wg.Go(func() { mux.ServeHTTP(secondRec, secondReq) })
	time.Sleep(20 * time.Millisecond)
	ai.mu.Lock()
	callsWhileBlocked := ai.calls
	ai.mu.Unlock()
	if callsWhileBlocked != 1 {
		close(block)
		wg.Wait()
		t.Fatalf("%d provider calls while first answer in flight", callsWhileBlocked)
	}
	close(block)
	wg.Wait()
	if firstRec.Code != http.StatusOK || secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("parallel results: first=%d second=%d", firstRec.Code, secondRec.Code)
	}
}

func TestAskAICloudDailyQuotaIgnoresCancellationBeforeCompletion(t *testing.T) {
	for _, route := range []string{"/ask-ai", "/ask-ai/events"} {
		t.Run(route, func(t *testing.T) {
			cfg := askAITestConfig()
			cfg.CloudHosted = true
			mux, store, siteID, userID, ai := setupAskAIHandlerTestEnv(t, cfg, quotaTestProvider("free"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ai.cancelBeforeReturn = cancel
			req := newAskAIHTTPTestRequest(t, http.MethodPost, "/api/sites/"+siteID.String()+route, askAISessionCookie(t, userID), "").WithContext(ctx)
			mux.ServeHTTP(httptest.NewRecorder(), req)
			teamID := requireSiteTeamID(t, store, siteID)
			start := time.Now().UTC().Truncate(24 * time.Hour)
			used, err := store.CountSuccessfulAskAIResponses(context.Background(), teamID, start, start.Add(24*time.Hour))
			if err != nil || used != 0 {
				t.Fatalf("canceled request counted: used=%d err=%v", used, err)
			}
		})
	}
}

func TestAskAICloudDailyQuotaCountsCompletedAnswerAfterStreamDeliveryFailure(t *testing.T) {
	cfg := askAITestConfig()
	cfg.CloudHosted = true
	mux, store, siteID, userID, _ := setupAskAIHandlerTestEnv(t, cfg, quotaTestProvider("free"))
	req := newAskAIHTTPTestRequest(t, http.MethodPost, "/api/sites/"+siteID.String()+"/ask-ai/events", askAISessionCookie(t, userID), "")
	mux.ServeHTTP(&eventuallyFailingAskAIStreamResponseWriter{header: http.Header{}, failOnWrite: 3}, req)
	teamID := requireSiteTeamID(t, store, siteID)
	start := time.Now().UTC().Truncate(24 * time.Hour)
	used, err := store.CountSuccessfulAskAIResponses(context.Background(), teamID, start, start.Add(24*time.Hour))
	if err != nil || used != 1 {
		t.Fatalf("completed answer should count after final delivery fails: used=%d err=%v", used, err)
	}
}

func TestAskAISelfHostedExcludesCloudDailyQuota(t *testing.T) {
	cfg := askAITestConfig()
	mux, _, siteID, userID, ai := setupAskAIHandlerTestEnv(t, cfg, quotaTestProvider("free"))
	cookie := askAISessionCookie(t, userID)
	for range 2 {
		if got := requestAskAI(t, mux, siteID, cookie, "").Code; got != http.StatusOK {
			t.Fatalf("self-hosted answer status: %d", got)
		}
	}
	code, status := requestAskAIStatus(t, mux, siteID, cookie)
	if code != http.StatusOK || !status.Available || status.DailyLimit != nil || status.DailyUsed != nil || status.DailyRemaining != nil || status.DailyResetAt != nil {
		t.Fatalf("self-hosted status should omit cloud allowance: code=%d status=%+v", code, status)
	}
	if ai.calls != 2 {
		t.Fatalf("provider calls: got %d, want 2", ai.calls)
	}
}
