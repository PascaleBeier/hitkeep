package system

func qrParams(prefix ...any) []any {
	params := append([]any{}, prefix...)
	return append(params, paramRef("#/components/parameters/qrID"))
}

func qrRangeParams(prefix ...any) []any {
	return append(qrParams(prefix...), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"))
}

func openAPIV1QRPaths() map[string]any {
	return mergeOpenAPIPathMaps(
		openAPIV1QRRedirectPaths(),
		openAPIV1QRAuthenticatedPaths(),
		openAPIV1QRSharePaths(),
	)
}

func openAPIV1QRRedirectPaths() map[string]any {
	return map[string]any{
		"/q/{token}": map[string]any{
			"get": op([]string{"QR Campaigns"}, "Open QR redirect", "Public dynamic QR redirect. Resolves a saved QR campaign token, records a privacy-safe QR open unless DNT/exclusions/spam suppression applies, and redirects to the saved destination with UTM/custom parameters plus hk_qr=<qr_code_uuid>.", nil, []any{paramRef("#/components/parameters/token")}, nil,
				map[string]any{"302": desc("Redirect to tracked destination"), "404": errResp("QR code not found")}),
		},
	}
}

func openAPIV1QRAuthenticatedPaths() map[string]any {
	return map[string]any{
		"/api/sites/{id}/qr-codes": map[string]any{
			"get": op([]string{"QR Campaigns"}, "List QR codes", "Lists saved QR campaign definitions for a site. Requires site.view.", secAnyAuth(), []any{paramRef("#/components/parameters/siteID")}, nil,
				map[string]any{"200": jsonSchemaResp("QR codes", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/QRCode"}})}),
			"post": op([]string{"QR Campaigns"}, "Create QR code", "Creates a dynamic QR campaign definition. Requires site.manage_data.", secCookie(), []any{paramRef("#/components/parameters/siteID")},
				jsonBody(map[string]any{"$ref": "#/components/schemas/QRCodeRequest"}),
				map[string]any{"201": jsonRefResp("Created QR code", "#/components/schemas/QRCode"), "400": errResp("Invalid QR request")}),
		},
		"/api/sites/{id}/qr-codes/{qrID}": map[string]any{
			"get": op([]string{"QR Campaigns"}, "Get QR code", "Returns one saved QR campaign definition. Requires site.view.", secAnyAuth(), qrParams(paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"200": jsonRefResp("QR code", "#/components/schemas/QRCode"), "404": errResp("Not found")}),
			"patch": op([]string{"QR Campaigns"}, "Update QR code", "Updates the destination, campaign parameters, custom parameters, and style for a dynamic QR code after print. Requires site.manage_data.", secCookie(), qrParams(paramRef("#/components/parameters/siteID")),
				jsonBody(map[string]any{"$ref": "#/components/schemas/QRCodeRequest"}),
				map[string]any{"200": jsonRefResp("QR code", "#/components/schemas/QRCode"), "404": errResp("Not found")}),
			"delete": op([]string{"QR Campaigns"}, "Archive QR code", "Soft-deletes a QR code so historical analytics remain available and printed codes are not hard-deleted accidentally. Requires site.manage_data.", secCookie(), qrParams(paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"204": desc("Archived"), "404": errResp("Not found")}),
		},
		"/api/sites/{id}/qr-codes/{qrID}/asset": map[string]any{
			"get": op([]string{"QR Campaigns"}, "Get QR graphic asset", "Returns the filesystem-backed raster graphic attached to a QR code. Public JSON exposes asset metadata only; storage paths are never returned. Requires site.view.", secAnyAuth(), qrParams(paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"200": desc("Image asset stream"), "404": errResp("Asset not found")}),
			"put": op([]string{"QR Campaigns"}, "Upload QR graphic asset", "Stores a PNG, JPEG, or WebP graphic for a QR code under the configured HitKeep data directory and persists only metadata in the control-plane database. Maximum size is 2 MB. Requires site.manage_data.", secCookie(), qrParams(paramRef("#/components/parameters/siteID")),
				map[string]any{"required": true, "content": map[string]any{"multipart/form-data": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"asset": map[string]any{"type": "string", "format": "binary"}}, "required": []string{"asset"}}}}},
				map[string]any{"200": jsonRefResp("QR asset", "#/components/schemas/QRCodeAsset"), "400": errResp("Invalid asset"), "413": errResp("Asset too large")}),
			"delete": op([]string{"QR Campaigns"}, "Delete QR graphic asset", "Removes the persisted graphic file and asset metadata from a QR code. Requires site.manage_data.", secCookie(), qrParams(paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"204": desc("Deleted"), "404": errResp("Not found")}),
		},
		"/api/sites/{id}/qr-codes/{qrID}/summary": map[string]any{
			"get": op([]string{"QR Campaigns"}, "Get QR analytics summary", "Returns redirect opens plus QR-scoped pageviews, visitors, pages, referrers, devices, and countries for one QR code. Requires site.view.", secAnyAuth(), qrRangeParams(paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"200": jsonRefResp("QR summary", "#/components/schemas/QRCodeSummary")}),
		},
		"/api/sites/{id}/qr-codes/{qrID}/opens/timeseries": map[string]any{
			"get": op([]string{"QR Campaigns"}, "Get QR open timeseries", "Returns QR redirect opens over time for one QR code. Requires site.view.", secAnyAuth(), qrRangeParams(paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"200": jsonSchemaResp("QR open series", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/QRCodeOpenSeriesPoint"}})}),
		},
		"/api/sites/{id}/qr-codes/{qrID}/takeout": map[string]any{
			"get": op([]string{"QR Campaigns"}, "Export QR takeout", "Exports one QR code definition, asset metadata, QR opens, and QR-filtered analytics rows. Requires site.view.", secAnyAuth(), append(qrParams(paramRef("#/components/parameters/siteID")), paramRef("#/components/parameters/format")), nil,
				map[string]any{"200": desc("Export file stream")}),
		},
		"/api/sites/{id}/qr-codes/{qrID}/share": map[string]any{
			"get": op([]string{"QR Campaigns"}, "List QR share links", "Lists QR-only share links for one QR code. Requires site.view.", secCookie(), qrParams(paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"200": jsonSchemaResp("QR share links", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/QRCodeShareLink"}})}),
			"post": op([]string{"QR Campaigns"}, "Create QR share link", "Creates a read-only QR-only share link exposing one QR code's stats and export. Requires site.manage_data.", secCookie(), qrParams(paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"201": jsonRefResp("QR share link", "#/components/schemas/QRCodeShareLink")}),
		},
		"/api/sites/{id}/qr-codes/{qrID}/share/{shareID}": map[string]any{
			"delete": op([]string{"QR Campaigns"}, "Delete QR share link", "Revokes a QR-only share link. Requires site.manage_data.", secCookie(), append(qrParams(paramRef("#/components/parameters/siteID")), paramRef("#/components/parameters/shareID")), nil,
				map[string]any{"204": desc("Deleted"), "404": errResp("Not found")}),
		},
	}
}

func openAPIV1QRSharePaths() map[string]any {
	return map[string]any{
		"/api/share/{token}/sites/{id}/qr-codes": map[string]any{
			"get": op([]string{"Share"}, "List shared QR codes", "Lists QR campaign definitions through an existing site share link.", nil, []any{paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID")}, nil,
				map[string]any{"200": jsonSchemaResp("QR codes", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/QRCode"}})}),
		},
		"/api/share/{token}/sites/{id}/qr-codes/{qrID}/summary": map[string]any{
			"get": op([]string{"Share"}, "Shared QR analytics summary", "Returns one QR code's analytics through an existing site share link.", nil, qrRangeParams(paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"200": jsonRefResp("QR summary", "#/components/schemas/QRCodeSummary")}),
		},
		"/api/share/{token}/sites/{id}/qr-codes/{qrID}": map[string]any{
			"get": op([]string{"Share"}, "Get shared QR code", "Returns one QR campaign definition through an existing site share link.", nil, qrParams(paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"200": jsonRefResp("QR code", "#/components/schemas/QRCode"), "404": errResp("Not found")}),
		},
		"/api/share/{token}/sites/{id}/qr-codes/{qrID}/asset": map[string]any{
			"get": op([]string{"Share"}, "Get shared QR asset", "Returns the filesystem-backed QR graphic through an existing site share link.", nil, qrParams(paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"200": desc("Image asset stream"), "404": errResp("Asset not found")}),
		},
		"/api/share/{token}/sites/{id}/qr-codes/{qrID}/opens/timeseries": map[string]any{
			"get": op([]string{"Share"}, "Shared QR open timeseries", "Returns QR redirect opens over time through an existing site share link.", nil, qrRangeParams(paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"200": jsonSchemaResp("QR open series", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/QRCodeOpenSeriesPoint"}})}),
		},
		"/api/share/{token}/sites/{id}/qr-codes/{qrID}/takeout": map[string]any{
			"get": op([]string{"Share"}, "Export shared QR takeout", "Exports one QR code's definition, opens, and scoped analytics through an existing site share link.", nil, append(qrParams(paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID")), paramRef("#/components/parameters/format")), nil,
				map[string]any{"200": desc("Export file stream")}),
		},
		"/api/qr-share/{token}/qr-code": map[string]any{
			"get": op([]string{"Share"}, "Get QR-only shared code", "Returns the single QR code exposed by a QR-only share link.", nil, []any{paramRef("#/components/parameters/token")}, nil,
				map[string]any{"200": jsonRefResp("QR code", "#/components/schemas/QRCode"), "404": errResp("Not found")}),
		},
		"/api/qr-share/{token}/qr-code/asset": map[string]any{
			"get": op([]string{"Share"}, "Get QR-only shared asset", "Returns the filesystem-backed QR graphic exposed by a QR-only share link.", nil, []any{paramRef("#/components/parameters/token")}, nil,
				map[string]any{"200": desc("Image asset stream"), "404": errResp("Asset not found")}),
		},
		"/api/qr-share/{token}/qr-code/summary": map[string]any{
			"get": op([]string{"Share"}, "Get QR-only shared summary", "Returns redirect opens and QR-scoped analytics for a QR-only share link.", nil, []any{paramRef("#/components/parameters/token"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to")}, nil,
				map[string]any{"200": jsonRefResp("QR summary", "#/components/schemas/QRCodeSummary")}),
		},
		"/api/qr-share/{token}/qr-code/opens/timeseries": map[string]any{
			"get": op([]string{"Share"}, "Get QR-only shared open timeseries", "Returns QR redirect opens over time for a QR-only share link.", nil, []any{paramRef("#/components/parameters/token"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to")}, nil,
				map[string]any{"200": jsonSchemaResp("QR open series", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/QRCodeOpenSeriesPoint"}})}),
		},
		"/api/qr-share/{token}/qr-code/takeout": map[string]any{
			"get": op([]string{"Share"}, "Export QR-only shared takeout", "Exports one QR code's definition, opens, and scoped analytics through a QR-only share link.", nil, []any{paramRef("#/components/parameters/token"), paramRef("#/components/parameters/format")}, nil,
				map[string]any{"200": desc("Export file stream")}),
		},
	}
}

func openAPIV1QRSchemas() map[string]any {
	return mergeOpenAPIMapGroups(
		openAPIV1QRCodeDefinitionSchemas(),
		openAPIV1QRCodeAnalyticsSchemas(),
	)
}

func openAPIV1QRCodeDefinitionSchemas() map[string]any {
	return map[string]any{
		"QRCode": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":              map[string]any{"type": "string", "format": "uuid"},
				"site_id":         map[string]any{"type": "string", "format": "uuid"},
				"created_by":      map[string]any{"type": "string", "format": "uuid"},
				"name":            map[string]any{"type": "string"},
				"destination_url": map[string]any{"type": "string", "format": "uri"},
				"utm_source":      map[string]any{"type": "string"},
				"utm_medium":      map[string]any{"type": "string"},
				"utm_campaign":    map[string]any{"type": "string"},
				"utm_term":        map[string]any{"type": "string"},
				"utm_content":     map[string]any{"type": "string"},
				"custom_params":   map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
				"style":           map[string]any{"type": "object", "additionalProperties": true},
				"redirect_url":    map[string]any{"type": "string", "format": "uri", "description": "Dynamic HitKeep redirect URL to encode in printed QR assets."},
				"token_hint":      map[string]any{"type": "string"},
				"has_asset":       map[string]any{"type": "boolean"},
				"created_at":      map[string]any{"type": "string", "format": "date-time"},
				"updated_at":      map[string]any{"type": "string", "format": "date-time"},
				"archived_at":     map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"QRCodeRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":            map[string]any{"type": "string"},
				"destination_url": map[string]any{"type": "string", "format": "uri"},
				"utm_source":      map[string]any{"type": "string"},
				"utm_medium":      map[string]any{"type": "string"},
				"utm_campaign":    map[string]any{"type": "string"},
				"utm_term":        map[string]any{"type": "string"},
				"utm_content":     map[string]any{"type": "string"},
				"custom_params":   map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
				"style":           map[string]any{"type": "object", "additionalProperties": true},
			},
			"required": []string{"name", "destination_url"},
		},
	}
}

func openAPIV1QRCodeAnalyticsSchemas() map[string]any {
	return map[string]any{
		"QRCodeAsset": map[string]any{
			"type":        "object",
			"description": "Public QR graphic metadata. The raster image is stored under the configured HitKeep data directory; filesystem storage keys and binary payloads are not exposed in JSON or table takeout.",
			"properties": map[string]any{
				"qr_code_id":   map[string]any{"type": "string", "format": "uuid"},
				"site_id":      map[string]any{"type": "string", "format": "uuid"},
				"filename":     map[string]any{"type": "string"},
				"content_type": map[string]any{"type": "string", "enum": []string{"image/png", "image/jpeg", "image/webp"}},
				"byte_size":    map[string]any{"type": "integer"},
				"width":        map[string]any{"type": "integer"},
				"height":       map[string]any{"type": "integer"},
				"checksum":     map[string]any{"type": "string"},
				"created_at":   map[string]any{"type": "string", "format": "date-time"},
				"updated_at":   map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"QRCodeSummary": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"qr_code":       map[string]any{"$ref": "#/components/schemas/QRCode"},
				"open_count":    map[string]any{"type": "integer"},
				"pageviews":     map[string]any{"type": "integer"},
				"visitors":      map[string]any{"type": "integer"},
				"top_pages":     map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_referrers": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_devices":   map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
				"top_countries": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}},
			},
		},
		"QRCodeOpenSeriesPoint": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"time":  map[string]any{"type": "string", "format": "date-time"},
				"opens": map[string]any{"type": "integer"},
			},
		},
		"QRCodeShareLink": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":         map[string]any{"type": "string", "format": "uuid"},
				"site_id":    map[string]any{"type": "string", "format": "uuid"},
				"qr_code_id": map[string]any{"type": "string", "format": "uuid"},
				"token_hint": map[string]any{"type": "string"},
				"url":        map[string]any{"type": "string", "format": "uri"},
				"token":      map[string]any{"type": "string"},
				"created_at": map[string]any{"type": "string", "format": "date-time"},
			},
		},
	}
}
