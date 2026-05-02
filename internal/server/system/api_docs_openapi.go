package system

func OpenAPISpecV1(publicURL string) map[string]any {
	return openAPISpecV1(publicURL)
}

func openAPISpecV1(publicURL string) map[string]any {
	return map[string]any{
		"openapi":           "3.1.0",
		"jsonSchemaDialect": "https://spec.openapis.org/oas/3.1/dialect/base",
		"info": map[string]any{
			"title":       "HitKeep REST API",
			"version":     "v1",
			"description": "Complete HTTP API for HitKeep (session, API key, and public share-token endpoints).",
		},
		"servers": []map[string]string{{"url": publicURL}},
		"tags": []map[string]string{
			{"name": "System", "description": "Service health and API documentation endpoints."},
			{"name": "Ingest", "description": "Public tracking and event ingestion endpoints."},
			{"name": "Auth", "description": "Authentication, password, and sign-in flows."},
			{"name": "Cloud", "description": "Managed HitKeep Cloud endpoints. Available only in hosted cloud builds with billing enabled."},
			{"name": "User", "description": "Authenticated user profile, preferences, and security endpoints."},
			{"name": "Permissions", "description": "Authenticated permission context endpoints."},
			{"name": "Admin", "description": "Instance-level admin and membership management endpoints."},
			{"name": "Sites", "description": "Site lifecycle, stats, hits, and retention endpoints."},
			{"name": "Imports", "description": "Historical analytics import validation, upload, and lifecycle endpoints."},
			{"name": "Goals", "description": "Goal and goal-timeseries endpoints."},
			{"name": "Funnels", "description": "Funnel CRUD and analytics endpoints."},
			{"name": "Share", "description": "Share-link management and public shared analytics endpoints."},
			{"name": "Takeout", "description": "Data export endpoints for user and site data."},
			{"name": "Reports", "description": "Report subscription endpoints for digest and per-site scheduled analytics emails."},
			{"name": "Teams", "description": "Tenant team membership and active-team context endpoints."},
		},
		"components": openAPIV1Components(),
		"paths":      openAPIV1Paths(),
	}
}
