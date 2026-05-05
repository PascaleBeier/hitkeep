package system

func openAPIV1SearchConsoleReportPaths() map[string]any {
	return map[string]any{
		"/api/sites/{id}/search-console/overview": map[string]any{
			"get": searchConsoleReportOp("Get Search Console overview", "Returns aggregate Google Search Console clicks, impressions, CTR, and average position for a mapped site.", "#/components/schemas/SearchConsoleOverview"),
		},
		"/api/sites/{id}/search-console/series": map[string]any{
			"get": searchConsoleReportOp("Get Search Console series", "Returns daily Google Search Console metrics for a mapped site.", "#/components/schemas/SearchConsoleSeriesResponse"),
		},
		"/api/sites/{id}/search-console/queries": map[string]any{
			"get": searchConsoleReportOp("Get top Search Console queries", "Returns top Search Console query rows for a mapped site. Query values are imported aggregate Google data and are not session filters.", "#/components/schemas/SearchConsoleDimensionResponse"),
		},
		"/api/sites/{id}/search-console/pages": map[string]any{
			"get": searchConsoleReportOp("Get top Search Console pages", "Returns top Search Console page rows for a mapped site.", "#/components/schemas/SearchConsoleDimensionResponse"),
		},
		"/api/sites/{id}/search-console/breakdowns": map[string]any{
			"get": op([]string{"Reports"}, "Get Search Console country or device breakdown", "Returns Search Console rows grouped by country or device for a mapped site.", secAnyAuth(), append(searchConsoleReportParams(),
				map[string]any{"name": "dimension", "in": "query", "required": true, "description": "Breakdown dimension.", "schema": map[string]any{"type": "string", "enum": []string{"country", "device"}}},
			), nil, searchConsoleReportResponses("#/components/schemas/SearchConsoleDimensionResponse")),
		},
	}
}

func searchConsoleReportOp(summary, description, schemaRef string) map[string]any {
	return op([]string{"Reports"}, summary, description, secAnyAuth(), searchConsoleReportParams(), nil, searchConsoleReportResponses(schemaRef))
}

func searchConsoleReportParams() []any {
	return []any{
		paramRef("#/components/parameters/siteID"),
		paramRef("#/components/parameters/from"),
		paramRef("#/components/parameters/to"),
		map[string]any{"name": "page", "in": "query", "description": "Exact Search Console page URL filter.", "schema": map[string]any{"type": "string"}},
		map[string]any{"name": "path", "in": "query", "description": "HitKeep path filter matched against Search Console page URLs.", "schema": map[string]any{"type": "string"}},
		map[string]any{"name": "country", "in": "query", "description": "Search Console country code filter.", "schema": map[string]any{"type": "string"}},
		map[string]any{"name": "device", "in": "query", "description": "Search Console device filter.", "schema": map[string]any{"type": "string"}},
		map[string]any{"name": "limit", "in": "query", "description": "Maximum rows for table endpoints. Between 1 and 100.", "schema": map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 10}},
	}
}

func searchConsoleReportResponses(schemaRef string) map[string]any {
	return map[string]any{
		"200": jsonRefResp("Search Console report", schemaRef),
		"400": errResp("Invalid request"),
		"401": errResp("Unauthorized"),
		"403": errResp("Access denied"),
		"412": errResp("Search Console property is not mapped"),
		"429": errResp("Too many requests"),
		"503": errResp("Service not available"),
	}
}
