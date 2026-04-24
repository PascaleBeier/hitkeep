package system

func openAPIV1Schemas() map[string]any {
	return mergeOpenAPIMapGroups(
		openAPIV1AnalyticsSchemas(),
		openAPIV1AccountSchemas(),
	)
}

func openAPIV1AnalyticsSchemas() map[string]any {
	return map[string]any{
		"Error": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
		"Status": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":  map[string]any{"type": "string"},
				"message": map[string]any{"type": "string"},
			},
		},
		"AdminDeleteUserBlockedResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":  map[string]any{"type": "string"},
				"code":    map[string]any{"type": "string"},
				"message": map[string]any{"type": "string"},
				"teams":   map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Team"}},
			},
			"required": []string{"status", "code", "message", "teams"},
		},
		"SiteTransferResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":              map[string]any{"type": "string"},
				"site_id":             map[string]any{"type": "string", "format": "uuid"},
				"source_team_id":      map[string]any{"type": "string", "format": "uuid"},
				"destination_team_id": map[string]any{"type": "string", "format": "uuid"},
			},
		},
		"Site": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":                  map[string]any{"type": "string", "format": "uuid"},
				"user_id":             map[string]any{"type": "string", "format": "uuid"},
				"domain":              map[string]any{"type": "string"},
				"data_retention_days": map[string]any{"type": "integer"},
				"created_at":          map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"ShareLink": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":         map[string]any{"type": "string", "format": "uuid"},
				"site_id":    map[string]any{"type": "string", "format": "uuid"},
				"token_hint": map[string]any{"type": "string"},
				"created_at": map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"Hit": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":              map[string]any{"type": "string", "format": "uuid"},
				"site_id":         map[string]any{"type": "string", "format": "uuid"},
				"session_id":      map[string]any{"type": "string", "format": "uuid"},
				"page_id":         map[string]any{"type": "string", "format": "uuid"},
				"timestamp":       map[string]any{"type": "string", "format": "date-time"},
				"path":            map[string]any{"type": "string"},
				"hostname":        map[string]any{"type": "string"},
				"referrer":        map[string]any{"type": "string"},
				"user_agent":      map[string]any{"type": "string"},
				"viewport_width":  map[string]any{"type": "integer"},
				"viewport_height": map[string]any{"type": "integer"},
				"screen_width":    map[string]any{"type": "integer"},
				"screen_height":   map[string]any{"type": "integer"},
				"language":        map[string]any{"type": "string"},
				"country_code":    map[string]any{"type": "string"},
				"utm_source":      map[string]any{"type": "string"},
				"utm_medium":      map[string]any{"type": "string"},
				"utm_campaign":    map[string]any{"type": "string"},
				"utm_term":        map[string]any{"type": "string"},
				"utm_content":     map[string]any{"type": "string"},
				"is_unique":       map[string]any{"type": "boolean"},
			},
		},
		"PaginatedHits": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data":  map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Hit"}},
				"total": map[string]any{"type": "integer"},
			},
		},
		"MetricStat": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":  map[string]any{"type": "string"},
				"value": map[string]any{"type": "integer"},
			},
		},
		"AIFetch": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":               map[string]any{"type": "string", "format": "uuid"},
				"site_id":          map[string]any{"type": "string", "format": "uuid"},
				"timestamp":        map[string]any{"type": "string", "format": "date-time"},
				"assistant_name":   map[string]any{"type": "string"},
				"assistant_family": map[string]any{"type": "string"},
				"path":             map[string]any{"type": "string"},
				"hostname":         map[string]any{"type": "string"},
				"status_code":      map[string]any{"type": "integer"},
				"content_type":     map[string]any{"type": "string"},
				"resource_type":    map[string]any{"type": "string", "enum": []string{"html", "document", "image", "other"}},
				"response_ms":      map[string]any{"type": "integer"},
				"bytes_served":     map[string]any{"type": "integer"},
				"user_agent":       map[string]any{"type": "string"},
			},
		},
		"AIFetchIngestPayload": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":         map[string]any{"type": "string"},
				"hostname":     map[string]any{"type": "string"},
				"status_code":  map[string]any{"type": "integer", "minimum": 100, "maximum": 599},
				"content_type": map[string]any{"type": "string"},
				"response_ms":  map[string]any{"type": "integer", "minimum": 0},
				"bytes_served": map[string]any{"type": "integer", "minimum": 0},
				"user_agent":   map[string]any{"type": "string"},
			},
			"required": []string{"path", "status_code", "user_agent"},
		},
		"AIFetchOverview": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"total_requests":      map[string]any{"type": "integer"},
				"unique_paths":        map[string]any{"type": "integer"},
				"unique_assistants":   map[string]any{"type": "integer"},
				"error_rate_4xx":      map[string]any{"type": "number"},
				"error_rate_5xx":      map[string]any{"type": "number"},
				"median_response_ms":  map[string]any{"type": "integer"},
				"total_bytes":         map[string]any{"type": "integer"},
				"top_assistants":      map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_families":        map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_paths":           map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_error_paths":     map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"resource_type_split": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
			},
		},
		"AIFetchSeriesPoint": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"time":  map[string]any{"type": "string", "format": "date-time"},
				"count": map[string]any{"type": "integer"},
			},
		},
		"AIFetchCorrelationSummary": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"total_fetches":        map[string]any{"type": "integer"},
				"fetched_paths":        map[string]any{"type": "integer"},
				"correlated_paths":     map[string]any{"type": "integer"},
				"ai_referred_visits":   map[string]any{"type": "integer"},
				"uncorrelated_fetches": map[string]any{"type": "integer"},
			},
		},
		"AIFetchCitationYieldRow": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":               map[string]any{"type": "string"},
				"assistant_name":     map[string]any{"type": "string"},
				"fetch_count":        map[string]any{"type": "integer"},
				"ai_referred_visits": map[string]any{"type": "integer"},
				"citation_yield_pct": map[string]any{"type": "number"},
			},
		},
		"AIFetchOpportunityRow": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":               map[string]any{"type": "string"},
				"fetch_count":        map[string]any{"type": "integer"},
				"ai_referred_visits": map[string]any{"type": "integer"},
				"error_requests":     map[string]any{"type": "integer"},
				"error_rate_pct":     map[string]any{"type": "number"},
			},
		},
		"AIFetchFailureHotspot": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"assistant_name": map[string]any{"type": "string"},
				"path_prefix":    map[string]any{"type": "string"},
				"total_requests": map[string]any{"type": "integer"},
				"error_requests": map[string]any{"type": "integer"},
				"error_rate_pct": map[string]any{"type": "number"},
			},
		},
		"AIFetchCorrelationReport": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"summary":           map[string]any{"$ref": "#/components/schemas/AIFetchCorrelationSummary"},
				"citation_yield":    map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/AIFetchCitationYieldRow"}},
				"opportunity_pages": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/AIFetchOpportunityRow"}},
				"failure_hotspots":  map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/AIFetchFailureHotspot"}},
			},
		},
		"ChartDataPoint": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"time":      map[string]any{"type": "string", "format": "date-time"},
				"pageviews": map[string]any{"type": "integer"},
				"visitors":  map[string]any{"type": "integer"},
			},
		},
		"GoalStats": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"goal_id":         map[string]any{"type": "string", "format": "uuid"},
				"name":            map[string]any{"type": "string"},
				"conversions":     map[string]any{"type": "integer"},
				"conversion_rate": map[string]any{"type": "number"},
			},
		},
		"SiteStats": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"live_visitors":        map[string]any{"type": "integer"},
				"total_pageviews":      map[string]any{"type": "integer"},
				"unique_sessions":      map[string]any{"type": "integer"},
				"bounce_rate":          map[string]any{"type": "number"},
				"avg_session_duration": map[string]any{"type": "number"},
				"pages_per_session":    map[string]any{"type": "number"},
				"chart_data":           map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/ChartDataPoint"}},
				"top_pages":            map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_landing_pages":    map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_exit_pages":       map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_referrers":        map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_devices":          map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_countries":        map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_languages":        map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_ai_bots":          map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_ai_sources":       map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_utm_campaigns":    map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_utm_contents":     map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_utm_mediums":      map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_utm_sources":      map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_utm_terms":        map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"ai_bot_hits":          map[string]any{"type": "integer"},
				"ai_source_visits":     map[string]any{"type": "integer"},
				"utm_campaign_hits":    map[string]any{"type": "integer"},
				"utm_content_hits":     map[string]any{"type": "integer"},
				"utm_medium_hits":      map[string]any{"type": "integer"},
				"utm_source_hits":      map[string]any{"type": "integer"},
				"utm_term_hits":        map[string]any{"type": "integer"},
				"goals":                map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/GoalStats"}},
			},
		},
		"EcommerceSummary": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"revenue":                  map[string]any{"type": "number"},
				"orders":                   map[string]any{"type": "integer"},
				"average_order_value":      map[string]any{"type": "number"},
				"checkout_starts":          map[string]any{"type": "integer"},
				"checkout_conversion_rate": map[string]any{"type": "number"},
				"currency":                 map[string]any{"type": "string"},
			},
		},
		"EcommerceSeriesPoint": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"time":    map[string]any{"type": "string", "format": "date-time"},
				"revenue": map[string]any{"type": "number"},
				"orders":  map[string]any{"type": "integer"},
			},
		},
		"EcommerceProductStat": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"item_id":   map[string]any{"type": "string"},
				"item_name": map[string]any{"type": "string"},
				"revenue":   map[string]any{"type": "number"},
				"orders":    map[string]any{"type": "integer"},
				"quantity":  map[string]any{"type": "integer"},
			},
		},
		"EcommerceSourceStat": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"utm_source":   map[string]any{"type": "string"},
				"utm_medium":   map[string]any{"type": "string"},
				"utm_campaign": map[string]any{"type": "string"},
				"referrer":     map[string]any{"type": "string"},
				"revenue":      map[string]any{"type": "number"},
				"orders":       map[string]any{"type": "integer"},
			},
		},
		"Goal": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":         map[string]any{"type": "string", "format": "uuid"},
				"site_id":    map[string]any{"type": "string", "format": "uuid"},
				"name":       map[string]any{"type": "string"},
				"type":       map[string]any{"type": "string", "enum": []string{"event", "path"}},
				"value":      map[string]any{"type": "string"},
				"created_at": map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"GoalSeriesPoint": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"time":        map[string]any{"type": "string", "format": "date-time"},
				"conversions": map[string]any{"type": "integer"},
			},
		},
		"FunnelStep": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type":  map[string]any{"type": "string", "enum": []string{"event", "path"}},
				"value": map[string]any{"type": "string"},
			},
		},
		"Funnel": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":         map[string]any{"type": "string", "format": "uuid"},
				"site_id":    map[string]any{"type": "string", "format": "uuid"},
				"name":       map[string]any{"type": "string"},
				"steps":      map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/FunnelStep"}},
				"created_at": map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"FunnelSeriesPoint": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"time":        map[string]any{"type": "string", "format": "date-time"},
				"entries":     map[string]any{"type": "integer"},
				"completions": map[string]any{"type": "integer"},
			},
		},
		"FunnelStepStats": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"step_index":      map[string]any{"type": "integer"},
				"name":            map[string]any{"type": "string"},
				"visitors":        map[string]any{"type": "integer"},
				"dropoff":         map[string]any{"type": "integer"},
				"conversion_rate": map[string]any{"type": "number"},
			},
		},
		"FunnelStats": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"funnel_id":               map[string]any{"type": "string", "format": "uuid"},
				"name":                    map[string]any{"type": "string"},
				"steps":                   map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/FunnelStepStats"}},
				"total_entries":           map[string]any{"type": "integer"},
				"total_completions":       map[string]any{"type": "integer"},
				"overall_conversion_rate": map[string]any{"type": "number"},
			},
		},
	}
}
