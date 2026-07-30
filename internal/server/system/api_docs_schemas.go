package system

var opportunityEvidenceOpenAPISchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"id":            map[string]any{"type": "string"},
		"label_key":     map[string]any{"type": "string"},
		"value":         map[string]any{"type": "string"},
		"detail_key":    map[string]any{"type": "string"},
		"detail_params": map[string]any{"type": "object", "additionalProperties": true},
	},
	"required": []string{"id", "label_key", "value"},
}

var opportunityScoreBreakdownOpenAPISchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"sample":        map[string]any{"type": "integer"},
		"impact":        map[string]any{"type": "integer"},
		"urgency":       map[string]any{"type": "integer"},
		"effort":        map[string]any{"type": "integer"},
		"actionability": map[string]any{"type": "integer"},
		"evidence_fit":  map[string]any{"type": "integer"},
		"freshness":     map[string]any{"type": "integer"},
		"total":         map[string]any{"type": "integer"},
	},
	"required": []string{"sample", "impact", "urgency", "effort", "actionability", "evidence_fit", "freshness", "total"},
}

var opportunityDigestPreviewItemOpenAPISchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"id":                 map[string]any{"type": "string", "format": "uuid"},
		"site_id":            map[string]any{"type": "string", "format": "uuid"},
		"kind":               map[string]any{"type": "string", "enum": []string{"conversion", "traffic", "ai", "search", "setup"}},
		"type_key":           map[string]any{"type": "string"},
		"category":           map[string]any{"type": "string"},
		"title_key":          map[string]any{"type": "string"},
		"action_key":         map[string]any{"type": "string"},
		"digest_key":         map[string]any{"type": "string"},
		"copy_params":        map[string]any{"type": "object", "additionalProperties": true},
		"impact_value":       map[string]any{"type": "string"},
		"impact_label_key":   map[string]any{"type": "string"},
		"confidence":         map[string]any{"type": "string", "enum": []string{"high", "medium"}},
		"score":              map[string]any{"type": "integer"},
		"score_breakdown":    map[string]any{"$ref": "#/components/schemas/OpportunityScoreBreakdown"},
		"status":             map[string]any{"type": "string", "enum": []string{"new", "saved"}},
		"route_label_key":    map[string]any{"type": "string"},
		"route_params":       map[string]any{"type": "object", "additionalProperties": true},
		"route_icon":         map[string]any{"type": "string"},
		"evidence":           map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/OpportunityEvidence"}},
		"cited_evidence_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	},
	"required": []string{"id", "site_id", "kind", "type_key", "category", "title_key", "action_key", "digest_key", "copy_params", "impact_value", "impact_label_key", "confidence", "score", "score_breakdown", "status", "route_label_key", "route_icon", "evidence", "cited_evidence_ids"},
}

