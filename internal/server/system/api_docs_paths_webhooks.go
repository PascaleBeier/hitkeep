package system

func webhookIDParam() map[string]any {
	return map[string]any{
		"name": "webhookID", "in": "path", "required": true,
		"schema": map[string]any{"type": "string", "format": "uuid"},
	}
}

func openAPIV1WebhookPaths() map[string]any {
	paths := map[string]any{}
	addWebhookScopePaths(paths, "/api/admin/webhooks", nil, "instance")
	addWebhookScopePaths(paths, "/api/sites/{id}/webhooks", []any{paramRef("#/components/parameters/siteID")}, "site")
	return paths
}

func addWebhookScopePaths(paths map[string]any, base string, scopeParams []any, scope string) {
	itemParams := append(append([]any{}, scopeParams...), webhookIDParam())
	signing := "Deliveries use the WebhookEventPayload schema with at-least-once, unordered semantics. Verify X-HitKeep-Signature (v1=<hex HMAC-SHA256>) over timestamp + \".\" + body with the one-time secret, reject stale X-HitKeep-Timestamp values, and deduplicate by X-HitKeep-Event-ID or X-HitKeep-Delivery-ID."
	paths[base+"/events"] = map[string]any{
		"get": op([]string{"Webhooks"}, "List "+scope+" webhook events", "Lists event types valid for this scope.", secCookie(), scopeParams, nil,
			map[string]any{"200": jsonSchemaResp("Event catalog", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/WebhookEventDescriptor"}})}),
	}
	paths[base] = map[string]any{
		"get": op([]string{"Webhooks"}, "List "+scope+" webhooks", "Human dashboard session only.", secCookie(), scopeParams, nil,
			map[string]any{"200": jsonSchemaResp("Webhook list", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Webhook"}})}),
		"post": op([]string{"Webhooks"}, "Create "+scope+" webhook", "Creates a webhook and returns its signing secret exactly once. Human dashboard session only.", secCookie(), scopeParams,
			jsonBody(map[string]any{"$ref": "#/components/schemas/WebhookInput"}),
			map[string]any{"201": jsonRefResp("Created webhook and one-time secret", "#/components/schemas/WebhookSecretResponse"), "400": errResp("Invalid event or destination")}),
	}
	paths[base+"/{webhookID}"] = map[string]any{
		"put": op([]string{"Webhooks"}, "Update "+scope+" webhook", "Updates configuration without changing the signing secret.", secCookie(), itemParams,
			jsonBody(map[string]any{"$ref": "#/components/schemas/WebhookInput"}), map[string]any{"200": jsonRefResp("Updated webhook", "#/components/schemas/Webhook")}),
		"delete": op([]string{"Webhooks"}, "Delete "+scope+" webhook", "Deletes configuration. Existing delivery outcome records remain subject to retention.", secCookie(), itemParams, nil,
			map[string]any{"204": desc("Deleted")}),
	}
	paths[base+"/{webhookID}/rotate"] = map[string]any{
		"post": op([]string{"Webhooks"}, "Rotate webhook signing secret", "Immediately replaces the secret and returns the new value exactly once.", secCookie(), itemParams, nil,
			map[string]any{"200": jsonRefResp("Webhook and new one-time secret", "#/components/schemas/WebhookSecretResponse")}),
	}
	paths[base+"/{webhookID}/test"] = map[string]any{
		"post": op([]string{"Webhooks"}, "Queue webhook test", signing, secCookie(), itemParams, nil,
			map[string]any{"202": jsonRefResp("Queued test delivery", "#/components/schemas/WebhookTestResponse")}),
	}
	paths[base+"/{webhookID}/deliveries"] = map[string]any{
		"get": op([]string{"Webhooks"}, "List webhook delivery outcomes", signing+" Response bodies and signing secrets are never returned.", secCookie(), itemParams, nil,
			map[string]any{"200": jsonSchemaResp("Delivery outcomes", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/WebhookDelivery"}})}),
	}
}
