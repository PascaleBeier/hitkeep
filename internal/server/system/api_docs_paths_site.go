package system

func openAPIV1AdminSitePaths() map[string]any {
	return map[string]any{
		"/api/admin/users": map[string]any{
			"get": op([]string{"Admin"}, "List users", "Lists users for admin management.", secCookie(), nil, nil, map[string]any{"200": jsonSchemaResp("User list", map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": true}})}),
		},
		"/api/admin/users/{id}/role": map[string]any{
			"post": op([]string{"Admin"}, "Update user role", "Updates instance role for target user.", secCookie(), []any{paramRef("#/components/parameters/adminUserID")},
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"role": map[string]any{"type": "string"}}, "required": []string{"role"}}),
				map[string]any{"200": jsonRefResp("Status", "#/components/schemas/Status")}),
		},
		"/api/admin/users/{id}/disable-2fa": map[string]any{
			"post": op([]string{"Admin"}, "Disable user MFA", "Owner-only recovery action that clears TOTP, passkeys, pending MFA challenges, and remember-me sessions for the target user.", secCookie(), []any{paramRef("#/components/parameters/adminUserID")}, nil, map[string]any{
				"200": jsonRefResp("Disable user MFA response", "#/components/schemas/AdminDisableUserMFAResponse"),
				"403": errResp("Forbidden"),
				"404": errResp("Not found"),
			}),
		},
		"/api/admin/users/{id}": map[string]any{
			"delete": op([]string{"Admin"}, "Delete user", "Deletes user account (cannot delete self). Deletion is blocked if the target user is the sole owner of any team.", secCookie(), []any{paramRef("#/components/parameters/adminUserID")}, nil, map[string]any{
				"200": jsonRefResp("Status", "#/components/schemas/Status"),
				"409": jsonRefResp("Delete user blocked response", "#/components/schemas/AdminDeleteUserBlockedResponse"),
			}),
		},
		"/api/admin/sites": map[string]any{
			"get": op([]string{"Admin"}, "List all sites", "Lists all sites for admin management.", secCookie(), nil, nil, map[string]any{"200": jsonSchemaResp("Site list", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Site"}})}),
		},
		"/api/admin/sites/{id}": map[string]any{
			"delete": op([]string{"Admin"}, "Delete site (admin)", "Deletes site by admin endpoint.", secCookie(), []any{paramRef("#/components/parameters/siteID")}, nil, map[string]any{"200": jsonRefResp("Status", "#/components/schemas/Status")}),
		},
		"/api/admin/teams/{id}": map[string]any{
			"delete": op([]string{"Admin"}, "Delete archived team", "Permanently deletes an archived non-default team and removes its per-tenant analytics database directory.", secCookie(), []any{paramRef("#/components/parameters/teamID")}, nil, map[string]any{
				"200": jsonRefResp("Delete archived team response", "#/components/schemas/AdminDeleteTeamResponse"),
				"400": errResp("Archive the team first, and ensure it has no sites"),
				"404": errResp("Team not found"),
			}),
		},
		"/api/admin/exclusions": map[string]any{
			"get": op([]string{"Admin"}, "List global exclusions", "Lists instance-level IP/CIDR exclusions used by ingest filtering.", secCookie(), nil, nil,
				map[string]any{"200": jsonSchemaResp("Global exclusions", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/IPExclusion"}})}),
			"post": op([]string{"Admin"}, "Create global exclusion", "Creates instance-level IP/CIDR exclusion for all sites.", secCookie(), nil,
				jsonBody(map[string]any{"$ref": "#/components/schemas/IPExclusionCreateRequest"}),
				map[string]any{"201": jsonRefResp("Created exclusion", "#/components/schemas/IPExclusion"), "400": errResp("Invalid IP/CIDR")}),
		},
		"/api/admin/exclusions/{ruleID}": map[string]any{
			"delete": op([]string{"Admin"}, "Delete global exclusion", "Deletes instance-level IP/CIDR exclusion rule.", secCookie(), []any{paramRef("#/components/parameters/ruleID")}, nil,
				map[string]any{"204": desc("Deleted"), "404": errResp("Not found")}),
		},
		"/api/sites/{id}/members": map[string]any{
			"get": op([]string{"Admin"}, "List site members", "Lists site members and roles.", secCookie(), []any{paramRef("#/components/parameters/siteID")}, nil, map[string]any{"200": jsonSchemaResp("Members", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/SiteMember"}})}),
			"post": op([]string{"Admin"}, "Add site member", "Adds member to site and optionally sends invite.", secCookie(), []any{paramRef("#/components/parameters/siteID")},
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"email": map[string]any{"type": "string", "format": "email"}, "role": map[string]any{"type": "string"}}, "required": []string{"email", "role"}}),
				map[string]any{"200": jsonRefResp("Status", "#/components/schemas/Status")}),
		},
		"/api/sites/{id}/members/{userId}": map[string]any{
			"delete": op([]string{"Admin"}, "Remove site member", "Removes a user from site membership.", secCookie(), []any{paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/userID")}, nil, map[string]any{"200": jsonRefResp("Status", "#/components/schemas/Status")}),
		},

		"/api/sites": map[string]any{
			"get": op([]string{"Sites"}, "List accessible sites", "Lists sites visible to caller (session or API key scope).", secAnyAuth(), nil, nil, map[string]any{"200": jsonSchemaResp("Sites", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Site"}})}),
			"post": op([]string{"Sites"}, "Create site", "Creates new tracked site.", secCookie(), nil,
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"domain": map[string]any{"type": "string"}}, "required": []string{"domain"}}),
				map[string]any{"200": jsonRefResp("Site", "#/components/schemas/Site"), "409": errResp("Domain exists")}),
		},
		"/api/sites/{id}": map[string]any{
			"delete": op([]string{"Sites"}, "Delete site", "Deletes a site and associated analytics data.", secCookie(), []any{paramRef("#/components/parameters/siteID")}, nil, map[string]any{"200": jsonRefResp("Status", "#/components/schemas/Status")}),
		},
		"/api/sites/{id}/stats": map[string]any{
			"get": op([]string{"Sites"}, "Get site stats", "Aggregated site metrics and charts.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"),
				paramRef("#/components/parameters/filter"), paramRef("#/components/parameters/filterType"), paramRef("#/components/parameters/filterValue"),
				paramRef("#/components/parameters/goalIDQuery"), paramRef("#/components/parameters/funnelIDQuery"),
			}, nil, map[string]any{"200": jsonRefResp("Site stats", "#/components/schemas/SiteStats")}),
		},
		"/api/sites/{id}/hits": map[string]any{
			"get": op([]string{"Sites"}, "Get site hits", "Returns paginated raw hits.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"),
				paramRef("#/components/parameters/limit"), paramRef("#/components/parameters/offset"), paramRef("#/components/parameters/query"),
				paramRef("#/components/parameters/sort"), paramRef("#/components/parameters/order"),
				paramRef("#/components/parameters/filter"), paramRef("#/components/parameters/filterType"), paramRef("#/components/parameters/filterValue"),
			}, nil, map[string]any{"200": jsonRefResp("Paginated hits", "#/components/schemas/PaginatedHits")}),
		},
		"/api/sites/{id}/hits/export": map[string]any{
			"get": op([]string{"Sites"}, "Export site hits", "Exports filtered site hits in csv/xlsx/parquet/json/ndjson.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"),
				paramRef("#/components/parameters/query"), paramRef("#/components/parameters/filter"), paramRef("#/components/parameters/filterType"), paramRef("#/components/parameters/filterValue"), paramRef("#/components/parameters/format"),
			}, nil, map[string]any{"200": desc("Export file stream")}),
		},
		"/api/sites/{id}/ecommerce": map[string]any{
			"get": op([]string{"Sites"}, "Get ecommerce summary", "Returns revenue, orders, average order value, checkout starts, and checkout conversion for a site.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"),
				paramRef("#/components/parameters/filter"), paramRef("#/components/parameters/filterType"), paramRef("#/components/parameters/filterValue"),
				paramRef("#/components/parameters/itemID"), paramRef("#/components/parameters/itemName"),
			}, nil, map[string]any{"200": jsonRefResp("Ecommerce summary", "#/components/schemas/EcommerceSummary")}),
		},
		"/api/sites/{id}/ecommerce/timeseries": map[string]any{
			"get": op([]string{"Sites"}, "Get ecommerce timeseries", "Returns revenue and order counts over time for a site.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"),
				paramRef("#/components/parameters/filter"), paramRef("#/components/parameters/filterType"), paramRef("#/components/parameters/filterValue"),
				paramRef("#/components/parameters/itemID"), paramRef("#/components/parameters/itemName"),
			}, nil, map[string]any{"200": jsonSchemaResp("Ecommerce timeseries", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/EcommerceSeriesPoint"}})}),
		},
		"/api/sites/{id}/ecommerce/products": map[string]any{
			"get": op([]string{"Sites"}, "Get top ecommerce products", "Returns top products by revenue from purchase events.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"),
				paramRef("#/components/parameters/filter"), paramRef("#/components/parameters/filterType"), paramRef("#/components/parameters/filterValue"),
				paramRef("#/components/parameters/itemID"), paramRef("#/components/parameters/itemName"), paramRef("#/components/parameters/limit"),
			}, nil, map[string]any{"200": jsonSchemaResp("Ecommerce products", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/EcommerceProductStat"}})}),
		},
		"/api/sites/{id}/ecommerce/sources": map[string]any{
			"get": op([]string{"Sites"}, "Get ecommerce sources", "Returns revenue and order counts grouped by UTM source, medium, campaign, and referrer.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"),
				paramRef("#/components/parameters/filter"), paramRef("#/components/parameters/filterType"), paramRef("#/components/parameters/filterValue"),
				paramRef("#/components/parameters/itemID"), paramRef("#/components/parameters/itemName"), paramRef("#/components/parameters/limit"),
			}, nil, map[string]any{"200": jsonSchemaResp("Ecommerce sources", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/EcommerceSourceStat"}})}),
		},
		"/api/sites/{id}/ingest/ai-fetch": map[string]any{
			"post": op([]string{"Sites"}, "Record AI fetch", "Accepts a server-side AI crawler fetch record for a site. The user agent must match a known AI bot. Intended for edge or log-forwarded fetch analytics.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"),
			}, jsonBody(map[string]any{"$ref": "#/components/schemas/AIFetchIngestPayload"}), map[string]any{
				"202": desc("Accepted"),
			}),
		},
		"/api/sites/{id}/ai-fetch/overview": map[string]any{
			"get": op([]string{"Sites"}, "Get AI fetch overview", "Returns aggregate AI fetch metrics for a site including request counts, error rates, response time, assistant breakdowns, and resource type split.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"),
				map[string]any{"name": "assistant_name", "in": "query", "description": "Optional AI assistant bot name filter.", "schema": map[string]any{"type": "string"}},
				map[string]any{"name": "assistant_family", "in": "query", "description": "Optional AI assistant family filter.", "schema": map[string]any{"type": "string"}},
				map[string]any{"name": "resource_type", "in": "query", "description": "Optional AI fetch resource type filter.", "schema": map[string]any{"type": "string", "enum": []string{"html", "document", "image", "other"}}},
			}, nil, map[string]any{"200": jsonRefResp("AI fetch overview", "#/components/schemas/AIFetchOverview")}),
		},
		"/api/sites/{id}/ai-fetch/timeseries": map[string]any{
			"get": op([]string{"Sites"}, "Get AI fetch timeseries", "Returns AI fetch request counts over time for the selected site and filter set.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"),
				map[string]any{"name": "assistant_name", "in": "query", "description": "Optional AI assistant bot name filter.", "schema": map[string]any{"type": "string"}},
				map[string]any{"name": "assistant_family", "in": "query", "description": "Optional AI assistant family filter.", "schema": map[string]any{"type": "string"}},
				map[string]any{"name": "resource_type", "in": "query", "description": "Optional AI fetch resource type filter.", "schema": map[string]any{"type": "string", "enum": []string{"html", "document", "image", "other"}}},
			}, nil, map[string]any{"200": jsonSchemaResp("AI fetch timeseries", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/AIFetchSeriesPoint"}})}),
		},
		"/api/sites/{id}/ai-fetch/correlation": map[string]any{
			"get": op([]string{"Sites"}, "Get AI fetch correlation report", "Returns directional AI fetch correlation metrics for a site by matching AI crawler fetches to later AI-referred visits on the same path within a bounded window. Assistant filters apply to the fetch side only; correlated visit counts include any AI assistant referrer that later drove a human visit to the same path.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"),
				map[string]any{"name": "assistant_name", "in": "query", "description": "Optional AI assistant bot name filter.", "schema": map[string]any{"type": "string"}},
				map[string]any{"name": "assistant_family", "in": "query", "description": "Optional AI assistant family filter.", "schema": map[string]any{"type": "string"}},
				map[string]any{"name": "resource_type", "in": "query", "description": "Optional AI fetch resource type filter.", "schema": map[string]any{"type": "string", "enum": []string{"html", "document", "image", "other"}}},
				map[string]any{"name": "window_days", "in": "query", "description": "Directional correlation window in days. Must be between 1 and 90. Defaults to 30.", "schema": map[string]any{"type": "integer", "minimum": 1, "maximum": 90, "default": 30}},
			}, nil, map[string]any{"200": jsonRefResp("AI fetch correlation report", "#/components/schemas/AIFetchCorrelationReport")}),
		},
		"/api/sites/{id}/ai-fetch/export": map[string]any{
			"get": op([]string{"Sites"}, "Export AI fetch records", "Exports AI fetch records for the selected site and date range in csv/xlsx/parquet/json/ndjson. Optional assistant and resource filters restrict the export to a subset of crawler traffic.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"),
				paramRef("#/components/parameters/from"),
				paramRef("#/components/parameters/to"),
				paramRef("#/components/parameters/format"),
				map[string]any{"name": "assistant_name", "in": "query", "description": "Optional AI assistant bot name filter.", "schema": map[string]any{"type": "string"}},
				map[string]any{"name": "assistant_family", "in": "query", "description": "Optional AI assistant family filter.", "schema": map[string]any{"type": "string"}},
				map[string]any{"name": "resource_type", "in": "query", "description": "Optional AI fetch resource type filter.", "schema": map[string]any{"type": "string", "enum": []string{"html", "document", "image", "other"}}},
			}, nil, map[string]any{"200": desc("Export file stream")}),
		},
		"/api/sites/{id}/ai-chatbots/export": map[string]any{
			"get": op([]string{"Sites"}, "Export AI chatbot events", "Exports AI chatbot instrumentation events for the selected site and date range in csv/xlsx/parquet/json/ndjson. Optional scope filters restrict the export to a single provider, bot, surface, or model.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"),
				paramRef("#/components/parameters/from"),
				paramRef("#/components/parameters/to"),
				paramRef("#/components/parameters/format"),
				map[string]any{"name": "scope_key", "in": "query", "description": "Optional chatbot scope filter key.", "schema": map[string]any{"type": "string", "enum": []string{"provider", "bot_id", "surface", "model"}}},
				map[string]any{"name": "scope_value", "in": "query", "description": "Optional chatbot scope filter value. Requires scope_key.", "schema": map[string]any{"type": "string"}},
			}, nil, map[string]any{"200": desc("Export file stream")}),
		},
		"/api/favicon/{domain}": map[string]any{
			"get": op([]string{"Sites"}, "Get favicon", "Proxies favicon by domain.", nil, []any{paramRef("#/components/parameters/domain")}, nil, map[string]any{"200": desc("Favicon image")}),
		},
		"/api/sites/{id}/retention": map[string]any{
			"put": op([]string{"Sites"}, "Update retention policy", "Updates per-site retention days.", secCookie(), []any{paramRef("#/components/parameters/siteID")},
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"days": map[string]any{"type": "integer", "minimum": 0}}, "required": []string{"days"}}),
				map[string]any{"200": desc("Updated")}),
		},
		"/api/sites/{id}/transfer-team": map[string]any{
			"post": op([]string{"Sites"}, "Transfer site to another team", "Moves a site into another team the caller can administer and migrates analytics data to the destination tenant store.", secCookie(), []any{paramRef("#/components/parameters/siteID")},
				jsonBody(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"team_id": map[string]any{"type": "string", "format": "uuid"},
					},
					"required": []string{"team_id"},
				}),
				map[string]any{
					"200": jsonRefResp("Site transferred", "#/components/schemas/SiteTransferResponse"),
					"403": errResp("Access denied"),
				}),
		},
		"/api/sites/{id}/exclusions": map[string]any{
			"get": op([]string{"Sites"}, "List site exclusions", "Lists per-site IP/CIDR exclusions used by ingest filtering.", secCookie(), []any{paramRef("#/components/parameters/siteID")}, nil,
				map[string]any{"200": jsonSchemaResp("Site exclusions", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/IPExclusion"}})}),
			"post": op([]string{"Sites"}, "Create site exclusion", "Creates per-site IP/CIDR exclusion rule.", secCookie(), []any{paramRef("#/components/parameters/siteID")},
				jsonBody(map[string]any{"$ref": "#/components/schemas/IPExclusionCreateRequest"}),
				map[string]any{"201": jsonRefResp("Created exclusion", "#/components/schemas/IPExclusion"), "400": errResp("Invalid IP/CIDR")}),
		},
		"/api/sites/{id}/exclusions/{ruleID}": map[string]any{
			"delete": op([]string{"Sites"}, "Delete site exclusion", "Deletes a per-site IP/CIDR exclusion rule.", secCookie(), []any{paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/ruleID")}, nil,
				map[string]any{"204": desc("Deleted"), "404": errResp("Not found")}),
		},

		"/api/sites/{id}/goals": map[string]any{
			"get": op([]string{"Goals"}, "List goals", "Lists goals for a site.", secAnyAuth(), []any{paramRef("#/components/parameters/siteID")}, nil, map[string]any{"200": jsonSchemaResp("Goals", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Goal"}})}),
			"post": op([]string{"Goals"}, "Create goal", "Creates a conversion goal.", secCookie(), []any{paramRef("#/components/parameters/siteID")},
				jsonBody(map[string]any{"$ref": "#/components/schemas/Goal"}),
				map[string]any{"201": desc("Created")}),
		},
		"/api/sites/{id}/goals/{goalID}": map[string]any{
			"delete": op([]string{"Goals"}, "Delete goal", "Deletes goal from site.", secCookie(), []any{paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/goalID")}, nil, map[string]any{"200": desc("Deleted")}),
		},
		"/api/sites/{id}/goals/timeseries": map[string]any{
			"get": op([]string{"Goals"}, "Goal timeseries", "Returns goal conversion timeseries.", secAnyAuth(), []any{paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"), paramRef("#/components/parameters/goalIDQuery")}, nil,
				map[string]any{"200": jsonSchemaResp("Goal timeseries", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/GoalSeriesPoint"}})}),
		},

		"/api/sites/{id}/funnels": map[string]any{
			"get": op([]string{"Funnels"}, "List funnels", "Lists funnels for a site.", secAnyAuth(), []any{paramRef("#/components/parameters/siteID")}, nil, map[string]any{"200": jsonSchemaResp("Funnels", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Funnel"}})}),
			"post": op([]string{"Funnels"}, "Create funnel", "Creates a multi-step funnel.", secCookie(), []any{paramRef("#/components/parameters/siteID")},
				jsonBody(map[string]any{"$ref": "#/components/schemas/Funnel"}),
				map[string]any{"201": desc("Created")}),
		},
		"/api/sites/{id}/funnels/{funnelID}": map[string]any{
			"delete": op([]string{"Funnels"}, "Delete funnel", "Deletes funnel from site.", secCookie(), []any{paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/funnelID")}, nil, map[string]any{"200": desc("Deleted")}),
		},
		"/api/sites/{id}/funnels/timeseries": map[string]any{
			"get": op([]string{"Funnels"}, "Funnel timeseries", "Returns funnel entry/completion timeseries.", secAnyAuth(), []any{paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"), paramRef("#/components/parameters/funnelIDQuery")}, nil,
				map[string]any{"200": jsonSchemaResp("Funnel timeseries", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/FunnelSeriesPoint"}})}),
		},
		"/api/sites/{id}/funnels/{funnelID}/stats": map[string]any{
			"get": op([]string{"Funnels"}, "Funnel stats", "Returns full funnel step stats.", secAnyAuth(), []any{paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/funnelID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to")}, nil,
				map[string]any{"200": jsonRefResp("Funnel stats", "#/components/schemas/FunnelStats")}),
		},

		"/api/user/takeout": map[string]any{
			"get": op([]string{"Takeout"}, "User takeout", "Exports all user data across sites as xlsx/csv/parquet/json/ndjson.", secCookie(), []any{paramRef("#/components/parameters/format")}, nil, map[string]any{"200": desc("Export file stream")}),
		},
		"/api/sites/{id}/takeout": map[string]any{
			"get": op([]string{"Takeout"}, "Site takeout", "Exports site data as xlsx/csv/parquet/json/ndjson.", secCookie(), []any{paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/format")}, nil, map[string]any{"200": desc("Export file stream")}),
		},

		"/api/user/report-subscriptions": map[string]any{
			"get": op([]string{"Reports"}, "Get report subscriptions", "Returns all report subscription preferences for the authenticated user, including per-site and digest settings.", secCookie(), nil, nil,
				map[string]any{"200": jsonRefResp("Report subscriptions", "#/components/schemas/ReportSubscriptions")}),
		},
		"/api/user/report-subscriptions/digest": map[string]any{
			"put": op([]string{"Reports"}, "Update digest subscription", "Updates the consolidated digest subscription frequencies for the authenticated user.", secCookie(), nil,
				jsonBody(map[string]any{"$ref": "#/components/schemas/DigestSubscription"}),
				map[string]any{"204": desc("Updated"), "400": errResp("Invalid request")}),
		},
		"/api/user/report-subscriptions/sites/{site_id}": map[string]any{
			"put": op([]string{"Reports"}, "Update site report subscription", "Updates per-site report subscription frequencies for the authenticated user.", secCookie(), []any{paramRef("#/components/parameters/reportSiteID")},
				jsonBody(map[string]any{"$ref": "#/components/schemas/DigestSubscription"}),
				map[string]any{"204": desc("Updated"), "400": errResp("Invalid site ID or request")}),
		},

		"/api/sites/{id}/share": map[string]any{
			"get": op([]string{"Share"}, "List share links", "Lists share links for site.", secCookie(), []any{paramRef("#/components/parameters/siteID")}, nil, map[string]any{"200": jsonSchemaResp("Share links", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/ShareLink"}})}),
			"post": op([]string{"Share"}, "Create share link", "Creates new read-only share token URL.", secCookie(), []any{paramRef("#/components/parameters/siteID")}, nil,
				map[string]any{"200": jsonSchemaResp("Share link created", map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string", "format": "uuid"}, "url": map[string]any{"type": "string"}, "token": map[string]any{"type": "string"}, "token_hint": map[string]any{"type": "string"}, "created_at": map[string]any{"type": "string", "format": "date-time"}}})}),
		},
		"/api/sites/{id}/share/{shareID}": map[string]any{
			"delete": op([]string{"Share"}, "Delete share link", "Revokes a share link.", secCookie(), []any{paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/shareID")}, nil, map[string]any{"204": desc("Deleted")}),
		},
		"/api/share/{token}/site": map[string]any{
			"get": op([]string{"Share"}, "Get shared site", "Gets site metadata from share token.", nil, []any{paramRef("#/components/parameters/token")}, nil, map[string]any{"200": jsonRefResp("Site", "#/components/schemas/Site")}),
		},
		"/api/share/{token}/sites/{id}/stats": map[string]any{
			"get": op([]string{"Share"}, "Shared site stats", "Returns stats through share token.", nil, []any{paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"), paramRef("#/components/parameters/filter"), paramRef("#/components/parameters/filterType"), paramRef("#/components/parameters/filterValue"), paramRef("#/components/parameters/goalIDQuery"), paramRef("#/components/parameters/funnelIDQuery")}, nil, map[string]any{"200": jsonRefResp("Site stats", "#/components/schemas/SiteStats")}),
		},
		"/api/share/{token}/sites/{id}/hits": map[string]any{
			"get": op([]string{"Share"}, "Shared hits", "Returns paginated raw hits through share token.", nil, []any{paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"), paramRef("#/components/parameters/limit"), paramRef("#/components/parameters/offset"), paramRef("#/components/parameters/query"), paramRef("#/components/parameters/sort"), paramRef("#/components/parameters/order"), paramRef("#/components/parameters/filter"), paramRef("#/components/parameters/filterType"), paramRef("#/components/parameters/filterValue")}, nil, map[string]any{"200": jsonRefResp("Paginated hits", "#/components/schemas/PaginatedHits")}),
		},
		"/api/share/{token}/sites/{id}/hits/export": map[string]any{
			"get": op([]string{"Share"}, "Export shared hits", "Exports hits through share token in csv/xlsx/parquet/json/ndjson.", nil, []any{paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"), paramRef("#/components/parameters/query"), paramRef("#/components/parameters/filter"), paramRef("#/components/parameters/filterType"), paramRef("#/components/parameters/filterValue"), paramRef("#/components/parameters/format")}, nil, map[string]any{"200": desc("Export file stream")}),
		},
		"/api/share/{token}/sites/{id}/goals": map[string]any{
			"get": op([]string{"Share"}, "Shared goals", "Lists goals through share token.", nil, []any{paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID")}, nil, map[string]any{"200": jsonSchemaResp("Goals", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Goal"}})}),
		},
		"/api/share/{token}/sites/{id}/goals/timeseries": map[string]any{
			"get": op([]string{"Share"}, "Shared goal timeseries", "Returns goal timeseries through share token.", nil, []any{paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"), paramRef("#/components/parameters/goalIDQuery")}, nil, map[string]any{"200": jsonSchemaResp("Goal timeseries", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/GoalSeriesPoint"}})}),
		},
		"/api/share/{token}/sites/{id}/funnels": map[string]any{
			"get": op([]string{"Share"}, "Shared funnels", "Lists funnels through share token.", nil, []any{paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID")}, nil, map[string]any{"200": jsonSchemaResp("Funnels", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Funnel"}})}),
		},
		"/api/share/{token}/sites/{id}/funnels/timeseries": map[string]any{
			"get": op([]string{"Share"}, "Shared funnel timeseries", "Returns funnel timeseries through share token.", nil, []any{paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"), paramRef("#/components/parameters/funnelIDQuery")}, nil, map[string]any{"200": jsonSchemaResp("Funnel timeseries", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/FunnelSeriesPoint"}})}),
		},
		"/api/share/{token}/sites/{id}/funnels/{funnelID}/stats": map[string]any{
			"get": op([]string{"Share"}, "Shared funnel stats", "Returns funnel stats through share token.", nil, []any{paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/funnelID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to")}, nil, map[string]any{"200": jsonRefResp("Funnel stats", "#/components/schemas/FunnelStats")}),
		},
	}
}
