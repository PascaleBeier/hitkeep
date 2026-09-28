package system

func openAPIV1IntegrationPaths() map[string]any {
	return map[string]any{
		"/api/user/teams/{id}/integrations/google-search-console/status": map[string]any{
			"get": op([]string{"Integrations"}, "Get Google Search Console status", "Returns team-level Google Search Console connection and credential status.", secCookie(), []any{paramRef("#/components/parameters/teamID")}, nil,
				map[string]any{
					"200": jsonSchemaResp("Google Search Console status", googleSearchConsoleStatusSchema()),
					"403": errResp("Access denied"),
				}),
		},
		"/api/user/teams/{id}/integrations/google-search-console/connect": map[string]any{
			"post": op([]string{"Integrations"}, "Start Google Search Console OAuth", "Creates a state-bound read-only Google Search Console OAuth URL for the team.", secCookie(), []any{paramRef("#/components/parameters/teamID")},
				jsonBody(map[string]any{
					"type":       "object",
					"properties": map[string]any{"return_path": map[string]any{"type": "string"}},
				}),
				map[string]any{
					"200": jsonSchemaResp("Google Search Console OAuth URL", map[string]any{
						"type":       "object",
						"properties": map[string]any{"auth_url": map[string]any{"type": "string", "format": "uri"}},
						"required":   []string{"auth_url"},
					}),
					"403": errResp("Access denied"),
					"412": errResp("Credentials missing"),
				}),
		},
		"/api/user/teams/{id}/integrations/google-search-console/properties": map[string]any{
			"get": op([]string{"Integrations"}, "List Google Search Console properties", "Lists Search Console properties visible to the connected team account and caches non-secret property metadata.", secCookie(), []any{paramRef("#/components/parameters/teamID")}, nil,
				map[string]any{
					"200": jsonSchemaResp("Google Search Console properties", googleSearchConsolePropertiesSchema()),
					"403": errResp("Access denied"),
					"412": errResp("Google Search Console is not connected"),
				}),
		},
		"/api/sites/{id}/integrations/google-search-console": map[string]any{
			"get": op([]string{"Integrations"}, "Get site Search Console mapping", "Returns the Search Console property mapping for a site.", secCookie(), []any{paramRef("#/components/parameters/siteID")}, nil,
				map[string]any{
					"200": jsonSchemaResp("Google Search Console site mapping", googleSearchConsoleSiteMappingSchema()),
					"403": errResp("Access denied"),
				}),
		},
		"/api/sites/{id}/integrations/google-search-console/property": map[string]any{
			"put": op([]string{"Integrations"}, "Map Search Console property", "Maps a visible Search Console property to a HitKeep site.", secCookie(), []any{paramRef("#/components/parameters/siteID")},
				jsonBody(map[string]any{
					"type":       "object",
					"properties": map[string]any{"property_uri": map[string]any{"type": "string"}},
					"required":   []string{"property_uri"},
				}),
				map[string]any{
					"200": jsonSchemaResp("Google Search Console site mapping", googleSearchConsoleSiteMappingSchema()),
					"400": errResp("Invalid or unavailable property"),
					"403": errResp("Access denied"),
					"412": errResp("Google Search Console is not connected"),
				}),
			"delete": op([]string{"Integrations"}, "Unmap Search Console property", "Removes the Search Console property mapping from a HitKeep site.", secCookie(), []any{paramRef("#/components/parameters/siteID")}, nil,
				map[string]any{
					"200": jsonSchemaResp("Google Search Console site mapping", googleSearchConsoleSiteMappingSchema()),
					"403": errResp("Access denied"),
					"412": errResp("Google Search Console is not connected"),
				}),
		},
		"/api/sites/{id}/integrations/google-search-console/sync": map[string]any{
			"post": op([]string{"Integrations"}, "Request Search Console sync", "Queues a manual Search Console import for the mapped site and returns the current mapping sync status.", secCookie(), []any{paramRef("#/components/parameters/siteID")}, nil,
				map[string]any{
					"200": jsonSchemaResp("Google Search Console site mapping", googleSearchConsoleSiteMappingSchema()),
					"403": errResp("Access denied"),
					"412": errResp("Google Search Console is not connected or the site is not mapped"),
				}),
		},
		"/api/user/teams/{id}/integrations/google-search-console": map[string]any{
			"delete": op([]string{"Integrations"}, "Disconnect Google Search Console", "Disconnects the team-level Google Search Console connection and clears stored token material.", secCookie(), []any{paramRef("#/components/parameters/teamID")}, nil,
				map[string]any{
					"200": jsonRefResp("Status", "#/components/schemas/Status"),
					"403": errResp("Access denied"),
				}),
		},
		"/api/integrations/google-search-console/oauth/callback": map[string]any{
			"get": op([]string{"Integrations"}, "Complete Google Search Console OAuth", "Completes the authenticated Google Search Console OAuth callback.", secCookie(), []any{
				map[string]any{"name": "state", "in": "query", "required": true, "schema": map[string]any{"type": "string"}},
				map[string]any{"name": "code", "in": "query", "required": true, "schema": map[string]any{"type": "string"}},
			}, nil,
				map[string]any{
					"302": desc("Redirects to the integration page."),
					"400": errResp("Invalid OAuth state or callback"),
					"502": errResp("OAuth exchange failed"),
				}),
		},
	}
}
