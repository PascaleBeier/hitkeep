package main

import (
	"fmt"
	mrand "math/rand"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func fireConversionEvents(batch *seedWriteBatch, siteID, sessionID uuid.UUID, goals goalIDs, rng *mrand.Rand, ts time.Time, entryPage string, utm *utmParams) int {
	_ = goals
	count := 0

	type conversionEvent struct {
		prob  float64
		name  string
		props map[string]any
	}

	conversions := []conversionEvent{
		{0.030, "newsletter_signup", map[string]any{
			"source": randomSignupSource(rng),
			"format": randomNewsletterFormat(rng),
		}},
		{0.020, "trial_started", map[string]any{
			"plan":    randomTrialPlan(rng),
			"billing": randomBilling(rng),
		}},
		{0.015, "demo_requested", map[string]any{
			"company_size": randomCompanySize(rng),
			"industry":     randomIndustry(rng),
			"source":       randomDemoSource(rng),
		}},
	}

	for _, c := range conversions {
		if rng.Float64() < c.prob {
			ev := &api.Event{
				SiteID:     siteID,
				SessionID:  sessionID,
				Name:       c.name,
				Properties: c.props,
				Timestamp:  ts,
			}
			batch.addEvent(ev)
			count++
		}
	}
	count += fireEcommerceEvents(batch, siteID, sessionID, rng, ts, entryPage, utm)
	count += fireAIChatbotEvents(batch, siteID, sessionID, rng, ts, entryPage, utm)
	count += fireAutomaticTrackingEvents(batch, siteID, sessionID, rng, ts, entryPage)
	return count
}

func fireAutomaticTrackingEvents(batch *seedWriteBatch, siteID, sessionID uuid.UUID, rng *mrand.Rand, ts time.Time, entryPage string) int {
	count := 0

	outboundProb := 0.09
	downloadProb := 0.06
	formProb := 0.04

	switch entryPage {
	case "/pricing", "/docs/getting-started", "/docs/configuration", "/docs/api-reference":
		outboundProb = 0.18
		downloadProb = 0.14
		formProb = 0.08
	case "/signup", "/contact":
		formProb = 0.16
	case "/blog/privacy-first-analytics-2025", "/blog/self-hosted-vs-cloud-analytics", "/blog/replace-google-analytics":
		outboundProb = 0.2
		downloadProb = 0.11
	}

	if rng.Float64() < outboundProb {
		target := pickWeighted(rng, outboundTargets)
		if insertSeedEvent(batch, &api.Event{
			SiteID:    siteID,
			SessionID: sessionID,
			Name:      "outbound_click",
			Properties: map[string]any{
				"target_host":     target.host,
				"target_path":     target.path,
				"target_protocol": target.protocol,
			},
			Timestamp: ts.Add(15 * time.Second),
		}) {
			count++
		}
	}

	if rng.Float64() < downloadProb {
		target := pickWeighted(rng, downloadTargets)
		if insertSeedEvent(batch, &api.Event{
			SiteID:    siteID,
			SessionID: sessionID,
			Name:      "file_download",
			Properties: map[string]any{
				"file_host": target.host,
				"file_path": target.path,
				"file_ext":  target.ext,
			},
			Timestamp: ts.Add(30 * time.Second),
		}) {
			count++
		}
	}

	if rng.Float64() < formProb {
		target := pickWeighted(rng, formTargets)
		if insertSeedEvent(batch, &api.Event{
			SiteID:    siteID,
			SessionID: sessionID,
			Name:      "form_submit",
			Properties: map[string]any{
				"action_host": target.host,
				"action_path": target.path,
				"method":      target.method,
				"same_origin": target.sameOrigin,
				"form_id":     target.formID,
			},
			Timestamp: ts.Add(45 * time.Second),
		}) {
			count++
		}
	}

	return count
}

func fireEcommerceEvents(batch *seedWriteBatch, siteID, sessionID uuid.UUID, rng *mrand.Rand, ts time.Time, entryPage string, utm *utmParams) int {
	product := pickWeighted(rng, ecommerceProducts)
	viewProb := 0.16
	cartProb := 0.42
	checkoutProb := 0.58
	purchaseProb := 0.62

	switch entryPage {
	case "/pricing", "/signup":
		viewProb = 0.42
		cartProb = 0.56
		checkoutProb = 0.66
		purchaseProb = 0.7
	case "/features", "/docs/getting-started":
		viewProb = 0.26
	}

	if utm != nil {
		switch strings.ToLower(strings.TrimSpace(utm.source)) {
		case "google", "newsletter", "producthunt":
			viewProb += 0.07
			cartProb += 0.05
			checkoutProb += 0.04
			purchaseProb += 0.05
		case "linkedin":
			viewProb += 0.04
			checkoutProb += 0.03
		}
	}

	if rng.Float64() >= minFloat(viewProb, 0.88) {
		return 0
	}

	count := 0
	billing := randomBilling(rng)
	coupon := randomCoupon(rng)
	items, totalValue, totalQuantity, primary := randomPurchaseItems(rng, product, billing)
	currency := "USD"

	viewProps := buildCatalogEventProps(items[0], currency)
	if insertSeedEvent(batch, &api.Event{
		SiteID:     siteID,
		SessionID:  sessionID,
		Name:       "view_item",
		Properties: viewProps,
		Timestamp:  ts,
	}) {
		count++
	}

	if rng.Float64() >= minFloat(cartProb, 0.92) {
		return count
	}

	cartProps := buildCatalogEventProps(items[0], currency)
	cartProps["quantity"] = items[0]["quantity"]
	cartProps["items"] = items
	if insertSeedEvent(batch, &api.Event{
		SiteID:     siteID,
		SessionID:  sessionID,
		Name:       "add_to_cart",
		Properties: cartProps,
		Timestamp:  ts.Add(45 * time.Second),
	}) {
		count++
	}

	if rng.Float64() >= minFloat(checkoutProb, 0.94) {
		return count
	}

	checkoutProps := map[string]any{
		"checkout_id": fmt.Sprintf("chk_%s", uuid.NewString()[:12]),
		"value":       totalValue,
		"currency":    currency,
		"items_count": totalQuantity,
		"coupon":      coupon,
		"items":       items,
	}
	if insertSeedEvent(batch, &api.Event{
		SiteID:     siteID,
		SessionID:  sessionID,
		Name:       "begin_checkout",
		Properties: checkoutProps,
		Timestamp:  ts.Add(90 * time.Second),
	}) {
		count++
	}

	if rng.Float64() >= minFloat(purchaseProb, 0.96) {
		return count
	}

	transactionID := fmt.Sprintf("ord_%s", uuid.NewString()[:12])
	purchaseProps := map[string]any{
		"transaction_id": transactionID,
		"order_id":       transactionID,
		"value":          totalValue,
		"amount":         totalValue,
		"currency":       currency,
		"items_count":    totalQuantity,
		"billing":        billing,
		"tax":            0,
		"shipping":       0,
		"coupon":         coupon,
		"items":          items,
	}
	if insertSeedEvent(batch, &api.Event{
		SiteID:     siteID,
		SessionID:  sessionID,
		Name:       "purchase",
		Properties: purchaseProps,
		Timestamp:  ts.Add(3 * time.Minute),
	}) {
		count++
	}

	legacyPurchaseProps := map[string]any{
		"plan":     primary.plan,
		"billing":  billing,
		"amount":   totalValue,
		"currency": currency,
	}
	if coupon != "" {
		legacyPurchaseProps["coupon"] = coupon
	}
	if insertSeedEvent(batch, &api.Event{
		SiteID:     siteID,
		SessionID:  sessionID,
		Name:       "purchase_completed",
		Properties: legacyPurchaseProps,
		Timestamp:  ts.Add(3*time.Minute + 10*time.Second),
	}) {
		count++
	}

	return count
}

func insertSeedEvent(batch *seedWriteBatch, event *api.Event) bool {
	batch.addEvent(event)
	return true
}

func fireAIChatbotEvents(batch *seedWriteBatch, siteID, sessionID uuid.UUID, rng *mrand.Rand, ts time.Time, entryPage string, utm *utmParams) int {
	startProb := 0.05
	switch entryPage {
	case "/docs/getting-started", "/docs/configuration", "/docs/api-reference":
		startProb = 0.28
	case "/pricing", "/signup":
		startProb = 0.19
	case "/features":
		startProb = 0.14
	case "/contact":
		startProb = 0.18
	}

	if utm != nil {
		switch strings.ToLower(strings.TrimSpace(utm.source)) {
		case "google", "newsletter", "linkedin":
			startProb += 0.03
		case "producthunt":
			startProb += 0.02
		}
	}

	if rng.Float64() >= minFloat(startProb, 0.42) {
		return 0
	}

	count := 0
	conversationID := uuid.NewString()
	bot := pickWeighted(rng, chatbotBots)
	surface := randomChatbotSurface(entryPage, rng)
	intent := randomChatbotIntent(entryPage, rng)
	startedAt := ts.Add(20 * time.Second)
	messageCount := 1
	if rng.Float64() < 0.38 {
		messageCount++
	}
	if rng.Float64() < 0.14 {
		messageCount++
	}

	if insertSeedEvent(batch, &api.Event{
		SiteID:    siteID,
		SessionID: sessionID,
		Name:      "assistant.chat_started",
		Properties: map[string]any{
			"bot_id":   bot.botID,
			"provider": bot.provider,
			"model":    bot.model,
			"surface":  surface,
		},
		Timestamp: startedAt,
	}) {
		count++
	}

	responseBase := startedAt.Add(8 * time.Second)
	citationCount := 0
	for i := 0; i < messageCount; i++ {
		messageIndex := i + 1
		promptAt := startedAt.Add(time.Duration(i) * 95 * time.Second)
		responseMs := 450 + rng.Intn(1100)
		toolCount := 0
		if rng.Float64() < 0.42 {
			toolCount = 1 + rng.Intn(2)
		}
		currentCitations := 0
		if rng.Float64() < 0.58 {
			currentCitations = 1 + rng.Intn(3)
		}
		citationCount += currentCitations

		if insertSeedEvent(batch, &api.Event{
			SiteID:    siteID,
			SessionID: sessionID,
			Name:      "assistant.message_sent",
			Properties: map[string]any{
				"conversation_id": conversationID,
				"message_index":   messageIndex,
				"intent":          intent,
			},
			Timestamp: promptAt,
		}) {
			count++
		}

		if insertSeedEvent(batch, &api.Event{
			SiteID:    siteID,
			SessionID: sessionID,
			Name:      "assistant.response_rendered",
			Properties: map[string]any{
				"conversation_id": conversationID,
				"message_index":   messageIndex,
				"response_ms":     responseMs,
				"tool_count":      toolCount,
				"citation_count":  currentCitations,
			},
			Timestamp: responseBase.Add(time.Duration(i) * 95 * time.Second),
		}) {
			count++
		}

		if currentCitations > 0 && rng.Float64() < 0.34 {
			if insertSeedEvent(batch, &api.Event{
				SiteID:    siteID,
				SessionID: sessionID,
				Name:      "assistant.citation_clicked",
				Properties: map[string]any{
					"conversation_id": conversationID,
					"citation_url":    randomCitationURL(entryPage, rng),
					"citation_index":  1 + rng.Intn(currentCitations),
				},
				Timestamp: responseBase.Add(time.Duration(i)*95*time.Second + 12*time.Second),
			}) {
				count++
			}
		}
	}

	if rng.Float64() < 0.16 {
		if insertSeedEvent(batch, &api.Event{
			SiteID:    siteID,
			SessionID: sessionID,
			Name:      "assistant.handoff_requested",
			Properties: map[string]any{
				"conversation_id": conversationID,
				"message_index":   messageCount,
				"reason":          randomHandoffReason(entryPage, rng),
			},
			Timestamp: responseBase.Add(time.Duration(messageCount)*95*time.Second + 15*time.Second),
		}) {
			count++
		}
	}

	if rng.Float64() < assistedGoalProbability(entryPage) {
		goalName := randomChatbotGoalName(entryPage, rng)
		if insertSeedEvent(batch, &api.Event{
			SiteID:    siteID,
			SessionID: sessionID,
			Name:      "assistant.goal_assisted",
			Properties: map[string]any{
				"conversation_id": conversationID,
				"goal_name":       goalName,
				"goal_value":      goalName,
			},
			Timestamp: responseBase.Add(time.Duration(messageCount)*95*time.Second + 30*time.Second),
		}) {
			count++
		}
	}

	return count
}

func buildCatalogEventProps(item map[string]any, currency string) map[string]any {
	props := map[string]any{
		"item_id":   item["item_id"],
		"item_name": item["item_name"],
		"category":  item["item_category"],
		"price":     item["price"],
		"currency":  currency,
		"items":     []map[string]any{item},
	}
	if quantity, ok := item["quantity"]; ok {
		props["quantity"] = quantity
	}
	return props
}

func randomPurchaseItems(rng *mrand.Rand, product ecommerceProduct, billing string) ([]map[string]any, float64, int, ecommerceProduct) {
	primaryPrice := productPrice(product, billing)
	primaryQuantity := 1
	if product.category == "add-on" {
		primaryQuantity = 1 + rng.Intn(3)
	}

	items := []map[string]any{
		{
			"item_id":       product.itemID,
			"item_name":     product.itemName,
			"item_category": product.category,
			"price":         primaryPrice,
			"quantity":      primaryQuantity,
		},
	}
	total := float64(primaryPrice * primaryQuantity)
	totalQuantity := primaryQuantity

	if product.plan == "business" && rng.Float64() < 0.42 {
		seatPack := ecommerceProduct{itemID: "team-seat-pack", itemName: "Team Seat Pack", plan: "business", category: "add-on", price: 49, priceYear: 490}
		quantity := 1 + rng.Intn(3)
		items = append(items, map[string]any{
			"item_id":       seatPack.itemID,
			"item_name":     seatPack.itemName,
			"item_category": seatPack.category,
			"price":         seatPack.price,
			"quantity":      quantity,
		})
		total += float64(seatPack.price * quantity)
		totalQuantity += quantity
	}

	if billing == "annual" && rng.Float64() < 0.18 {
		upgrade := ecommerceProduct{itemID: "annual-upgrade", itemName: "Annual Upgrade", plan: product.plan, category: "upgrade", price: 199, priceYear: 199}
		items = append(items, map[string]any{
			"item_id":       upgrade.itemID,
			"item_name":     upgrade.itemName,
			"item_category": upgrade.category,
			"price":         upgrade.price,
			"quantity":      1,
		})
		total += float64(upgrade.price)
		totalQuantity++
	}

	return items, total, totalQuantity, product
}
