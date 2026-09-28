package system

func openAPIV1Components() map[string]any {
	return map[string]any{
		"securitySchemes": openAPIV1SecuritySchemes(),
		"parameters":      openAPIV1Parameters(),
		"responses":       openAPIV1Responses(),
		"schemas":         openAPIV1Schemas(),
	}
}

func openAPIV1Responses() map[string]any {
	return map[string]any{
		"DatabaseUnavailable": map[string]any{
			"description": "The shared database or an open tenant database is recovering, requires operator attention, or is unavailable.",
			"headers": map[string]any{
				"Retry-After": map[string]any{
					"description": "Seconds before retrying the request.",
					"schema":      map[string]any{"type": "integer", "minimum": 1},
				},
			},
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"$ref": "#/components/schemas/DatabaseUnavailable"},
				},
			},
		},
	}
}

func openAPIV1SecuritySchemes() map[string]any {
	return map[string]any{
		"cookieAuth": map[string]any{
			"type":        "apiKey",
			"in":          "cookie",
			"name":        "hk_token",
			"description": "Session cookie authentication.",
		},
		"bearerAuth": map[string]any{
			"type":         "http",
			"scheme":       "bearer",
			"bearerFormat": "APIClientToken",
			"description":  "API client token in Authorization header.",
		},
		"apiKeyAuth": map[string]any{
			"type":        "apiKey",
			"in":          "header",
			"name":        "X-API-Key",
			"description": "API client token in X-API-Key header.",
		},
	}
}
