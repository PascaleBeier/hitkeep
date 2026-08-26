package main

import (
	"context"
	"fmt"
	mrand "math/rand"
	"time"

	"github.com/google/uuid"

	"hitkeep/hklog"
	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

type webVitalsProfile struct {
	lcpBase  float64
	inpBase  float64
	clsBase  float64
	fcpBase  float64
	ttfbBase float64
	weight   int
}

type webVitalHitContext struct {
	sessionID uuid.UUID
	pageID    uuid.UUID
	timestamp time.Time
}

var webVitalsProfiles = map[string]webVitalsProfile{
	"/":                                    {lcpBase: 1900, inpBase: 130, clsBase: 0.07, fcpBase: 1150, ttfbBase: 360, weight: 34},
	"/pricing":                             {lcpBase: 2450, inpBase: 210, clsBase: 0.09, fcpBase: 1500, ttfbBase: 520, weight: 26},
	"/features":                            {lcpBase: 2150, inpBase: 170, clsBase: 0.08, fcpBase: 1320, ttfbBase: 440, weight: 18},
	"/signup":                              {lcpBase: 2850, inpBase: 260, clsBase: 0.12, fcpBase: 1650, ttfbBase: 560, weight: 18},
	"/docs/getting-started":                {lcpBase: 1750, inpBase: 120, clsBase: 0.05, fcpBase: 1050, ttfbBase: 320, weight: 20},
	"/docs/configuration":                  {lcpBase: 1850, inpBase: 140, clsBase: 0.06, fcpBase: 1120, ttfbBase: 350, weight: 16},
	"/docs/api-reference":                  {lcpBase: 2350, inpBase: 190, clsBase: 0.08, fcpBase: 1420, ttfbBase: 500, weight: 14},
	"/blog/privacy-first-analytics-2025":   {lcpBase: 2650, inpBase: 180, clsBase: 0.1, fcpBase: 1580, ttfbBase: 470, weight: 14},
	"/blog/replace-google-analytics":       {lcpBase: 2550, inpBase: 175, clsBase: 0.09, fcpBase: 1520, ttfbBase: 460, weight: 12},
	"/blog/self-hosted-vs-cloud-analytics": {lcpBase: 2750, inpBase: 190, clsBase: 0.1, fcpBase: 1620, ttfbBase: 490, weight: 12},
}

func seedWebVitals(ctx context.Context, store *database.Store, siteID uuid.UUID, numDays int, rng *mrand.Rand) (int, error) {
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -numDays).Truncate(24 * time.Hour)
	paths := make([]weightedEntry[string], 0, len(webVitalsProfiles))
	for path, profile := range webVitalsProfiles {
		paths = append(paths, weightedEntry[string]{value: path, weight: profile.weight})
	}
	hitContexts := loadWebVitalHitContexts(ctx, store, siteID, start, now)

	total := 0
	batch := make([]*api.WebVital, 0, 512)
	for d := range numDays {
		day := start.Add(time.Duration(d) * 24 * time.Hour)
		samplesToday := 90 + rng.Intn(60)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			samplesToday = 42 + rng.Intn(30)
		}
		growthFactor := 0.96 + (float64(d)/float64(max(numDays, 1)))*0.08
		if d > numDays/2 {
			growthFactor *= 0.9
		}

		for range samplesToday {
			path := pickWeighted(rng, paths)
			profile := webVitalsProfiles[path]
			sessionID := uuid.New()
			pageID := uuid.New()
			ts := randomTimeInDay(rng, day)
			if contexts := hitContexts[path]; len(contexts) > 0 {
				context := contexts[rng.Intn(len(contexts))]
				sessionID = context.sessionID
				pageID = context.pageID
				ts = context.timestamp.Add(time.Duration(250+rng.Intn(9000)) * time.Millisecond)
			}
			nav := pickWeighted(rng, []weightedEntry[string]{
				{"navigate", 72},
				{"reload", 12},
				{"back_forward", 11},
				{"prerender", 5},
			})

			for _, sample := range webVitalMetricSamples(profile, growthFactor, rng) {
				navCopy := nav
				batch = append(batch, &api.WebVital{
					SiteID:         siteID,
					SessionID:      sessionID,
					PageID:         pageID,
					Metric:         sample.metric,
					Value:          sample.value,
					Path:           path,
					NavigationType: &navCopy,
					Timestamp:      ts.Add(time.Duration(rng.Intn(9000)) * time.Millisecond),
					TrackerSource:  "browser",
					TrackerVersion: "seed",
				})
				total++
			}
		}

		if len(batch) >= 500 {
			if err := store.CreateWebVitalsBulk(ctx, batch); err != nil {
				return total, fmt.Errorf("insert web vitals for %s: %w", day.Format("2006-01-02"), err)
			}
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		if err := store.CreateWebVitalsBulk(ctx, batch); err != nil {
			return total, fmt.Errorf("insert final web vitals batch: %w", err)
		}
	}
	hklog.LoggerFromContext(ctx).Info("Web Vitals seeded", "samples", total)
	return total, nil
}

func loadWebVitalHitContexts(ctx context.Context, store *database.Store, siteID uuid.UUID, start, end time.Time) map[string][]webVitalHitContext {
	rows, err := store.DB().QueryContext(ctx, `
		SELECT session_id, page_id, path, timestamp
		FROM hits
		WHERE site_id = ? AND timestamp >= ? AND timestamp <= ?
	`, siteID, start, end)
	if err != nil {
		hklog.LoggerFromContext(ctx).Warn("Failed to load hit context for Web Vitals seed", "error", err)
		return nil
	}
	defer rows.Close()

	contexts := map[string][]webVitalHitContext{}
	for rows.Next() {
		var (
			sessionID uuid.UUID
			pageID    uuid.UUID
			path      string
			timestamp time.Time
		)
		if err := rows.Scan(&sessionID, &pageID, &path, &timestamp); err != nil {
			hklog.LoggerFromContext(ctx).Warn("Failed to scan hit context for Web Vitals seed", "error", err)
			return contexts
		}
		if _, ok := webVitalsProfiles[path]; !ok {
			continue
		}
		contexts[path] = append(contexts[path], webVitalHitContext{
			sessionID: sessionID,
			pageID:    pageID,
			timestamp: timestamp,
		})
	}
	if err := rows.Err(); err != nil {
		hklog.LoggerFromContext(ctx).Warn("Failed to read hit context for Web Vitals seed", "error", err)
	}
	return contexts
}

type webVitalMetricSample struct {
	metric api.WebVitalMetric
	value  float64
}

func webVitalMetricSamples(profile webVitalsProfile, growthFactor float64, rng *mrand.Rand) []webVitalMetricSample {
	multiplier := growthFactor * (0.75 + rng.Float64()*0.6)
	if rng.Float64() < 0.08 {
		multiplier *= 1.45 + rng.Float64()*0.55
	}
	clsMultiplier := 0.72 + rng.Float64()*0.75
	if rng.Float64() < 0.06 {
		clsMultiplier *= 2.4
	}
	return []webVitalMetricSample{
		{metric: api.WebVitalLCP, value: maxFloat(400, profile.lcpBase*multiplier)},
		{metric: api.WebVitalINP, value: maxFloat(25, profile.inpBase*multiplier)},
		{metric: api.WebVitalCLS, value: maxFloat(0, profile.clsBase*clsMultiplier)},
		{metric: api.WebVitalFCP, value: maxFloat(300, profile.fcpBase*multiplier)},
		{metric: api.WebVitalTTFB, value: maxFloat(50, profile.ttfbBase*multiplier)},
	}
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
