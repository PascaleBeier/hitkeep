package main

import (
	"context"
	"fmt"
	mrand "math/rand"
	"time"

	"github.com/google/uuid"

	"hitkeep/hklog"
	"hitkeep/internal/aianalytics"
	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

// seedDemoHostname is the hostname every seeded hit and fetch record carries.
// It matches the default -domain flag; seedHostname() is the runtime normalizer
// for a caller-supplied domain and is a different thing entirely.
const seedDemoHostname = "acme-analytics.io"

func seedTraffic(ctx context.Context, store *database.Store, siteID uuid.UUID, goals goalIDs, numDays int, rng *mrand.Rand) (seedStats, error) {
	now := time.Now().UTC()
	if numDays <= 0 {
		return seedStats{}, nil
	}
	start := now.AddDate(0, 0, -(numDays - 1)).Truncate(24 * time.Hour)

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
			region, city, provider, asn, asnOrg := seedGeoNetworkMetadata(country, rng)
			lang := pickWeighted(rng, languages)
			utmEntry := pickWeighted(rng, utmCampaigns)
			ref := pickWeighted(rng, referrers)

			vw, vh, sw, sh := pickViewport(rng, uaEntry.kind)

			entryPage := pickWeighted(rng, pages)

			sessionStart := randomTimeInElapsedDay(rng, day, now)

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
					Region:         region,
					City:           city,
					Provider:       provider,
					ASN:            asn,
					ASNOrg:         asnOrg,
					Language:       lang,
					ViewportWidth:  &vw,
					ViewportHeight: &vh,
					ScreenWidth:    &sw,
					ScreenHeight:   &sh,
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
			hklog.LoggerFromContext(ctx).Debug("Progress", "day", d+1, "of", numDays, "hits_so_far", stats.hits)
		}
	}

	return stats, nil
}

// seedAIVisibility seeds the whole AI visibility surface for a site: server-log
// AI fetches, the AI-referred visits they drive, and tracked AI bot pageviews.
func seedAIVisibility(ctx context.Context, store *database.Store, siteID uuid.UUID, numDays int, rng *mrand.Rand) (aiFetchSeedStats, error) {
	now := time.Now().UTC()
	if numDays <= 0 {
		return aiFetchSeedStats{}, nil
	}
	start := now.AddDate(0, 0, -(numDays - 1)).Truncate(24 * time.Hour)
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
			bytesServed := target.bytesMin + rng.Int63n(max(target.bytesMax-target.bytesMin, 1))
			contentType := target.contentType
			userAgent := bot.userAgent
			hostname := seedDemoHostname

			fetch := &api.AIFetch{
				SiteID:            siteID,
				Timestamp:         randomTimeInElapsedDay(rng, day, now),
				AssistantName:     bot.name,
				AssistantFamily:   bot.family,
				AssistantCategory: classifySeedAssistantCategory(userAgent),
				Path:              target.path,
				Hostname:          &hostname,
				StatusCode:        target.statusCode,
				ContentType:       &contentType,
				ResourceType:      classifySeedResourceType(target.contentType),
				ResponseMs:        &responseMs,
				BytesServed:       &bytesServed,
				UserAgent:         &userAgent,
			}

			batch.addAIFetch(fetch)
			stats.fetches++
			sessionCount, hitCount := seedAIReferredVisits(&batch, siteID, fetch, target, rng)
			stats.sessions += sessionCount
			stats.hits += hitCount
		}

		stats.botHits += seedAIBotHits(&batch, siteID, day, now, rng)

		if day.UTC().Truncate(24 * time.Hour).Equal(now.Truncate(24 * time.Hour)) {
			for _, spec := range pinnedAIFetches {
				batch.addAIFetch(seedPinnedAIFetch(siteID, day, now, spec))
				stats.fetches++
			}
		}

		if err := batch.flush(ctx, store); err != nil {
			return stats, fmt.Errorf("flush ai visibility batch for %s: %w", day.Format("2006-01-02"), err)
		}
	}

	hklog.LoggerFromContext(ctx).Info("AI visibility seeded", "fetches", stats.fetches, "ai_referred_sessions", stats.sessions, "ai_referred_hits", stats.hits, "ai_bot_hits", stats.botHits)
	return stats, nil
}

