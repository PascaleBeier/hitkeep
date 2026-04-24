package system

func openAPIV1Components() map[string]any {
	return map[string]any{
		"securitySchemes": openAPIV1SecuritySchemes(),
		"parameters":      openAPIV1Parameters(),
		"schemas":         openAPIV1Schemas(),
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
