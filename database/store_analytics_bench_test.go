package database

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
)

// benchmarkSiteStats is a month of traffic for one busy site: 200k hits in
// multi-page sessions across 60 paths, referrers, countries, and agents.
func benchmarkSiteStats(b *testing.B) (*Store, api.AnalyticsParams) {
	b.Helper()
	ctx := context.Background()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		b.Fatal(err)
	}
	userID, err := store.CreateUser(ctx, "bench@example.com", "hash")
	if err != nil {
		b.Fatal(err)
	}
	site, err := store.CreateSite(ctx, userID, "bench.example")
	if err != nil {
		b.Fatal(err)
	}
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	rng := rand.New(rand.NewPCG(1, 2))
	referrers := []string{"", "", "https://www.google.com/", "https://news.ycombinator.com/item?id=1", "https://chatgpt.com/", "https://t.co/x"}
	agents := []string{
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0 Safari/537.36",
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; GPTBot/1.2; +https://openai.com/gptbot)",
	}
	countries := []string{"DE", "US", "FR", "GB", "NL"}
	hits := make([]*api.Hit, 0, 200_000)
	for len(hits) < cap(hits) {
		session, referrer, country := uuid.New(), referrers[rng.IntN(len(referrers))], countries[rng.IntN(len(countries))]
		// 2,000 distinct agents: real traffic repeats agents, but not three.
		agent := fmt.Sprintf("%s build/%d", agents[rng.IntN(len(agents))], rng.IntN(2000))
		at := start.Add(time.Duration(rng.IntN(29*24*3600)) * time.Second)
		for page := range 1 + rng.IntN(5) {
			width := 390 + rng.IntN(3)*700
			hit := &api.Hit{
				SiteID: site.ID, SessionID: session, PageID: uuid.New(), Timestamp: at.Add(time.Duration(page) * time.Minute),
				Path: fmt.Sprintf("/page-%d", rng.IntN(60)), UserAgent: &agent, CountryCode: &country, ViewportWidth: &width,
			}
			if page == 0 && referrer != "" {
				hit.Referrer = &referrer
			}
			hits = append(hits, hit)
		}
	}
	for chunk := range slices.Chunk(hits, 10_000) {
		if err := store.CreateHitsBulk(ctx, chunk); err != nil {
			b.Fatal(err)
		}
	}
	return store, api.AnalyticsParams{SiteID: site.ID, Start: start, End: start.AddDate(0, 0, 29)}
}

func BenchmarkGetSiteStats(b *testing.B) {
	store, params := benchmarkSiteStats(b)
	ctx := context.Background()
	for name, sections := range map[string]SiteStatsSections{
		"all":        AllSiteStatsSections,
		"kpis":       {},
		"chart":      {Chart: true},
		"top_lists":  {TopLists: true},
		"entry_exit": {EntryExit: true},
		"goals":      {Goals: true},
	} {
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				if _, err := store.GetSiteStatsSections(ctx, params, sections); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkComparisonAndBreakdown(b *testing.B) {
	store, params := benchmarkSiteStats(b)
	ctx := context.Background()
	b.Run("comparison", func(b *testing.B) {
		compare := params
		compare.CompareStart, compare.CompareEnd = params.Start.AddDate(0, 0, -29), params.Start
		for b.Loop() {
			if _, err := store.GetComparisonStats(ctx, compare); err != nil {
				b.Fatal(err)
			}
		}
	})
	for _, dimension := range []string{"page", "browser", "ai_bot"} {
		b.Run("breakdown_"+dimension, func(b *testing.B) {
			for b.Loop() {
				if _, err := store.GetDimensionBreakdown(ctx, params, dimension, 50); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
