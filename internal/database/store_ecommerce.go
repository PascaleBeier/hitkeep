package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"hitkeep/internal/api"
)

type ecommerceEventRecord struct {
	name        string
	timestamp   time.Time
	properties  []byte
	utmSource   string
	utmMedium   string
	utmCampaign string
	referrer    string
	device      string
	country     string
	city        string
	provider    string
	asn         string
}

type ecommerceItem struct {
	itemID   string
	itemName string
	quantity int
	price    float64
}

type ecommercePurchase struct {
	timestamp   time.Time
	transaction string
	value       float64
	currency    string
	coupon      string
	items       []ecommerceItem
	utmSource   string
	utmMedium   string
	utmCampaign string
	referrer    string
}

type ecommerceCheckout struct {
	timestamp time.Time
	items     []ecommerceItem
	city      string
	provider  string
	asn       string
}

type ecommerceDataset struct {
	purchases []ecommercePurchase
	checkouts []ecommerceCheckout
}

type ecommerceProductAccumulator struct {
	itemID   string
	itemName string
	revenue  float64
	orders   int
	quantity int
}

type ecommerceSourceAccumulator struct {
	utmSource   string
	utmMedium   string
	utmCampaign string
	referrer    string
	revenue     float64
	orders      int
}

func normalizeEcommerceEventName(name string) string {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "order_completed":
		return "purchase"
	case "checkout_started":
		return "begin_checkout"
	case "product_viewed":
		return "view_item"
	default:
		return strings.TrimSpace(strings.ToLower(name))
	}
}

func (s *Store) GetEcommerceSummary(ctx context.Context, params api.EcommerceParams) (*api.EcommerceSummary, error) {
	dataset, err := s.loadEcommerceDataset(ctx, params)
	if err != nil {
		return nil, err
	}

	summary := &api.EcommerceSummary{
		Currency: "",
	}

	currencyCounts := map[string]int{}
	cityCounts := map[string]int{}
	providerCounts := map[string]int{}
	asnCounts := map[string]int{}
	for _, purchase := range dataset.purchases {
		summary.Orders++
		summary.Revenue += purchase.value
		if purchase.currency != "" {
			currencyCounts[purchase.currency]++
		}
	}
	for _, checkout := range dataset.checkouts {
		if matchesEcommerceItemFilter(checkout.items, params.ItemID, params.ItemName) {
			summary.CheckoutStarts++
			incrementMetricCount(cityCounts, checkout.city)
			incrementMetricCount(providerCounts, checkout.provider)
			incrementMetricCount(asnCounts, checkout.asn)
		}
	}

	if summary.Orders > 0 {
		summary.AverageOrderValue = summary.Revenue / float64(summary.Orders)
	}
	if summary.CheckoutStarts > 0 {
		summary.CheckoutConversionRate = (float64(summary.Orders) / float64(summary.CheckoutStarts)) * 100
	}
	bestCount := 0
	for currency, count := range currencyCounts {
		if count > bestCount {
			bestCount = count
			summary.Currency = currency
		}
	}
	summary.TopCities = ecommerceMetricStats(cityCounts, params.Limit)
	summary.TopProviders = ecommerceMetricStats(providerCounts, params.Limit)
	summary.TopASNs = ecommerceMetricStats(asnCounts, params.Limit)

	return summary, nil
}

func (s *Store) GetEcommerceTimeSeries(ctx context.Context, params api.EcommerceParams) ([]api.EcommerceSeriesPoint, error) {
	dataset, err := s.loadEcommerceDataset(ctx, params)
	if err != nil {
		return nil, err
	}

	truncUnit := ecommerceTruncUnit(params.Start, params.End)
	revenueByBucket := make(map[time.Time]float64)
	ordersByBucket := make(map[time.Time]int)

	for _, purchase := range dataset.purchases {
		bucket := truncToUnit(purchase.timestamp, truncUnit)
		revenueByBucket[bucket] += purchase.value
		ordersByBucket[bucket]++
	}

	buckets := buildSeriesBuckets(params.Start, params.End, truncUnit)
	series := make([]api.EcommerceSeriesPoint, 0, len(buckets))
	for _, bucket := range buckets {
		series = append(series, api.EcommerceSeriesPoint{
			Time:    bucket,
			Revenue: revenueByBucket[bucket],
			Orders:  ordersByBucket[bucket],
		})
	}

	return series, nil
}

