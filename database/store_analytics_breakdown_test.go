package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
)

func TestSiteStatsSectionsAndAgentBreakdowns(t *testing.T) {
	ctx := context.Background()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	userID, err := store.CreateUser(ctx, "sections@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, userID, "sections.example")
	if err != nil {
		t.Fatal(err)
	}
	safari := "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"
	gptBot := "Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; GPTBot/1.2; +https://openai.com/gptbot)"
	at := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	for i, agent := range []string{safari, safari, safari, gptBot} {
		if err := store.CreateHit(ctx, &api.Hit{SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: at.Add(time.Duration(i) * time.Minute), Path: "/", UserAgent: &agent}); err != nil {
			t.Fatal(err)
		}
	}
	params := api.AnalyticsParams{SiteID: site.ID, Start: at.Add(-time.Hour), End: at.Add(time.Hour)}

	kpis, err := store.GetSiteStatsSections(ctx, params, SiteStatsSections{})
	if err != nil {
		t.Fatal(err)
	}
	if kpis.TotalPageviews != 4 || len(kpis.TopPages) != 0 || len(kpis.TopLandingPages) != 0 || len(kpis.ChartData) != 0 {
		t.Fatalf("KPI-only stats = %+v", kpis)
	}
	full, err := store.GetSiteStats(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if full.TotalPageviews != 4 || len(full.TopPages) != 1 || len(full.TopBrowsers) == 0 || full.AIBotHits != 1 {
		t.Fatalf("full stats = %+v", full)
	}

	bots, err := store.GetDimensionBreakdown(ctx, params, "ai_bot", 10)
	if err != nil {
		t.Fatal(err)
	}
	// Non-agent traffic has no AI agent name and is left out.
	if len(bots) != 1 || bots[0].Pageviews != 1 || bots[0].Name != full.TopAIBots[0].Name {
		t.Fatalf("ai_bot breakdown = %+v, overview top AI bots = %+v", bots, full.TopAIBots)
	}
	browsers, err := store.GetDimensionBreakdown(ctx, params, "browser", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(browsers) != 1 || browsers[0].Pageviews != 3 || browsers[0].Visitors != 3 {
		t.Fatalf("browser breakdown = %+v", browsers)
	}
}
