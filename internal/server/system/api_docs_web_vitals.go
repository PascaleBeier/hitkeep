package system

func openAPIV1WebVitalPaths() map[string]any {
	return mergeOpenAPIPathMaps(map[string]any{
		"/api/sites/{id}/web-vitals/summary": map[string]any{
			"get": op([]string{"Sites"}, "Get Web Vitals summary", "Returns p75, sample count, rating counts, and derived p75 rating for each Web Vital metric.", secAnyAuth(), webVitalsParams(false), nil,
				map[string]any{"200": jsonSchemaResp("Web Vitals summary", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/WebVitalSummaryMetric"}})}),
		},
		"/api/sites/{id}/web-vitals/timeseries": map[string]any{
			"get": op([]string{"Sites"}, "Get Web Vitals timeseries", "Returns p75 and rating counts bucketed by hour or day for one Web Vital metric.", secAnyAuth(), webVitalsParams(true), nil,
				map[string]any{"200": jsonSchemaResp("Web Vitals timeseries", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/WebVitalSeriesPoint"}})}),
		},
		"/api/sites/{id}/web-vitals/pages": map[string]any{
			"get": op([]string{"Sites"}, "Get Web Vitals page breakdown", "Returns page paths ranked by the selected metric p75, with per-metric p75 cells for every Web Vital available on each path.", secAnyAuth(), append(webVitalsParams(true), paramRef("#/components/parameters/limit")), nil,
				map[string]any{"200": jsonSchemaResp("Web Vitals pages", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/WebVitalPageRow"}})}),
		},
		"/api/sites/{id}/web-vitals/breakdown": map[string]any{
			"get": op([]string{"Sites"}, "Get Web Vitals visitor context breakdown", "Returns Web Vitals p75 and rating counts for one metric grouped by browser, country, language, device, city, provider, or ASN using the matching pageview context.", secAnyAuth(), append(webVitalsParams(true),
				map[string]any{"name": "dimension", "in": "query", "required": true, "description": "Visitor context dimension.", "schema": map[string]any{"type": "string", "enum": []string{"browser", "country", "language", "device", "city", "provider", "asn"}}},
				paramRef("#/components/parameters/limit"),
			), nil,
				map[string]any{"200": jsonSchemaResp("Web Vitals breakdown", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/WebVitalDimensionRow"}})}),
		},
	}, openAPIV1SharedWebVitalPaths())
}

func openAPIV1SharedWebVitalPaths() map[string]any {
	return map[string]any{
		"/api/share/{token}/sites/{id}/web-vitals/summary": map[string]any{
			"get": op([]string{"Share"}, "Shared Web Vitals summary", "Returns Web Vitals summary through a read-only share token.", nil, sharedWebVitalsParams(false), nil,
				map[string]any{"200": jsonSchemaResp("Web Vitals summary", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/WebVitalSummaryMetric"}})}),
		},
		"/api/share/{token}/sites/{id}/web-vitals/timeseries": map[string]any{
			"get": op([]string{"Share"}, "Shared Web Vitals timeseries", "Returns Web Vitals timeseries through a read-only share token.", nil, sharedWebVitalsParams(true), nil,
				map[string]any{"200": jsonSchemaResp("Web Vitals timeseries", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/WebVitalSeriesPoint"}})}),
		},
		"/api/share/{token}/sites/{id}/web-vitals/pages": map[string]any{
			"get": op([]string{"Share"}, "Shared Web Vitals page breakdown", "Returns Web Vitals page breakdown through a read-only share token.", nil, append(sharedWebVitalsParams(true), paramRef("#/components/parameters/limit")), nil,
				map[string]any{"200": jsonSchemaResp("Web Vitals pages", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/WebVitalPageRow"}})}),
		},
		"/api/share/{token}/sites/{id}/web-vitals/breakdown": map[string]any{
			"get": op([]string{"Share"}, "Shared Web Vitals visitor context breakdown", "Returns Web Vitals visitor context breakdown through a read-only share token.", nil, append(sharedWebVitalsParams(true),
				map[string]any{"name": "dimension", "in": "query", "required": true, "description": "Visitor context dimension.", "schema": map[string]any{"type": "string", "enum": []string{"browser", "country", "language", "device", "city", "provider", "asn"}}},
				paramRef("#/components/parameters/limit"),
			), nil,
				map[string]any{"200": jsonSchemaResp("Web Vitals breakdown", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/WebVitalDimensionRow"}})}),
		},
	}
}

func webVitalsParams(requireMetric bool) []any {
	params := make([]any, 0, 6)
	params = append(params,
		paramRef("#/components/parameters/siteID"),
		paramRef("#/components/parameters/from"),
		paramRef("#/components/parameters/to"),
		paramRef("#/components/parameters/webVitalPath"),
		paramRef("#/components/parameters/webVitalRating"),
	)
	metric := paramRef("#/components/parameters/webVitalMetric")
	if requireMetric {
		metric["required"] = true
	}
	return append(params, metric)
}

func sharedWebVitalsParams(requireMetric bool) []any {
	return append([]any{paramRef("#/components/parameters/token")}, webVitalsParams(requireMetric)...)
}

func openAPIV1WebVitalSchemas() map[string]any {
	return map[string]any{
		"WebVitalIngestPayload": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"n":    map[string]any{"type": "string", "enum": []string{"LCP", "INP", "CLS", "FCP", "TTFB"}, "description": "Metric name."},
				"v":    map[string]any{"type": "number", "minimum": 0, "description": "Metric value. LCP, INP, FCP, and TTFB use milliseconds. CLS is unitless."},
				"p":    map[string]any{"type": "string", "description": "Browser path. Query strings and hashes are stripped server-side."},
				"nt":   map[string]any{"type": "string", "description": "Navigation type."},
				"sid":  map[string]any{"type": "string", "format": "uuid"},
				"pid":  map[string]any{"type": "string", "format": "uuid"},
				"tsrc": map[string]any{"type": "string"},
				"tv":   map[string]any{"type": "string"},
			},
			"required": []string{"n", "v", "p", "sid", "pid"},
		},
		"WebVitalSummaryMetric": webVitalAggregateSchema(false),
		"WebVitalSeriesPoint": map[string]any{
			"type": "object",
			"properties": mergeOpenAPIMapGroups(webVitalRatingCountProperties(false), map[string]any{
				"time": map[string]any{"type": "string", "format": "date-time"},
			}),
		},
		"WebVitalPageRow": map[string]any{
			"type": "object",
			"properties": mergeOpenAPIMapGroups(webVitalRatingCountProperties(true), map[string]any{
				"rating": map[string]any{"type": "string", "enum": []string{"good", "needs_improvement", "poor"}},
				"metrics": map[string]any{
					"type":                 "object",
					"additionalProperties": map[string]any{"$ref": "#/components/schemas/WebVitalMetricBreakdown"},
				},
			}),
		},
		"WebVitalMetricBreakdown": map[string]any{
			"type": "object",
			"properties": mergeOpenAPIMapGroups(webVitalRatingCountProperties(false), map[string]any{
				"rating": map[string]any{"type": "string", "enum": []string{"good", "needs_improvement", "poor"}},
			}),
		},
		"WebVitalDimensionRow": map[string]any{
			"type": "object",
			"properties": mergeOpenAPIMapGroups(webVitalRatingCountProperties(false), map[string]any{
				"name":   map[string]any{"type": "string"},
				"rating": map[string]any{"type": "string", "enum": []string{"good", "needs_improvement", "poor"}},
			}),
		},
	}
}

func webVitalAggregateSchema(includePath bool) map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": webVitalAggregateProperties(includePath),
	}
}

func webVitalAggregateProperties(includePath bool) map[string]any {
	properties := webVitalRatingCountProperties(includePath)
	properties["metric"] = map[string]any{"type": "string", "enum": []string{"LCP", "INP", "CLS", "FCP", "TTFB"}}
	properties["rating"] = map[string]any{"type": "string", "enum": []string{"good", "needs_improvement", "poor"}}
	return properties
}

func webVitalRatingCountProperties(includePath bool) map[string]any {
	properties := map[string]any{
		"p75":               map[string]any{"type": "number"},
		"samples":           map[string]any{"type": "integer"},
		"good":              map[string]any{"type": "integer"},
		"needs_improvement": map[string]any{"type": "integer"},
		"poor":              map[string]any{"type": "integer"},
	}
	if includePath {
		properties["path"] = map[string]any{"type": "string"}
	}
	return properties
}
