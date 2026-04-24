package main

import (
	"context"
	"fmt"
	"log/slog"
	mrand "math/rand"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

func seedTraffic(ctx context.Context, store *database.Store, siteID uuid.UUID, goals goalIDs, numDays int, rng *mrand.Rand) (seedStats, error) {
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -numDays).Truncate(24 * time.Hour)

	var stats seedStats
	var batch seedWriteBatch

	for d := range numDays {
		day := start.Add(time.Duration(d) * 24 * time.Hour)
		weekday := day.Weekday()

		base := 180
		if weekday == time.Saturday || weekday == time.Sunday {
			base = 80
		}

		growth := 1.0 + (float64(d)/float64(numDays))*0.8
		variation := 0.75 + rng.Float64()*0.5

		if rng.Float64() < 0.07 {
			variation *= 2.5 + rng.Float64()*2.0
		}

		dailyHits := max(int(float64(base)*growth*variation), 10)

		hitsLeft := dailyHits
		for hitsLeft > 0 {
			sessionLen := min(1+rng.Intn(5), hitsLeft)
			hitsLeft -= sessionLen

			sessionID := uuid.New()
			stats.sessions++

			uaEntry := pickWeighted(rng, userAgents)
			country := pickWeighted(rng, countries)
			lang := pickWeighted(rng, languages)
			utmEntry := pickWeighted(rng, utmCampaigns)
			ref := pickWeighted(rng, referrers)

			vw, vh, sw, sh := pickViewport(rng, uaEntry.kind)

			entryPage := pickWeighted(rng, pages)

			sessionStart := randomTimeInDay(rng, day)

			for i := range sessionLen {
				var page string
				if i == 0 {
					page = entryPage
				} else {
					page = pickWeighted(rng, pages)
				}

				ts := sessionStart.Add(time.Duration(i*90+rng.Intn(120)) * time.Second)

				h := &api.Hit{
					SiteID:         siteID,
					SessionID:      sessionID,
					PageID:         uuid.New(),
					Timestamp:      ts,
					Path:           page,
					UserAgent:      new(uaEntry.ua),
					CountryCode:    country,
					Language:       lang,
					ViewportWidth:  new(vw),
					ViewportHeight: new(vh),
					ScreenWidth:    new(sw),
					ScreenHeight:   new(sh),
					IsUnique:       new(i == 0),
				}

				if i == 0 {
					h.Referrer = ref
				}

				if i == 0 && utmEntry != nil {
					h.UTMSource = new(utmEntry.source)
					h.UTMMedium = new(utmEntry.medium)
					h.UTMCampaign = new(utmEntry.campaign)
					h.UTMTerm = utmEntry.term
					h.UTMContent = utmEntry.content
				}

				batch.addHit(h)
				stats.hits++
			}

			events := fireConversionEvents(&batch, siteID, sessionID, goals, rng, sessionStart.Add(time.Duration(sessionLen*90+30)*time.Second), entryPage, utmEntry)
			stats.events += events
		}

		if err := batch.flush(ctx, store); err != nil {
			return stats, fmt.Errorf("flush traffic batch for %s: %w", day.Format("2006-01-02"), err)
		}

		if d%10 == 0 || d == numDays-1 {
			slog.Info("Progress", "day", d+1, "of", numDays, "hits_so_far", stats.hits)
		}
	}

	return stats, nil
}

func seedAIFetches(ctx context.Context, store *database.Store, siteID uuid.UUID, numDays int, rng *mrand.Rand) (aiFetchSeedStats, error) {
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -numDays).Truncate(24 * time.Hour)
	stats := aiFetchSeedStats{}
	var batch seedWriteBatch

	for d := range numDays {
		day := start.Add(time.Duration(d) * 24 * time.Hour)
		fetchesToday := 10 + rng.Intn(12)
		if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday {
			fetchesToday += 4 + rng.Intn(6)
		}
		if rng.Float64() < 0.12 {
			fetchesToday += 8 + rng.Intn(10)
		}

		for i := 0; i < fetchesToday; i++ {
			bot := pickWeighted(rng, aiFetchBots)
			target := pickWeighted(rng, aiFetchTargets)
			responseMs := target.responseMin + rng.Intn(max(target.responseMax-target.responseMin, 1))
			bytesServed := target.bytesMin + rng.Int63n(max64(target.bytesMax-target.bytesMin, 1))
			contentType := target.contentType
			userAgent := bot.userAgent
			hostname := "acme-analytics.io"

			fetch := &api.AIFetch{
				SiteID:          siteID,
				Timestamp:       randomTimeInDay(rng, day),
				AssistantName:   bot.name,
				AssistantFamily: bot.family,
				Path:            target.path,
				Hostname:        &hostname,
				StatusCode:      target.statusCode,
				ContentType:     &contentType,
				ResourceType:    classifySeedResourceType(target.contentType),
				ResponseMs:      &responseMs,
				BytesServed:     &bytesServed,
				UserAgent:       &userAgent,
			}

			batch.addAIFetch(fetch)
			stats.fetches++
			sessionCount, hitCount := seedAIReferredVisits(&batch, siteID, fetch, target, rng)
			stats.sessions += sessionCount
			stats.hits += hitCount
		}

		if err := batch.flush(ctx, store); err != nil {
			return stats, fmt.Errorf("flush ai visibility batch for %s: %w", day.Format("2006-01-02"), err)
		}
	}

	slog.Info("AI visibility seeded", "fetches", stats.fetches, "ai_referred_sessions", stats.sessions, "ai_referred_hits", stats.hits)
	return stats, nil
}
