package system

func openAPIV1WebhookSchemas() map[string]any {
	webhookProperties := map[string]any{
		"id":          map[string]any{"type": "string", "format": "uuid"},
		"site_id":     map[string]any{"type": []string{"string", "null"}, "format": "uuid"},
		"scope":       map[string]any{"type": "string", "enum": []string{"instance", "site"}},
		"name":        map[string]any{"type": "string"},
		"description": map[string]any{"type": "string"},
		"url":         map[string]any{"type": "string", "format": "uri"},
		"enabled":     map[string]any{"type": "boolean"},
		"events":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"created_at":  map[string]any{"type": "string", "format": "date-time"},
		"updated_at":  map[string]any{"type": "string", "format": "date-time"},
	}
	attempt := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":              map[string]any{"type": "string", "format": "uuid"},
			"attempt_number":  map[string]any{"type": "integer"},
			"status":          map[string]any{"type": "string"},
			"response_status": map[string]any{"type": "integer"},
			"error_code":      map[string]any{"type": "string"},
			"error_message":   map[string]any{"type": "string"},
			"started_at":      map[string]any{"type": "string", "format": "date-time"},
			"completed_at":    map[string]any{"type": "string", "format": "date-time"},
			"next_attempt_at": map[string]any{"type": []string{"string", "null"}, "format": "date-time"},
		},
	}
	return map[string]any{
		"WebhookEventDescriptor": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type":        map[string]any{"type": "string"},
				"site_scoped": map[string]any{"type": "boolean"},
				"scopes":      map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"instance", "site"}}},
			},
			"required": []string{"type", "site_scoped", "scopes"},
		},
		"WebhookEventPayload": map[string]any{
			"type":        "object",
			"description": "Exact JSON body signed and sent for one delivery. api_version is the running HitKeep major.minor and changes with the webhook contract for that release line.",
			"properties": map[string]any{
				"api_version": map[string]any{"type": "string", "pattern": `^\d+\.\d+$`, "examples": []string{"2.10"}},
				"id":          map[string]any{"type": "string", "format": "uuid", "description": "Stable event ID used for receiver deduplication."},
				"delivery_id": map[string]any{"type": "string", "format": "uuid", "description": "Stable ID for this endpoint delivery."},
				"type":        map[string]any{"type": "string"},
				"created_at":  map[string]any{"type": "string", "format": "date-time"},
				"data":        map[string]any{"type": "object", "additionalProperties": true},
			},
			"required": []string{"api_version", "id", "delivery_id", "type", "created_at", "data"},
		},
		"Webhook": map[string]any{
			"type":       "object",
			"properties": webhookProperties,
			"required":   []string{"id", "scope", "name", "description", "url", "enabled", "events", "created_at", "updated_at"},
		},
		"WebhookInput": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":        map[string]any{"type": "string", "maxLength": 120},
				"description": map[string]any{"type": "string", "maxLength": 500},
				"url":         map[string]any{"type": "string", "format": "uri", "description": "HTTPS destination. Loopback, private, link-local, multicast, and otherwise unsafe addresses are rejected unless development targets are explicitly enabled."},
				"enabled":     map[string]any{"type": "boolean"},
				"events":      map[string]any{"type": "array", "minItems": 1, "uniqueItems": true, "items": map[string]any{"type": "string"}},
			},
			"required": []string{"name", "url", "enabled", "events"},
		},
		"WebhookSecretResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"webhook": map[string]any{"$ref": "#/components/schemas/Webhook"},
				"secret":  map[string]any{"type": "string", "description": "One-time signing secret. It is returned only on creation or rotation."},
			},
			"required": []string{"webhook", "secret"},
		},
		"WebhookTestResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"event_id":     map[string]any{"type": "string", "format": "uuid"},
				"delivery_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string", "format": "uuid"}},
			},
			"required": []string{"event_id", "delivery_ids"},
		},
		"WebhookDeliveryAttempt": attempt,
		"WebhookDelivery": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":                 map[string]any{"type": "string", "format": "uuid"},
				"event_id":           map[string]any{"type": "string", "format": "uuid"},
				"webhook_id":         map[string]any{"type": "string", "format": "uuid"},
				"site_id":            map[string]any{"type": []string{"string", "null"}, "format": "uuid"},
				"event_type":         map[string]any{"type": "string"},
				"status":             map[string]any{"type": "string", "enum": []string{"pending", "processing", "retrying", "succeeded", "failed"}},
				"attempt_count":      map[string]any{"type": "integer"},
				"next_attempt_at":    map[string]any{"type": []string{"string", "null"}, "format": "date-time"},
				"last_attempt_at":    map[string]any{"type": []string{"string", "null"}, "format": "date-time"},
				"completed_at":       map[string]any{"type": []string{"string", "null"}, "format": "date-time"},
				"response_status":    map[string]any{"type": "integer"},
				"last_error_code":    map[string]any{"type": "string"},
				"last_error_message": map[string]any{"type": "string"},
				"created_at":         map[string]any{"type": "string", "format": "date-time"},
				"attempts":           map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/WebhookDeliveryAttempt"}},
			},
			"required": []string{"id", "event_id", "webhook_id", "event_type", "status", "attempt_count", "created_at", "attempts"},
		},
	}
}
