package blocking

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/database"
)

func setupFilterStore(t *testing.T) (*database.Store, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()

	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	userID, err := store.CreateUser(context.Background(), "owner@example.com", "hashed-secret")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	site, err := store.CreateSite(context.Background(), userID, "filter.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	otherSite, err := store.CreateSite(context.Background(), userID, "filter-2.example")
	if err != nil {
		t.Fatalf("create second site: %v", err)
	}

	return store, userID, site.ID, otherSite.ID
}

func TestIPFilterIsBlocked(t *testing.T) {
	store, userID, siteID, otherSiteID := setupFilterStore(t)
	defer store.Close()

	ctx := context.Background()

	if _, err := store.CreateInstanceExclusion(ctx, "203.0.113.5/32", "global monitor", userID); err != nil {
		t.Fatalf("create instance exclusion: %v", err)
	}
	if _, err := store.CreateSiteExclusion(ctx, siteID, "10.0.0.0/8", "office", userID); err != nil {
		t.Fatalf("create site exclusion: %v", err)
	}

	filter := NewIPFilter(store)
	if err := filter.Refresh(ctx); err != nil {
		t.Fatalf("refresh filter: %v", err)
	}

	if !filter.IsBlocked(siteID, "203.0.113.5") {
		t.Fatalf("expected global blocked ip to be blocked")
	}
	if !filter.IsBlocked(siteID, "10.1.2.3") {
		t.Fatalf("expected site blocked ip to be blocked")
	}
	if filter.IsBlocked(otherSiteID, "10.1.2.3") {
		t.Fatalf("expected site-specific blocked ip to be allowed for other site")
	}
	if filter.IsBlocked(siteID, "198.51.100.1") {
		t.Fatalf("expected non-blocked ip to be allowed")
	}
	if filter.IsBlocked(siteID, "") {
		t.Fatalf("expected empty ip to be allowed")
	}
}

func TestIPFilterBlocksCountries(t *testing.T) {
	store, userID, siteID, otherSiteID := setupFilterStore(t)
	defer store.Close()

	ctx := context.Background()

	if _, err := store.CreateInstanceCountryExclusion(ctx, "DE", "global country", userID); err != nil {
		t.Fatalf("create instance country exclusion: %v", err)
	}
	if _, err := store.CreateSiteCountryExclusion(ctx, siteID, "US", "site country", userID); err != nil {
		t.Fatalf("create site country exclusion: %v", err)
	}

	filter := NewIPFilter(store)
	if err := filter.Refresh(ctx); err != nil {
		t.Fatalf("refresh filter: %v", err)
	}

	if decision := filter.Evaluate(siteID, "198.51.100.1", "de"); !decision.Blocked || decision.Reason != BlockReasonInstanceCountry {
		t.Fatalf("expected instance country block, got %#v", decision)
	}
	if decision := filter.Evaluate(siteID, "198.51.100.1", "us"); !decision.Blocked || decision.Reason != BlockReasonSiteCountry {
		t.Fatalf("expected site country block, got %#v", decision)
	}
	if decision := filter.Evaluate(otherSiteID, "198.51.100.1", "us"); decision.Blocked {
		t.Fatalf("expected site country rule to allow other site, got %#v", decision)
	}
	if decision := filter.Evaluate(siteID, "198.51.100.1", ""); decision.Blocked {
		t.Fatalf("expected empty country to be allowed, got %#v", decision)
	}
}

func TestTrafficExclusionFilterMatchesContextAcrossScopes(t *testing.T) {
	store, userID, siteID, otherSiteID := setupFilterStore(t)
	defer store.Close()
	ctx := context.Background()
	teamID, err := store.GetSiteTenantID(ctx, siteID)
	if err != nil {
		t.Fatalf("resolve team: %v", err)
	}

	if _, err := store.CreateInstanceTrafficExclusion(ctx, database.TrafficExclusionValues{Type: "user_agent", UserAgent: "HealthCheck"}, userID); err != nil {
		t.Fatalf("create instance user-agent exclusion: %v", err)
	}
	if _, err := store.CreateTeamTrafficExclusion(ctx, teamID, database.TrafficExclusionValues{Type: "path", Path: "/admin"}, userID); err != nil {
		t.Fatalf("create team path exclusion: %v", err)
	}
	if _, err := store.CreateSiteTrafficExclusion(ctx, siteID, database.TrafficExclusionValues{Type: "path", Path: "/private"}, userID); err != nil {
		t.Fatalf("create site path exclusion: %v", err)
	}

	filter := NewIPFilter(store)
	if err := filter.Refresh(ctx); err != nil {
		t.Fatalf("refresh filter: %v", err)
	}

	if decision := filter.EvaluateTraffic(siteID, TrafficExclusionContext{UserAgent: "Mozilla HEALTHCHECK/1.0"}); !decision.Blocked || decision.Reason != BlockReasonInstanceUserAgent {
		t.Fatalf("expected case-insensitive instance user-agent match, got %#v", decision)
	}
	for _, candidate := range []string{"/admin", "/admin/", "/admin/users", "/admin//users/../settings/?tab=1#top", "https://example.test/admin/users?q=1#top"} {
		if decision := filter.EvaluateTraffic(siteID, TrafficExclusionContext{Path: candidate}); !decision.Blocked || decision.Reason != BlockReasonTeamPath {
			t.Fatalf("expected team path match for %q, got %#v", candidate, decision)
		}
	}
	for _, candidate := range []string{"/administrator", "/Admin", "/admins"} {
		if decision := filter.EvaluateTraffic(otherSiteID, TrafficExclusionContext{Path: candidate}); decision.Blocked {
			t.Fatalf("expected path boundary/case negative for %q, got %#v", candidate, decision)
		}
	}
	if decision := filter.EvaluateTraffic(siteID, TrafficExclusionContext{Path: "/private/reports"}); !decision.Blocked || decision.Reason != BlockReasonSitePath {
		t.Fatalf("expected site path match, got %#v", decision)
	}
	if decision := filter.EvaluateTraffic(otherSiteID, TrafficExclusionContext{Path: "/private/reports"}); decision.Blocked {
		t.Fatalf("expected site rule not to affect other site, got %#v", decision)
	}
}

func TestTrafficExclusionFilterRefreshesNewRulesAndRootMatchesAllPaths(t *testing.T) {
	store, userID, siteID, _ := setupFilterStore(t)
	defer store.Close()
	ctx := context.Background()
	filter := NewIPFilter(store)
	if err := filter.Refresh(ctx); err != nil {
		t.Fatalf("initial refresh: %v", err)
	}
	if decision := filter.EvaluateTraffic(siteID, TrafficExclusionContext{Path: "/anything"}); decision.Blocked {
		t.Fatalf("unexpected initial block: %#v", decision)
	}
	if _, err := store.CreateSiteTrafficExclusion(ctx, siteID, database.TrafficExclusionValues{Type: "path", Path: "/"}, userID); err != nil {
		t.Fatalf("create root path rule: %v", err)
	}
	if err := filter.Refresh(ctx); err != nil {
		t.Fatalf("refresh new root rule: %v", err)
	}
	if decision := filter.EvaluateTraffic(siteID, TrafficExclusionContext{Path: "/anything?q=1"}); !decision.Blocked || decision.Reason != BlockReasonSitePath {
		t.Fatalf("expected root path to match all paths after refresh, got %#v", decision)
	}
}

func TestNormalizeExclusionPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{input: " /admin//users/../ ", want: "/admin", ok: true},
		{input: "https://example.test/docs/?q=1#top", want: "/docs", ok: true},
		{input: "admin", want: "/admin", ok: true},
		{input: "/", want: "/", ok: true},
		{input: " ?q=1 ", ok: false},
		{input: "", ok: false},
	}
	for _, test := range tests {
		got, ok := NormalizeExclusionPath(test.input)
		if ok != test.ok || got != test.want {
			t.Fatalf("NormalizeExclusionPath(%q) = %q, %v; want %q, %v", test.input, got, ok, test.want, test.ok)
		}
	}
}