func (s *Store) GetEcommerceTopProducts(ctx context.Context, params api.EcommerceParams) ([]api.EcommerceProductStat, error) {
	dataset, err := s.loadEcommerceDataset(ctx, params)
	if err != nil {
		return nil, err
	}

	accumulators := map[string]*ecommerceProductAccumulator{}
	for _, purchase := range dataset.purchases {
		if len(purchase.items) == 0 {
			continue
		}
		for _, item := range purchase.items {
			key := ecommerceProductKey(item.itemID, item.itemName)
			acc, ok := accumulators[key]
			if !ok {
				acc = &ecommerceProductAccumulator{
					itemID:   item.itemID,
					itemName: item.itemName,
				}
				accumulators[key] = acc
			}
			acc.quantity += item.quantity
			acc.orders++
			acc.revenue += float64(item.quantity) * item.price
		}
	}

	stats := make([]api.EcommerceProductStat, 0, len(accumulators))
	for _, acc := range accumulators {
		stats = append(stats, api.EcommerceProductStat{
			ItemID:   acc.itemID,
			ItemName: acc.itemName,
			Revenue:  acc.revenue,
			Orders:   acc.orders,
			Quantity: acc.quantity,
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Revenue == stats[j].Revenue {
			if stats[i].Orders == stats[j].Orders {
				return stats[i].ItemName < stats[j].ItemName
			}
			return stats[i].Orders > stats[j].Orders
		}
		return stats[i].Revenue > stats[j].Revenue
	})

	limit := normalizeEcommerceLimit(params.Limit)
	if len(stats) > limit {
		stats = stats[:limit]
	}

	if stats == nil {
		return []api.EcommerceProductStat{}, nil
	}
	return stats, nil
}

func (s *Store) GetEcommerceSources(ctx context.Context, params api.EcommerceParams) ([]api.EcommerceSourceStat, error) {
	dataset, err := s.loadEcommerceDataset(ctx, params)
	if err != nil {
		return nil, err
	}

	accumulators := map[string]*ecommerceSourceAccumulator{}
	for _, purchase := range dataset.purchases {
		key := strings.Join([]string{purchase.utmSource, purchase.utmMedium, purchase.utmCampaign, purchase.referrer}, "\x1f")
		acc, ok := accumulators[key]
		if !ok {
			acc = &ecommerceSourceAccumulator{
				utmSource:   purchase.utmSource,
				utmMedium:   purchase.utmMedium,
				utmCampaign: purchase.utmCampaign,
				referrer:    purchase.referrer,
			}
			accumulators[key] = acc
		}
		acc.orders++
		acc.revenue += purchase.value
	}

	stats := make([]api.EcommerceSourceStat, 0, len(accumulators))
	for _, acc := range accumulators {
		stats = append(stats, api.EcommerceSourceStat{
			UTMSource:   acc.utmSource,
			UTMMedium:   acc.utmMedium,
			UTMCampaign: acc.utmCampaign,
			Referrer:    acc.referrer,
			Revenue:     acc.revenue,
			Orders:      acc.orders,
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Revenue == stats[j].Revenue {
			if stats[i].Orders == stats[j].Orders {
				if stats[i].UTMSource == stats[j].UTMSource {
					return stats[i].UTMCampaign < stats[j].UTMCampaign
				}
				return stats[i].UTMSource < stats[j].UTMSource
			}
			return stats[i].Orders > stats[j].Orders
		}
		return stats[i].Revenue > stats[j].Revenue
	})

	limit := normalizeEcommerceLimit(params.Limit)
	if len(stats) > limit {
		stats = stats[:limit]
	}

	if stats == nil {
		return []api.EcommerceSourceStat{}, nil
	}
	return stats, nil
}

func (s *Store) loadEcommerceDataset(ctx context.Context, params api.EcommerceParams) (*ecommerceDataset, error) {
	records, err := s.listEcommerceEvents(ctx, params)
	if err != nil {
		return nil, err
	}

	dataset := &ecommerceDataset{
		purchases: []ecommercePurchase{},
		checkouts: []ecommerceCheckout{},
	}

	for _, record := range records {
		props := map[string]any{}
		if len(record.properties) > 0 {
			if err := json.Unmarshal(record.properties, &props); err != nil {
				continue
			}
		}

		switch normalizeEcommerceEventName(record.name) {
		case "purchase":
			purchase, ok := normalizeEcommercePurchase(record.timestamp, record, props)
			if !ok {
				continue
			}
			if !matchesEcommerceItemFilter(purchase.items, params.ItemID, params.ItemName) {
				continue
			}
			dataset.purchases = append(dataset.purchases, purchase)
		case "begin_checkout":
			checkout := ecommerceCheckout{
				timestamp: record.timestamp,
				items:     normalizeEcommerceItems(props),
				city:      record.city,
				provider:  record.provider,
				asn:       record.asn,
			}
			dataset.checkouts = append(dataset.checkouts, checkout)
		}
	}

	return dataset, nil
}

func (s *Store) listEcommerceEvents(ctx context.Context, params api.EcommerceParams) ([]ecommerceEventRecord, error) {
	filterSQL, filterArgs := buildHitFilters(params.Filters, "h")

	withParts := []string{}
	args := make([]any, 0, 16)

	if len(params.Filters) > 0 {
		withParts = append(withParts, fmt.Sprintf(`
		session_scope AS (
			SELECT DISTINCT h.session_id
			FROM hits h
			WHERE h.site_id = ? AND h.timestamp >= ? AND h.timestamp <= ?%s
		)
	`, filterSQL))
		args = append(args, params.SiteID, params.Start, params.End)
		args = append(args, filterArgs...)
	}

	withParts = append(withParts, `
		session_attrs AS (
			SELECT
				h.session_id,
				arg_min(COALESCE(NULLIF(TRIM(h.utm_source), ''), '(Unspecified)'), h.timestamp) AS utm_source,
				arg_min(COALESCE(NULLIF(TRIM(h.utm_medium), ''), '(Unspecified)'), h.timestamp) AS utm_medium,
				arg_min(COALESCE(NULLIF(TRIM(h.utm_campaign), ''), '(Unspecified)'), h.timestamp) AS utm_campaign,
				arg_min(hk_referrer(h.referrer), h.timestamp) AS referrer,
				arg_min(hk_device(h.viewport_width), h.timestamp) AS device,
				arg_min(hk_country(h.country_code), h.timestamp) AS country,
				arg_min(COALESCE(NULLIF(TRIM(h.city), ''), '(Unknown)'), h.timestamp) AS city,
				arg_min(COALESCE(NULLIF(TRIM(h.provider), ''), '(Unknown)'), h.timestamp) AS provider,
				arg_min(hk_asn(h.asn, h.asn_org), h.timestamp) AS asn
			FROM hits h
			WHERE h.site_id = ? AND h.timestamp >= ? AND h.timestamp <= ?
			GROUP BY h.session_id
		)
	`)
	args = append(args, params.SiteID, params.Start, params.End)

	joinScope := ""
	if len(params.Filters) > 0 {
		joinScope = "INNER JOIN session_scope ss ON ss.session_id = e.session_id"
	}

	args = append(args, params.SiteID, params.Start, params.End)
	// #nosec G201 -- query fragments are assembled from fixed internal SQL templates plus parameterized filter clauses.
	query := fmt.Sprintf(`
		WITH %s
		SELECT
			e.name,
			e.timestamp,
			e.properties,
			sa.utm_source,
			sa.utm_medium,
			sa.utm_campaign,
			sa.referrer,
			sa.device,
			sa.country,
			sa.city,
			sa.provider,
			sa.asn
		FROM events e
		%s
		LEFT JOIN session_attrs sa ON sa.session_id = e.session_id
		WHERE e.site_id = ? AND e.timestamp >= ? AND e.timestamp <= ?
			AND e.name IN ('purchase', 'begin_checkout', 'view_item', 'add_to_cart', 'product_viewed', 'checkout_started', 'order_completed')
		ORDER BY e.timestamp
	`, strings.Join(withParts, ","), joinScope)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := []ecommerceEventRecord{}
	for rows.Next() {
		var (
			record                ecommerceEventRecord
			rawProps              any
			utmSource, utmMedium  sql.NullString
			utmCampaign, referrer sql.NullString
			device, country       sql.NullString
			city, provider, asn   sql.NullString
		)
		if err := rows.Scan(
			&record.name,
			&record.timestamp,
			&rawProps,
			&utmSource,
			&utmMedium,
			&utmCampaign,
			&referrer,
			&device,
			&country,
			&city,
			&provider,
			&asn,
		); err != nil {
			return nil, err
		}
		properties, err := normalizeJSONScanValue(rawProps)
		if err != nil {
			return nil, err
		}
		record.properties = properties
		record.utmSource = nullStringOrDefault(utmSource, "(Unspecified)")
		record.utmMedium = nullStringOrDefault(utmMedium, "(Unspecified)")
		record.utmCampaign = nullStringOrDefault(utmCampaign, "(Unspecified)")
		record.referrer = nullStringOrDefault(referrer, "(Direct)")
		record.device = nullStringOrDefault(device, "(Unknown)")
		record.country = nullStringOrDefault(country, "(Unknown)")
		record.city = nullStringOrDefault(city, "(Unknown)")
		record.provider = nullStringOrDefault(provider, "(Unknown)")
		record.asn = nullStringOrDefault(asn, "(Unknown)")
		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

func normalizeJSONScanValue(value any) ([]byte, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case []byte:
		return typed, nil
	case string:
		return []byte(typed), nil
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return nil, fmt.Errorf("marshal scanned json value: %w", err)
		}
		return encoded, nil
	}
}

func normalizeEcommercePurchase(timestamp time.Time, record ecommerceEventRecord, props map[string]any) (ecommercePurchase, bool) {
	items := normalizeEcommerceItems(props)
	value, ok := firstFloat(props["value"], props["amount"])
	if !ok {
		value = 0
	}
	transaction, _ := firstString(props["transaction_id"], props["order_id"])
	currency, _ := firstString(props["currency"])
	coupon, _ := firstString(props["coupon"])

	return ecommercePurchase{
		timestamp:   timestamp,
		transaction: transaction,
		value:       value,
		currency:    currency,
		coupon:      coupon,
		items:       items,
		utmSource:   record.utmSource,
		utmMedium:   record.utmMedium,
		utmCampaign: record.utmCampaign,
		referrer:    record.referrer,
	}, true
}

func normalizeEcommerceItems(props map[string]any) []ecommerceItem {
	items := make([]ecommerceItem, 0, 4)
	if rawItems, ok := props["items"].([]any); ok {
		for _, rawItem := range rawItems {
			itemMap, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			item := normalizeEcommerceItem(itemMap)
			if item.itemID == "" && item.itemName == "" {
				continue
			}
			items = append(items, item)
		}
	}
	if len(items) > 0 {
		return items
	}

	item := normalizeEcommerceItem(props)
	if item.itemID == "" && item.itemName == "" {
		return []ecommerceItem{}
	}
	return []ecommerceItem{item}
}

func normalizeEcommerceItem(props map[string]any) ecommerceItem {
	itemID, _ := firstString(props["item_id"], props["product_id"])
	itemName, _ := firstString(props["item_name"], props["product_name"])
	price, _ := firstFloat(props["price"], props["value"], props["amount"])
	quantity, ok := firstInt(props["quantity"])
	if !ok || quantity < 1 {
		quantity = 1
	}

	return ecommerceItem{
		itemID:   itemID,
		itemName: itemName,
		quantity: quantity,
		price:    price,
	}
}

func matchesEcommerceItemFilter(items []ecommerceItem, itemID, itemName string) bool {
	normalizedItemID := strings.TrimSpace(strings.ToLower(itemID))
	normalizedItemName := strings.TrimSpace(strings.ToLower(itemName))
	if normalizedItemID == "" && normalizedItemName == "" {
		return true
	}

	for _, item := range items {
		if normalizedItemID != "" && strings.TrimSpace(strings.ToLower(item.itemID)) == normalizedItemID {
			return true
		}
		if normalizedItemName != "" && strings.TrimSpace(strings.ToLower(item.itemName)) == normalizedItemName {
			return true
		}
	}

	return false
}

func ecommerceTruncUnit(start, end time.Time) string {
	duration := end.Sub(start)
	switch {
	case duration < 48*time.Hour:
		return "hour"
	case duration >= 180*24*time.Hour:
		return "month"
	default:
		return "day"
	}
}

func normalizeEcommerceLimit(limit int) int {
	switch {
	case limit <= 0:
		return 10
	case limit > 50:
		return 50
	default:
		return limit
	}
}

func incrementMetricCount(counts map[string]int, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "(Unknown)"
	}
	counts[name]++
}

func ecommerceMetricStats(counts map[string]int, limit int) []api.MetricStat {
	stats := make([]api.MetricStat, 0, len(counts))
	for name, value := range counts {
		stats = append(stats, api.MetricStat{Name: name, Value: value})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Value == stats[j].Value {
			return stats[i].Name < stats[j].Name
		}
		return stats[i].Value > stats[j].Value
	})
	limit = normalizeEcommerceLimit(limit)
	if len(stats) > limit {
		stats = stats[:limit]
	}
	if stats == nil {
		return []api.MetricStat{}
	}
	return stats
}