// seedAIBotHits adds tracked pageview hits from AI crawler user agents for one
// day. It reuses the aiFetchBots user agents and aiFetchTargets paths so the
// tracked bot traffic and the server-log fetch records show the same agent mix.
//
// Bots get single-hit sessions with no referrer and no viewport: real crawlers
// never report one, and that absence is what distinguishes them from browser
// sessions in the tracked stream. hk_device falls through to 'Desktop' for a
// NULL viewport, which is a property of the macro rather than something the
// fixture should paper over with a fabricated browser size.
func seedAIBotHits(batch *seedWriteBatch, siteID uuid.UUID, day, now time.Time, rng *mrand.Rand) int {
	hitsToday := 8 + rng.Intn(5)
	if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday {
		hitsToday += rng.Intn(5)
	}

	hostname := seedDemoHostname
	for range hitsToday {
		bot := pickWeighted(rng, aiFetchBots)
		target := pickWeighted(rng, aiFetchTargets)
		userAgent := bot.userAgent
		country := pickWeighted(rng, countries)
		region, city, provider, asn, asnOrg := seedGeoNetworkMetadata(country, rng)

		batch.addHit(&api.Hit{
			SiteID:      siteID,
			SessionID:   uuid.New(),
			PageID:      uuid.New(),
			Timestamp:   randomTimeInElapsedDay(rng, day, now),
			Path:        target.path,
			Hostname:    &hostname,
			UserAgent:   &userAgent,
			CountryCode: country,
			Region:      region,
			City:        city,
			Provider:    provider,
			ASN:         asn,
			ASNOrg:      asnOrg,
			IsUnique:    new(true),
		})
	}

	return hitsToday
}

// pinnedAIFetch describes one deterministic fetch record pinned to a fixed
// minute of the current day, so the demo always has same-day rows inside the
// dashboard's default report range.
type pinnedAIFetch struct {
	name        string
	family      string
	userAgent   string
	offset      time.Duration
	responseMs  int
	bytesServed int64
	// legacy leaves assistant_category empty, mimicking a row ingested before
	// that column existed. The merged AI activity report then has to recover the
	// category at query time through hk_ai_bot_category_from_name(assistant_name).
	legacy bool
}

// pinnedAIFetches keeps exactly one legacy record, so the category fallback
// stays visible in the demo data instead of only in tests.
var pinnedAIFetches = []pinnedAIFetch{
	{
		name:        "GPTBot",
		family:      "OpenAI",
		userAgent:   "Mozilla/5.0 (compatible; GPTBot/1.2; +https://openai.com/gptbot)",
		offset:      15 * time.Minute,
		responseMs:  144,
		bytesServed: 28_640,
	},
	{
		name:        "ClaudeBot",
		family:      "Anthropic",
		userAgent:   "Mozilla/5.0 (compatible; ClaudeBot/1.0; +https://www.anthropic.com/bot)",
		offset:      35 * time.Minute,
		responseMs:  168,
		bytesServed: 31_200,
		legacy:      true,
	},
}

func seedPinnedAIFetch(siteID uuid.UUID, day, now time.Time, spec pinnedAIFetch) *api.AIFetch {
	hostname := seedDemoHostname
	contentType := "text/html; charset=utf-8"
	responseMs := spec.responseMs
	bytesServed := spec.bytesServed
	userAgent := spec.userAgent
	timestamp := day.UTC().Truncate(24 * time.Hour).Add(spec.offset)
	if timestamp.After(now) {
		timestamp = now
	}

	category := ""
	if !spec.legacy {
		category = classifySeedAssistantCategory(userAgent)
	}

	return &api.AIFetch{
		SiteID:            siteID,
		Timestamp:         timestamp,
		AssistantName:     spec.name,
		AssistantFamily:   spec.family,
		AssistantCategory: category,
		Path:              "/docs/getting-started",
		Hostname:          &hostname,
		StatusCode:        200,
		ContentType:       &contentType,
		ResourceType:      "html",
		ResponseMs:        &responseMs,
		BytesServed:       &bytesServed,
		UserAgent:         &userAgent,
	}
}

// classifySeedAssistantCategory derives the AI agent category from the seeded
// user agent with the same classifier the ingest path uses, so demo rows match
// what a real deployment would have stored.
func classifySeedAssistantCategory(userAgent string) string {
	if identity := aianalytics.ClassifyBot(userAgent); identity != nil {
		return identity.Category
	}
	return ""
}