func openAPIV1Schemas() map[string]any {
	return mergeOpenAPIMapGroups(
		openAPIV1AnalyticsSchemas(),
		openAPIV1WebVitalSchemas(),
		openAPIV1QRSchemas(),
		openAPIV1AccountSchemas(),
		openAPIV1WebhookSchemas(),
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
		"DatabaseUnavailable": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{
					"type": "string",
					"enum": []string{"error"},
				},
				"code": map[string]any{
					"type": "string",
					"enum": []string{
						"database_unavailable",
						"database_recovering",
						"database_needs_attention",
					},
				},
				"message": map[string]any{"type": "string"},
				"retry_after_seconds": map[string]any{
					"type":    "integer",
					"minimum": 1,
				},
			},
			"required": []string{"status", "code", "message", "retry_after_seconds"},
		},
		"ReadinessUnavailable": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{
					"type": "string",
					"enum": []string{"not_ready"},
				},
				"reason": map[string]any{
					"type": "string",
					"enum": []string{
						"not_leader",
						"database_unavailable",
						"database_recovering",
						"database_needs_attention",
					},
				},
				"retry_after_seconds": map[string]any{
					"type":    "integer",
					"minimum": 1,
				},
			},
			"required": []string{"status", "reason", "retry_after_seconds"},
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
		"SiteStatsResetRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"confirm_domain": map[string]any{"type": "string"},
			},
			"required": []string{"confirm_domain"},
		},
		"SiteStatsResetResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":                 map[string]any{"type": "string", "enum": []string{"reset"}},
				"rows_cleared":           map[string]any{"type": "integer", "format": "int64"},
				"imports_marked_deleted": map[string]any{"type": "integer", "format": "int64"},
				"families_cleared":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"status", "rows_cleared", "imports_marked_deleted", "families_cleared"},
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
		"CustomTrackingDomain": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":                  map[string]any{"type": "string", "format": "uuid"},
				"team_id":             map[string]any{"type": "string", "format": "uuid"},
				"hostname":            map[string]any{"type": "string"},
				"verification_status": map[string]any{"type": "string", "enum": []string{"pending", "verified", "failed"}},
				"target_status":       map[string]any{"type": "string", "enum": []string{"pending", "verified", "failed"}},
				"tls_mode":            map[string]any{"type": "string", "enum": []string{"external", "caddy-on-demand"}},
				"tls_status":          map[string]any{"type": "string", "enum": []string{"pending", "verified", "failed"}},
				"enabled":             map[string]any{"type": "boolean"},
				"active":              map[string]any{"type": "boolean", "description": "True when the domain is enabled and all verification states are verified."},
				"dns_txt_name":        map[string]any{"type": "string"},
				"dns_txt_value":       map[string]any{"type": "string"},
				"dns_target":          map[string]any{"type": "string"},
				"last_error":          map[string]any{"type": "string"},
				"verified_at":         map[string]any{"type": "string", "format": "date-time"},
				"last_checked_at":     map[string]any{"type": "string", "format": "date-time"},
				"last_tls_ask_at":     map[string]any{"type": "string", "format": "date-time"},
				"created_at":          map[string]any{"type": "string", "format": "date-time"},
				"updated_at":          map[string]any{"type": "string", "format": "date-time"},
			},
			"required": []string{"id", "team_id", "hostname", "verification_status", "target_status", "tls_mode", "tls_status", "enabled", "active", "dns_txt_name", "dns_txt_value", "dns_target", "created_at", "updated_at"},
		},
		"CreateCustomTrackingDomainRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"hostname": map[string]any{"type": "string"},
			},
			"required": []string{"hostname"},
		},
		"UpdateCustomTrackingDomainRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"enabled": map[string]any{"type": "boolean"},
			},
			"required": []string{"enabled"},
		},
		"SiteTrackingDomainOptions": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"site_id":     map[string]any{"type": "string", "format": "uuid"},
				"team_id":     map[string]any{"type": "string", "format": "uuid"},
				"default_url": map[string]any{"type": "string"},
				"domains":     map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/CustomTrackingDomain"}},
			},
			"required": []string{"site_id", "team_id", "default_url", "domains"},
		},
		"SiteSetupState": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"has_ai_fetches":       map[string]any{"type": "boolean"},
				"has_chatbot_events":   map[string]any{"type": "boolean"},
				"has_custom_events":    map[string]any{"type": "boolean"},
				"has_ecommerce_events": map[string]any{"type": "boolean"},
				"has_web_vitals":       map[string]any{"type": "boolean"},
			},
			"required": []string{"has_ai_fetches", "has_chatbot_events", "has_custom_events", "has_ecommerce_events", "has_web_vitals"},
		},
		"SiteTrackingStatus": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"site_id":                   map[string]any{"type": "string", "format": "uuid"},
				"tenant_id":                 map[string]any{"type": "string", "format": "uuid"},
				"status":                    map[string]any{"type": "string", "enum": []string{"waiting", "live", "dormant", "domain_mismatch"}},
				"first_hit_at":              map[string]any{"type": "string", "format": "date-time"},
				"last_hit_at":               map[string]any{"type": "string", "format": "date-time"},
				"last_event_at":             map[string]any{"type": "string", "format": "date-time"},
				"last_hostname":             map[string]any{"type": "string"},
				"last_event_name":           map[string]any{"type": "string"},
				"last_automatic_event_at":   map[string]any{"type": "string", "format": "date-time"},
				"last_automatic_event_name": map[string]any{"type": "string"},
				"tracker_source":            map[string]any{"type": "string"},
				"tracker_version":           map[string]any{"type": "string"},
				"configured_domain":         map[string]any{"type": "string"},
				"updated_at":                map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"OpportunityEvidence":       opportunityEvidenceOpenAPISchema,
		"OpportunityScoreBreakdown": opportunityScoreBreakdownOpenAPISchema,
		"Opportunity":               opportunityPublicSchema(),
		"OpportunityListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"opportunities": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Opportunity"}},
			},
			"required": []string{"opportunities"},
		},
		"SharedOpportunity": opportunityPublicSchema(),
		"SharedOpportunityListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"opportunities": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/SharedOpportunity"}},
			},
			"required": []string{"opportunities"},
		},
		"OpportunityGenerateResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"opportunities": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Opportunity"}},
				"ai_status":     map[string]any{"type": "string"},
			},
			"required": []string{"opportunities", "ai_status"},
		},
		"OpportunityDigestPreviewItem": opportunityDigestPreviewItemOpenAPISchema,
		"OpportunityDigestPreviewResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"frequency":   map[string]any{"type": "string", "enum": []string{"daily", "weekly"}},
				"should_send": map[string]any{"type": "boolean"},
				"reason":      map[string]any{"type": "string", "enum": []string{"ready", "no_opportunities", "unsupported_frequency"}},
				"items":       map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/OpportunityDigestPreviewItem"}},
			},
			"required": []string{"frequency", "should_send", "reason", "items"},
		},
		"OpportunityStatusUpdateRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{"type": "string", "enum": []string{"new", "saved", "done", "dismissed"}},
			},
			"required": []string{"status"},
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
				"region":          map[string]any{"type": "string"},
				"city":            map[string]any{"type": "string"},
				"provider":        map[string]any{"type": "string"},
				"asn":             map[string]any{"type": "integer"},
				"asn_org":         map[string]any{"type": "string"},
				"utm_source":      map[string]any{"type": "string"},
				"utm_medium":      map[string]any{"type": "string"},
				"utm_campaign":    map[string]any{"type": "string"},
				"utm_term":        map[string]any{"type": "string"},
				"utm_content":     map[string]any{"type": "string"},
				"qr_code_id":      map[string]any{"type": "string", "format": "uuid"},
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
		"ImportExclusionReason": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"reason": map[string]any{"type": "string"},
				"detail": map[string]any{"type": "string"},
			},
		},
		"ImportEventCoverage": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"rows_scanned":  map[string]any{"type": "integer"},
				"rows_accepted": map[string]any{"type": "integer"},
				"events":        map[string]any{"type": "integer", "description": "Queryable imported custom-event count."},
				"visitors":      map[string]any{"type": "integer"},
				"event_names":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"property_keys": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		},
		"ImportEventPropertyCoverage": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"attributed_rows":                  map[string]any{"type": "integer"},
				"attributed_events":                map[string]any{"type": "integer"},
				"attributed_visitors":              map[string]any{"type": "integer"},
				"attributed_property_keys":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"unattributed_rows":                map[string]any{"type": "integer"},
				"unattributed_events":              map[string]any{"type": "integer"},
				"unattributed_visitors":            map[string]any{"type": "integer"},
				"unattributed_property_keys":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"unattributed_relationship":        map[string]any{"type": "string"},
				"unavailable_relationship_message": map[string]any{"type": "string"},
			},
		},
		"ImportEventDimensionCoverage": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"available":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"unavailable": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"reason":      map[string]any{"type": "string"},
			},
		},
		"ImportOverlapSummary": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"policy":                      map[string]any{"type": "string", "enum": []string{"skip_native_day"}},
				"native_traffic_days":         map[string]any{"type": "integer"},
				"native_event_days":           map[string]any{"type": "integer"},
				"native_event_keys":           map[string]any{"type": "integer"},
				"estimated_skipped_rows":      map[string]any{"type": "integer"},
				"estimated_skipped_pageviews": map[string]any{"type": "integer"},
				"estimated_skipped_events":    map[string]any{"type": "integer"},
			},
		},
		"ImportDatasetSummary": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key":           map[string]any{"type": "string"},
				"name":          map[string]any{"type": "string"},
				"files":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"rows_scanned":  map[string]any{"type": "integer"},
				"rows_accepted": map[string]any{"type": "integer"},
				"rows_skipped":  map[string]any{"type": "integer"},
				"visitors":      map[string]any{"type": "integer"},
				"visits":        map[string]any{"type": "integer"},
				"pageviews":     map[string]any{"type": "integer"},
				"events":        map[string]any{"type": "integer"},
			},
		},
		"ImportWarning": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code":    map[string]any{"type": "string"},
				"message": map[string]any{"type": "string"},
				"file":    map[string]any{"type": "string"},
			},
		},
		"ImportManifest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"provider":                 map[string]any{"type": "string"},
				"source_hash":              map[string]any{"type": "string"},
				"date_start":               map[string]any{"type": "string", "format": "date-time"},
				"date_end":                 map[string]any{"type": "string", "format": "date-time"},
				"files":                    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"ignored_files":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"missing_files":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"datasets":                 map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/ImportDatasetSummary"}},
				"event_coverage":           map[string]any{"$ref": "#/components/schemas/ImportEventCoverage"},
				"event_property_coverage":  map[string]any{"$ref": "#/components/schemas/ImportEventPropertyCoverage"},
				"event_dimension_coverage": map[string]any{"$ref": "#/components/schemas/ImportEventDimensionCoverage"},
				"overlap":                  map[string]any{"$ref": "#/components/schemas/ImportOverlapSummary"},
				"warnings":                 map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/ImportWarning"}},
				"rows_scanned":             map[string]any{"type": "integer"},
				"rows_accepted":            map[string]any{"type": "integer"},
				"rows_skipped":             map[string]any{"type": "integer"},
			},
		},
		"ImportJob": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":             map[string]any{"type": "string", "format": "uuid"},
				"site_id":        map[string]any{"type": "string", "format": "uuid"},
				"provider":       map[string]any{"type": "string"},
				"status":         map[string]any{"type": "string"},
				"source_hash":    map[string]any{"type": "string"},
				"bytes_total":    map[string]any{"type": "integer"},
				"bytes_received": map[string]any{"type": "integer"},
				"rows_scanned":   map[string]any{"type": "integer"},
				"rows_imported":  map[string]any{"type": "integer"},
				"error":          map[string]any{"type": "string"},
				"manifest":       map[string]any{"$ref": "#/components/schemas/ImportManifest"},
			},
		},
		"EventSeriesPoint": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"time":  map[string]any{"type": "string", "format": "date-time"},
				"count": map[string]any{"type": "integer"},
			},
		},
		"EventAudience": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"top_pages":     map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_referrers": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_devices":   map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_countries": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_cities":    map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_providers": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_asns":      map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"imported_excluded": map[string]any{
					"type":  "array",
					"items": map[string]any{"$ref": "#/components/schemas/ImportExclusionReason"},
				},
			},
		},
		"AIFetch": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":                 map[string]any{"type": "string", "format": "uuid"},
				"site_id":            map[string]any{"type": "string", "format": "uuid"},
				"timestamp":          map[string]any{"type": "string", "format": "date-time"},
				"assistant_name":     map[string]any{"type": "string"},
				"assistant_family":   map[string]any{"type": "string"},
				"assistant_category": map[string]any{"type": "string", "enum": []string{"ai_training_crawler", "ai_search_indexer", "ai_assistant", "ai_agent", "ai_coding_agent", "other_ai"}},
				"path":               map[string]any{"type": "string"},
				"hostname":           map[string]any{"type": "string"},
				"status_code":        map[string]any{"type": "integer"},
				"content_type":       map[string]any{"type": "string"},
				"resource_type":      map[string]any{"type": "string", "enum": []string{"html", "document", "image", "other"}},
				"response_ms":        map[string]any{"type": "integer"},
				"bytes_served":       map[string]any{"type": "integer"},
				"user_agent":         map[string]any{"type": "string"},
			},
		},
		"AIAgentCatalog": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"generated_at": map[string]any{"type": "string", "format": "date-time"},
				"agents": map[string]any{"type": "array", "items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":      map[string]any{"type": "string"},
						"family":    map[string]any{"type": "string"},
						"category":  map[string]any{"type": "string", "enum": []string{"ai_training_crawler", "ai_search_indexer", "ai_assistant", "ai_agent", "ai_coding_agent", "other_ai"}},
						"icon_host": map[string]any{"type": "string"},
					},
				}},
				"ai_referrers": map[string]any{"type": "array", "items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":      map[string]any{"type": "string"},
						"icon_host": map[string]any{"type": "string"},
					},
				}},
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
		"AIActivityStat": map[string]any{
			"type":        "object",
			"description": "One row of a merged AI activity top list. value is always tracked_hits + fetch_count. Dimensions only one source carries report zero on the other side: families and resource types exist only in server-log fetch records, referral sources only in tracked hits.",
			"properties": map[string]any{
				"name":         map[string]any{"type": "string"},
				"value":        map[string]any{"type": "integer"},
				"tracked_hits": map[string]any{"type": "integer"},
				"fetch_count":  map[string]any{"type": "integer"},
			},
		},
		"AIActivitySeriesPoint": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"time":            map[string]any{"type": "string", "format": "date-time"},
				"ai_requests":     map[string]any{"type": "integer", "description": "Tracked AI bot hits plus AI fetch records in the bucket."},
				"tracked_hits":    map[string]any{"type": "integer"},
				"fetch_count":     map[string]any{"type": "integer"},
				"referral_visits": map[string]any{"type": "integer", "description": "Distinct sessions referred by a known AI surface inside the bucket. Buckets are not additive: a session spanning two buckets counts in both."},
			},
		},
		"AIActivityReport": map[string]any{
			"type":        "object",
			"description": "Unified AI activity report merging tracked AI hits with server-log AI fetch records. Every count follows one rule: tracked hits plus fetch records. Depth metrics describe the fetch side only, because tracked hits carry no status code, response time, or transferred bytes.",
			"properties": map[string]any{
				"ai_requests":            map[string]any{"type": "integer"},
				"tracked_hits":           map[string]any{"type": "integer"},
				"fetch_count":            map[string]any{"type": "integer"},
				"referral_visits":        map[string]any{"type": "integer", "description": "Distinct sessions referred by a known AI surface across the whole range."},
				"paths_crawled":          map[string]any{"type": "integer", "description": "Distinct paths seen on either side, counted before the top-list cap."},
				"unique_agents":          map[string]any{"type": "integer"},
				"pageviews":              map[string]any{"type": "integer", "description": "Total hits for the range under the non-AI filters only, so the AI share of a segment stays comparable while an AI dimension is drilled into."},
				"error_rate_4xx":         map[string]any{"type": "number"},
				"error_rate_5xx":         map[string]any{"type": "number"},
				"median_response_ms":     map[string]any{"type": "integer"},
				"total_bytes":            map[string]any{"type": "integer"},
				"top_agents":             map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/AIActivityStat"}},
				"top_categories":         map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/AIActivityStat"}},
				"top_paths":              map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/AIActivityStat"}},
				"top_sources":            map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/AIActivityStat"}},
				"top_families":           map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/AIActivityStat"}},
				"top_resource_types":     map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/AIActivityStat"}},
				"top_error_paths":        map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/AIActivityStat"}},
				"top_agents_by_category": map[string]any{"type": "object", "description": "Merged agent top list per AI agent category, keyed by category.", "additionalProperties": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/AIActivityStat"}}},
				"series":                 map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/AIActivitySeriesPoint"}},
				"comparison": map[string]any{
					"type":        "object",
					"description": "Merged counters for the compare_from/compare_to window. Absent unless a comparison range is requested.",
					"properties": map[string]any{
						"ai_requests":     map[string]any{"type": "integer"},
						"tracked_hits":    map[string]any{"type": "integer"},
						"fetch_count":     map[string]any{"type": "integer"},
						"referral_visits": map[string]any{"type": "integer"},
						"paths_crawled":   map[string]any{"type": "integer"},
						"unique_agents":   map[string]any{"type": "integer"},
						"pageviews":       map[string]any{"type": "integer"},
					},
				},
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
		"AIFetchCorrelationPathRow": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":               map[string]any{"type": "string"},
				"fetch_count":        map[string]any{"type": "integer"},
				"ai_referred_visits": map[string]any{"type": "integer"},
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
				"citation_paths":    map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/AIFetchCorrelationPathRow"}},
				"opportunity_pages": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/AIFetchOpportunityRow"}},
				"failure_hotspots":  map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/AIFetchFailureHotspot"}},
			},
		},
		"SearchConsoleOverview": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data_source":      map[string]any{"type": "string", "const": "google_search_console"},
				"clicks":           map[string]any{"type": "integer"},
				"impressions":      map[string]any{"type": "integer"},
				"ctr":              map[string]any{"type": "number"},
				"average_position": map[string]any{"type": "number"},
			},
			"required": []string{"data_source", "clicks", "impressions", "ctr", "average_position"},
		},
		"SearchConsoleMetricPoint": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date":             map[string]any{"type": "string", "format": "date"},
				"clicks":           map[string]any{"type": "integer"},
				"impressions":      map[string]any{"type": "integer"},
				"ctr":              map[string]any{"type": "number"},
				"average_position": map[string]any{"type": "number"},
			},
			"required": []string{"date", "clicks", "impressions", "ctr", "average_position"},
		},
		"SearchConsoleSeriesResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data_source": map[string]any{"type": "string", "const": "google_search_console"},
				"series":      map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/SearchConsoleMetricPoint"}},
			},
			"required": []string{"data_source", "series"},
		},
		"SearchConsoleDimensionRow": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"value":            map[string]any{"type": "string"},
				"clicks":           map[string]any{"type": "integer"},
				"impressions":      map[string]any{"type": "integer"},
				"ctr":              map[string]any{"type": "number"},
				"average_position": map[string]any{"type": "number"},
			},
			"required": []string{"value", "clicks", "impressions", "ctr", "average_position"},
		},
		"SearchConsoleDimensionResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data_source": map[string]any{"type": "string", "const": "google_search_console"},
				"dimension":   map[string]any{"type": "string", "enum": []string{"query", "page", "country", "device"}},
				"rows":        map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/SearchConsoleDimensionRow"}},
			},
			"required": []string{"data_source", "dimension", "rows"},
		},
		"ChartDataPoint": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"time":      map[string]any{"type": "string", "format": "date-time"},
				"pageviews": map[string]any{"type": "integer"},
				"visitors":  map[string]any{"type": "integer"},
			},
		},
		"SiteOverviewStats": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"site_id":         map[string]any{"type": "string", "format": "uuid"},
				"status":          map[string]any{"type": "string", "enum": []string{"ready", "error"}},
				"total_pageviews": map[string]any{"type": "integer"},
				"unique_sessions": map[string]any{"type": "integer"},
				"bounce_rate":     map[string]any{"type": "number"},
				"chart_data":      map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/ChartDataPoint"}},
				"error":           map[string]any{"type": "string"},
			},
			"required": []string{"site_id", "status", "total_pageviews", "unique_sessions", "bounce_rate", "chart_data"},
		},
		"SitesOverviewStatsResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sites": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/SiteOverviewStats"}},
			},
			"required": []string{"sites"},
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
				"live_visitors":           map[string]any{"type": "integer"},
				"total_pageviews":         map[string]any{"type": "integer"},
				"unique_sessions":         map[string]any{"type": "integer"},
				"bounce_rate":             map[string]any{"type": "number"},
				"avg_session_duration":    map[string]any{"type": "number"},
				"pages_per_session":       map[string]any{"type": "number"},
				"chart_data":              map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/ChartDataPoint"}},
				"top_pages":               map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_landing_pages":       map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_exit_pages":          map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_referrers":           map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_devices":             map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_countries":           map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_cities":              map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_providers":           map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_asns":                map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_languages":           map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_ai_bots":             map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_ai_bot_categories":   map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_ai_bots_by_category": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}}},
				"top_ai_sources":          map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_utm_campaigns":       map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_utm_contents":        map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_utm_mediums":         map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_utm_sources":         map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_utm_terms":           map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"ai_bot_hits":             map[string]any{"type": "integer"},
				"ai_source_visits":        map[string]any{"type": "integer"},
				"utm_campaign_hits":       map[string]any{"type": "integer"},
				"utm_content_hits":        map[string]any{"type": "integer"},
				"utm_medium_hits":         map[string]any{"type": "integer"},
				"utm_source_hits":         map[string]any{"type": "integer"},
				"utm_term_hits":           map[string]any{"type": "integer"},
				"goals":                   map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/GoalStats"}},
				"funnels":                 map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Funnel"}},
				"imported_excluded": map[string]any{
					"type":  "array",
					"items": map[string]any{"$ref": "#/components/schemas/ImportExclusionReason"},
				},
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
				"top_cities":               map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_providers":            map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_asns":                 map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
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

func opportunityPublicSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":                 map[string]any{"type": "string", "format": "uuid"},
			"site_id":            map[string]any{"type": "string", "format": "uuid"},
			"kind":               map[string]any{"type": "string", "enum": []string{"conversion", "traffic", "ai", "search", "setup"}},
			"type_key":           map[string]any{"type": "string"},
			"title_key":          map[string]any{"type": "string"},
			"summary_key":        map[string]any{"type": "string"},
			"action_key":         map[string]any{"type": "string"},
			"digest_key":         map[string]any{"type": "string"},
			"copy_params":        map[string]any{"type": "object", "additionalProperties": true},
			"impact_value":       map[string]any{"type": "string"},
			"impact_label_key":   map[string]any{"type": "string"},
			"confidence":         map[string]any{"type": "string", "enum": []string{"high", "medium"}},
			"score":              map[string]any{"type": "integer"},
			"score_breakdown":    map[string]any{"$ref": "#/components/schemas/OpportunityScoreBreakdown"},
			"status":             map[string]any{"type": "string", "enum": []string{"new", "saved", "done", "dismissed"}},
			"route_label_key":    map[string]any{"type": "string"},
			"route_params":       map[string]any{"type": "object", "additionalProperties": true},
			"route_icon":         map[string]any{"type": "string"},
			"detector_version":   map[string]any{"type": "string"},
			"evidence":           map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/OpportunityEvidence"}},
			"cited_evidence_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"generated_at":       map[string]any{"type": "string", "format": "date-time"},
			"created_at":         map[string]any{"type": "string", "format": "date-time"},
			"updated_at":         map[string]any{"type": "string", "format": "date-time"},
		},
		"required": []string{"id", "site_id", "kind", "type_key", "title_key", "summary_key", "action_key", "digest_key", "copy_params", "impact_value", "impact_label_key", "confidence", "score", "score_breakdown", "status", "route_label_key", "route_icon", "detector_version", "evidence", "cited_evidence_ids", "generated_at", "created_at", "updated_at"},
	}
}

func googleSearchConsoleStatusSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status":                   map[string]any{"type": "string", "enum": []string{"connected", "disconnected", "credentials_missing"}},
			"configured":               map[string]any{"type": "boolean"},
			"connected":                map[string]any{"type": "boolean"},
			"credential_status":        map[string]any{"type": "string", "enum": []string{"configured", "missing"}},
			"connected_account_label":  map[string]any{"type": "string"},
			"last_connected_at":        map[string]any{"type": "string", "format": "date-time"},
			"last_disconnected_at":     map[string]any{"type": "string", "format": "date-time"},
			"needs_admin_action":       map[string]any{"type": "boolean"},
			"can_manage":               map[string]any{"type": "boolean"},
			"managed_credentials_mode": map[string]any{"type": "string", "enum": []string{"managed", "self_hosted"}},
		},
		"required": []string{"status", "configured", "connected", "credential_status", "needs_admin_action", "can_manage", "managed_credentials_mode"},
	}
}

func googleSearchConsolePropertiesSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"properties": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"uri":              map[string]any{"type": "string"},
						"permission_level": map[string]any{"type": "string"},
					},
					"required": []string{"uri", "permission_level"},
				},
			},
		},
		"required": []string{"properties"},
	}
}

func googleSearchConsoleSiteMappingSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"site_id":                   map[string]any{"type": "string", "format": "uuid"},
			"team_id":                   map[string]any{"type": "string", "format": "uuid"},
			"mapped":                    map[string]any{"type": "boolean"},
			"property_uri":              map[string]any{"type": "string"},
			"property_permission_level": map[string]any{"type": "string"},
			"mapped_at":                 map[string]any{"type": "string", "format": "date-time"},
			"can_manage":                map[string]any{"type": "boolean"},
			"sync_status":               googleSearchConsoleSyncStatusSchema(),
		},
		"required": []string{"site_id", "team_id", "mapped", "can_manage"},
	}
}

func googleSearchConsoleSyncStatusSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"state":               map[string]any{"type": "string", "enum": []string{"pending", "running", "succeeded", "failed", "needs_attention"}},
			"imported_start_date": map[string]any{"type": "string", "format": "date"},
			"imported_end_date":   map[string]any{"type": "string", "format": "date"},
			"last_success_at":     map[string]any{"type": "string", "format": "date-time"},
			"last_attempt_at":     map[string]any{"type": "string", "format": "date-time"},
			"last_error_category": map[string]any{"type": "string"},
			"last_error_message":  map[string]any{"type": "string", "description": "Full error response returned by Google for the latest failed sync."},
			"next_retry_at":       map[string]any{"type": "string", "format": "date-time"},
			"manual":              map[string]any{"type": "boolean"},
		},
		"required": []string{"state", "manual"},
	}
}