func ecommerceProductKey(itemID, itemName string) string {
	switch {
	case itemID != "":
		return "id:" + itemID
	case itemName != "":
		return "name:" + itemName
	default:
		return "unknown"
	}
}

func nullStringOrDefault(value sql.NullString, fallback string) string {
	if value.Valid && strings.TrimSpace(value.String) != "" {
		return value.String
	}
	return fallback
}

func firstString(values ...any) (string, bool) {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed != "" {
				return trimmed, true
			}
		}
	}
	return "", false
}

func firstFloat(values ...any) (float64, bool) {
	for _, value := range values {
		switch typed := value.(type) {
		case float64:
			return typed, true
		case float32:
			return float64(typed), true
		case int:
			return float64(typed), true
		case int64:
			return float64(typed), true
		case int32:
			return float64(typed), true
		case json.Number:
			parsed, err := typed.Float64()
			if err == nil {
				return parsed, true
			}
		}
	}
	return 0, false
}

func firstInt(values ...any) (int, bool) {
	for _, value := range values {
		switch typed := value.(type) {
		case int:
			return typed, true
		case int64:
			return int(typed), true
		case int32:
			return int(typed), true
		case float64:
			return int(typed), true
		case float32:
			return int(typed), true
		case json.Number:
			parsed, err := typed.Int64()
			if err == nil {
				return int(parsed), true
			}
		}
	}
	return 0, false
}
