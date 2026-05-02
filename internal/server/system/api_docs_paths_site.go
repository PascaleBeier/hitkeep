package system

func eventRangeParams(prefix ...any) []any {
	params := append([]any{}, prefix...)
	return append(params, paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"))
}

func eventNameParams(prefix ...any) []any {
	return append(eventRangeParams(prefix...), paramRef("#/components/parameters/eventName"))
}

func eventFilteredParams(prefix ...any) []any {
	return append(eventNameParams(prefix...),
		paramRef("#/components/parameters/eventPropertyKey"),
		paramRef("#/components/parameters/eventPropertyValue"),
		paramRef("#/components/parameters/filter"),
		paramRef("#/components/parameters/eventDimensionKey"),
		paramRef("#/components/parameters/eventDimensionValue"),
	)
}

func openAPIV1AdminSitePaths() map[string]any {
	return map[string]any{
		"/api/admin/system": map[string]any{
			"get": op([]string{"Admin"}, "Get system overview", "Returns version, runtime mode, uptime, public URL, and operator feature switch status.", secCookie(), nil, nil,
				map[string]any{"200": jsonRefResp("System overview", "#/components/schemas/SystemInfo")}),
		},
		"/api/admin/system/health": map[string]any{
			"get": op([]string{"Admin"}, "Get system health", "Returns instance health, database status, worker status, and cluster leader state.", secCookie(), nil, nil,
				map[string]any{"200": jsonRefResp("System health", "#/components/schemas/SystemHealth")}),
		},
		"/api/admin/system/storage": map[string]any{
			"get": op([]string{"Admin"}, "Get system storage", "Returns configured data paths, shared and tenant database sizes, backup path, spam cache path, and disk capacity fields when available.", secCookie(), nil, nil,
				map[string]any{"200": jsonRefResp("System storage", "#/components/schemas/SystemStorage")}),
		},
		"/api/admin/system/ingest": map[string]any{
			"get": op([]string{"Admin"}, "Get ingest volume", "Returns recent hit, event, rejection, spam, and hit-rate counters across the instance, including tenant analytics databases.", secCookie(), nil, nil,
				map[string]any{"200": jsonRefResp("System ingest stats", "#/components/schemas/SystemIngestStats")}),
		},
		"/api/admin/system/backups": map[string]any{
			"get": op([]string{"Admin"}, "Get backup status", "Returns automatic backup configuration and recent backup status.", secCookie(), nil, nil,
				map[string]any{"200": jsonRefResp("System backup status", "#/components/schemas/SystemBackupStatus")}),
		},
		"/api/admin/system/spam-filter": map[string]any{
			"get": op([]string{"Admin"}, "Get spam filter status", "Returns spam database path, rule count, auto-update state, last refresh time, and last error.", secCookie(), nil, nil,
				map[string]any{"200": jsonRefResp("Spam filter status", "#/components/schemas/SystemSpamStatus")}),
		},
		"/api/admin/system/spam-filter/refresh": map[string]any{
			"post": op([]string{"Admin"}, "Refresh spam database", "Downloads and rebuilds the spam database from the configured feeds. Writes an instance audit log entry with the actor and outcome.", secCookie(), nil, nil,
				map[string]any{
					"200": jsonRefResp("Refresh result", "#/components/schemas/SystemActionResponse"),
					"503": errResp("Spam filter not available"),
				}),
		},
		"/api/admin/system/import-stage-cleanup": map[string]any{
			"get": op([]string{"Admin"}, "Get import staging cleanup status", "Returns stale staged import file counts, byte totals, retention policy, and the latest cleanup run state.", secCookie(), nil, nil,
				map[string]any{
					"200": jsonRefResp("Import staging cleanup status", "#/components/schemas/SystemImportStageCleanupStatus"),
					"503": errResp("Store not available"),
				}),
		},
		"/api/admin/system/import-stage-cleanup/run": map[string]any{
			"post": op([]string{"Admin"}, "Run import staging cleanup", "Removes stale staged import upload files immediately and writes an instance audit log entry with the actor and outcome.", secCookie(), nil, nil,
				map[string]any{
					"200": jsonRefResp("Import staging cleanup result", "#/components/schemas/SystemImportStageCleanupRunResponse"),
					"409": errResp("Cleanup disabled"),
					"500": jsonRefResp("Cleanup failed", "#/components/schemas/SystemImportStageCleanupRunResponse"),
					"503": errResp("Store not available"),
				}),
		},
		"/api/admin/system/caches": map[string]any{
			"get": op([]string{"Admin"}, "Get cache status", "Returns LRU cache sizes, maximum sizes, TTLs, and pressure status for permissions, API clients, and API rate limiting.", secCookie(), nil, nil,
				map[string]any{"200": jsonRefResp("System cache status", "#/components/schemas/SystemCacheStatus")}),
		},
		"/api/admin/system/mail": map[string]any{
			"get": op([]string{"Admin"}, "Get mail status", "Returns configured mail driver, host, port, encryption, sender identity, masked username, password presence, and last test result.", secCookie(), nil, nil,
				map[string]any{"200": jsonRefResp("System mail status", "#/components/schemas/SystemMailStatus")}),
		},
		"/api/admin/system/mail/test": map[string]any{
			"post": op([]string{"Admin"}, "Send test email", "Sends a real test email to the specified recipient using the configured mail transport. Writes an instance audit log entry with the actor and outcome.", secCookie(), nil,
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"email": map[string]any{"type": "string", "format": "email"}}}),
				map[string]any{
					"200": jsonRefResp("Mail test result", "#/components/schemas/SystemActionResponse"),
					"400": errResp("Invalid recipient"),
					"503": errResp("Mailer not configured"),
				}),
		},
		"/api/admin/system/audit": map[string]any{
			"get": op([]string{"Admin"}, "List instance audit log", "Lists instance-level audit entries for system maintenance and admin operations. Malformed actor, date, limit, and offset filters return 400.", secCookie(), instanceAuditQueryParams(false), nil,
				map[string]any{
					"200": jsonRefResp("Instance audit entries", "#/components/schemas/InstanceAuditListResponse"),
					"400": errResp("Invalid audit filter"),
				}),
		},
		"/api/admin/system/audit/export": map[string]any{
			"get": op([]string{"Admin"}, "Export instance audit log", "Exports matching instance-level audit entries as JSON or CSV. The export limit defaults to 10000 rows and is capped at 50000 rows.", secCookie(), instanceAuditQueryParams(true), nil,
				map[string]any{
					"200": desc("Audit export"),
					"400": errResp("Invalid audit filter"),
					"403": errResp("Forbidden"),
				}),
		},
		"/api/admin/system/activation": map[string]any{
			"get": op([]string{"Admin"}, "List user activation", "Owner-only operational activation view for teams and sites. Returns setup status, aggregate hit/event counts, owner email, plan, and timestamps without visitor-level details.", secCookie(), activationQueryParams(), nil,
				map[string]any{
					"200": jsonRefResp("Activation rows", "#/components/schemas/SystemActivationResponse"),
					"400": errResp("Invalid activation filter"),
					"403": errResp("Forbidden"),
				}),
		},
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
		"/api/sites/{id}/tracking/status": map[string]any{
			"get": op([]string{"Sites"}, "Get tracking status", "Returns privacy-safe operational tracking metadata for a site, including live/waiting/dormant/domain-mismatch status and the latest accepted tracker source.", secCookie(), []any{paramRef("#/components/parameters/siteID")}, nil,
				map[string]any{
					"200": jsonRefResp("Tracking status", "#/components/schemas/SiteTrackingStatus"),
					"404": errResp("Site not found"),
				}),
		},
		"/api/sites/{id}/stats": map[string]any{
			"get": op([]string{"Sites"}, "Get site stats", "Aggregated site metrics and charts.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/from"), paramRef("#/components/parameters/to"),
				paramRef("#/components/parameters/filter"), paramRef("#/components/parameters/filterType"), paramRef("#/components/parameters/filterValue"),
				paramRef("#/components/parameters/goalIDQuery"), paramRef("#/components/parameters/funnelIDQuery"),
			}, nil, map[string]any{"200": jsonRefResp("Site stats", "#/components/schemas/SiteStats")}),
		},
		"/api/sites/{id}/importers": map[string]any{
			"get": op([]string{"Imports"}, "List site importers", "Lists available historical data import providers for the selected site.", secAnyAuth(), []any{paramRef("#/components/parameters/siteID")}, nil,
				map[string]any{"200": jsonSchemaResp("Importers", map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]any{
							"key":                 map[string]any{"type": "string"},
							"name":                map[string]any{"type": "string"},
							"accepted_extensions": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
							"capabilities":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
						"required": []string{"key", "name", "accepted_extensions", "capabilities"},
					},
				})}),
		},
		"/api/sites/{id}/imports/{provider}/uploads": map[string]any{
			"post": op([]string{"Imports"}, "Create import upload", "Creates a staged import upload session. Use chunk upload endpoints to send the declared ZIP or CSV files.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"),
				map[string]any{"name": "provider", "in": "path", "required": true, "schema": map[string]any{"type": "string", "enum": []string{"plausible", "simpleanalytics"}}},
			}, jsonBody(map[string]any{"type": "object", "properties": map[string]any{"files": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"filename": map[string]any{"type": "string"}, "size_bytes": map[string]any{"type": "integer"}, "sha256": map[string]any{"type": "string", "description": "Optional lowercase or uppercase hex SHA-256 checksum for the staged file."}}, "required": []string{"filename", "size_bytes"}}}}, "required": []string{"files"}}),
				map[string]any{"200": jsonSchemaResp("Upload session", map[string]any{"type": "object", "additionalProperties": true}), "400": errResp("Invalid upload"), "413": errResp("Import exceeds maximum staged size")}),
		},
		"/api/sites/{id}/imports/uploads/{importID}/files/{fileID}/chunks": map[string]any{
			"put": op([]string{"Imports"}, "Upload import chunk", "Uploads one raw file chunk at the requested byte offset.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"),
				map[string]any{"name": "importID", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
				map[string]any{"name": "fileID", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
				map[string]any{"name": "offset", "in": "query", "required": true, "schema": map[string]any{"type": "integer", "minimum": 0}},
			}, map[string]any{"required": true, "content": map[string]any{"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}}},
				map[string]any{"200": jsonSchemaResp("Chunk progress", map[string]any{"type": "object", "additionalProperties": true}), "409": errResp("Upload not accepting chunks")}),
		},
		"/api/sites/{id}/imports/uploads/{importID}/validate": map[string]any{
			"post": op([]string{"Imports"}, "Validate import upload", "Scans staged files row by row and returns the validation manifest without committing analytics rows.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"),
				map[string]any{"name": "importID", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
			}, nil, map[string]any{"200": jsonRefResp("Import job", "#/components/schemas/ImportJob"), "400": errResp("Validation failed")}),
		},
		"/api/sites/{id}/imports": map[string]any{
			"get": op([]string{"Imports"}, "List site imports", "Lists import jobs for the site.", secAnyAuth(), []any{paramRef("#/components/parameters/siteID")}, nil,
				map[string]any{"200": jsonSchemaResp("Import list", map[string]any{"type": "object", "properties": map[string]any{"imports": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/ImportJob"}}}})}),
		},
		"/api/sites/{id}/imports/{importID}": map[string]any{
			"get": op([]string{"Imports"}, "Get import", "Returns import status, files, and validation manifest.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"),
				map[string]any{"name": "importID", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
			}, nil, map[string]any{"200": jsonRefResp("Import job", "#/components/schemas/ImportJob")}),
			"delete": op([]string{"Imports"}, "Delete import", "Deletes imported aggregate rows for the selected import and marks its history row deleted.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"),
				map[string]any{"name": "importID", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
			}, nil, map[string]any{"200": jsonRefResp("Status", "#/components/schemas/Status"), "409": errResp("Import still running")}),
		},
		"/api/sites/{id}/imports/{importID}/start": map[string]any{
			"post": op([]string{"Imports"}, "Start validated import", "Starts a previously validated import job in the background.", secAnyAuth(), []any{
				paramRef("#/components/parameters/siteID"),
				map[string]any{"name": "importID", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
			}, nil, map[string]any{"200": jsonRefResp("Import job", "#/components/schemas/ImportJob"), "409": errResp("Import is not validated")}),
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
		"/api/sites/{id}/events/names": map[string]any{
			"get": op([]string{"Sites"}, "List event names", "Lists custom and automatic event names observed for a site in the selected date range.", secAnyAuth(), eventRangeParams(paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"200": jsonSchemaResp("Event names", map[string]any{"type": "array", "items": map[string]any{"type": "string"}})}),
		},
		"/api/sites/{id}/events/properties": map[string]any{
			"get": op([]string{"Sites"}, "List event property keys", "Lists JSON property keys observed for an event name in the selected date range.", secAnyAuth(), eventNameParams(paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"200": jsonSchemaResp("Event property keys", map[string]any{"type": "array", "items": map[string]any{"type": "string"}})}),
		},
		"/api/sites/{id}/events/breakdown": map[string]any{
			"get": op([]string{"Sites"}, "Get event property breakdown", "Returns distinct property values for one event property key, ordered by session count.", secAnyAuth(), append(eventNameParams(paramRef("#/components/parameters/siteID")), paramRef("#/components/parameters/eventPropertyKeyRequired")), nil,
				map[string]any{"200": jsonSchemaResp("Event property breakdown", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}})}),
		},
		"/api/sites/{id}/events/timeseries": map[string]any{
			"get": op([]string{"Sites"}, "Get event timeseries", "Returns event occurrence counts over time. Optional property filters and repeatable filter=type:value hit-dimension filters restrict the sessions counted.", secAnyAuth(), eventFilteredParams(paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"200": jsonSchemaResp("Event timeseries", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/EventSeriesPoint"}})}),
		},
		"/api/sites/{id}/events/audience": map[string]any{
			"get": op([]string{"Sites"}, "Get event audience", "Returns top pages, referrers, devices, and countries for sessions containing the selected event. Optional property filters and repeatable filter=type:value hit-dimension filters restrict the sessions included.", secAnyAuth(), eventFilteredParams(paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"200": jsonRefResp("Event audience", "#/components/schemas/EventAudience")}),
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
			"get": op([]string{"Sites"}, "List site exclusions", "Lists per-site IP/CIDR exclusions used by ingest filtering. Requires site data-control permission or the narrow instance site-exclusion permission.", secCookie(), []any{paramRef("#/components/parameters/siteID")}, nil,
				map[string]any{"200": jsonSchemaResp("Site exclusions", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/IPExclusion"}})}),
			"post": op([]string{"Sites"}, "Create site exclusion", "Creates per-site IP/CIDR exclusion rule. Requires site data-control permission or the narrow instance site-exclusion permission.", secCookie(), []any{paramRef("#/components/parameters/siteID")},
				jsonBody(map[string]any{"$ref": "#/components/schemas/IPExclusionCreateRequest"}),
				map[string]any{"201": jsonRefResp("Created exclusion", "#/components/schemas/IPExclusion"), "400": errResp("Invalid IP/CIDR")}),
		},
		"/api/sites/{id}/exclusions/{ruleID}": map[string]any{
			"delete": op([]string{"Sites"}, "Delete site exclusion", "Deletes a per-site IP/CIDR exclusion rule. Requires site data-control permission or the narrow instance site-exclusion permission.", secCookie(), []any{paramRef("#/components/parameters/siteID"), paramRef("#/components/parameters/ruleID")}, nil,
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
		"/api/share/{token}/sites/{id}/events/names": map[string]any{
			"get": op([]string{"Share"}, "Shared event names", "Lists event names through share token.", nil, eventRangeParams(paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"200": jsonSchemaResp("Event names", map[string]any{"type": "array", "items": map[string]any{"type": "string"}})}),
		},
		"/api/share/{token}/sites/{id}/events/properties": map[string]any{
			"get": op([]string{"Share"}, "Shared event property keys", "Lists event property keys through share token.", nil, eventNameParams(paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"200": jsonSchemaResp("Event property keys", map[string]any{"type": "array", "items": map[string]any{"type": "string"}})}),
		},
		"/api/share/{token}/sites/{id}/events/breakdown": map[string]any{
			"get": op([]string{"Share"}, "Shared event property breakdown", "Returns event property value breakdown through share token.", nil, append(eventNameParams(paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID")), paramRef("#/components/parameters/eventPropertyKeyRequired")), nil,
				map[string]any{"200": jsonSchemaResp("Event property breakdown", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/MetricStat"}})}),
		},
		"/api/share/{token}/sites/{id}/events/timeseries": map[string]any{
			"get": op([]string{"Share"}, "Shared event timeseries", "Returns event timeseries through share token. Optional property filters and repeatable filter=type:value hit-dimension filters restrict the sessions counted.", nil, eventFilteredParams(paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"200": jsonSchemaResp("Event timeseries", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/EventSeriesPoint"}})}),
		},
		"/api/share/{token}/sites/{id}/events/audience": map[string]any{
			"get": op([]string{"Share"}, "Shared event audience", "Returns event audience through share token. Optional property filters and repeatable filter=type:value hit-dimension filters restrict the sessions included.", nil, eventFilteredParams(paramRef("#/components/parameters/token"), paramRef("#/components/parameters/siteID")), nil,
				map[string]any{"200": jsonRefResp("Event audience", "#/components/schemas/EventAudience")}),
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

func activationQueryParams() []any {
	return []any{
		map[string]any{"name": "status", "in": "query", "schema": map[string]any{"type": "string", "enum": []string{"waiting", "live", "dormant", "domain_mismatch"}}},
		map[string]any{"name": "team", "in": "query", "schema": map[string]any{"type": "string"}},
		map[string]any{"name": "domain", "in": "query", "schema": map[string]any{"type": "string"}},
		map[string]any{"name": "last_seen_from", "in": "query", "schema": map[string]any{"type": "string", "format": "date-time"}},
		map[string]any{"name": "last_seen_to", "in": "query", "schema": map[string]any{"type": "string", "format": "date-time"}},
		paramRef("#/components/parameters/limit"),
		paramRef("#/components/parameters/offset"),
	}
}

func instanceAuditQueryParams(includeFormat bool) []any {
	params := []any{
		map[string]any{"name": "action", "in": "query", "schema": map[string]any{"type": "string"}, "description": "Optional exact action filter."},
		map[string]any{"name": "target_type", "in": "query", "schema": map[string]any{"type": "string"}, "description": "Optional target type filter."},
		map[string]any{"name": "outcome", "in": "query", "schema": map[string]any{"type": "string"}, "description": "Optional outcome filter, for example success or failure."},
		map[string]any{"name": "actor_id", "in": "query", "schema": map[string]any{"type": "string", "format": "uuid"}, "description": "Optional actor user ID filter."},
		map[string]any{"name": "from", "in": "query", "schema": map[string]any{"type": "string", "format": "date-time"}, "description": "Optional RFC3339 lower time bound."},
		map[string]any{"name": "to", "in": "query", "schema": map[string]any{"type": "string", "format": "date-time"}, "description": "Optional RFC3339 upper time bound."},
		map[string]any{"name": "query", "in": "query", "schema": map[string]any{"type": "string"}, "description": "Optional free-text search over action, actor, target, outcome, IP, request ID, and details."},
	}
	if includeFormat {
		params = append(params,
			map[string]any{"name": "format", "in": "query", "schema": map[string]any{"type": "string", "enum": []string{"json", "csv"}}, "description": "Export format. Defaults to json."},
			map[string]any{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 1, "maximum": 50000}, "description": "Maximum number of exported rows. Defaults to 10000."},
		)
	} else {
		params = append(params,
			map[string]any{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 1, "maximum": 200}, "description": "Maximum number of rows to return."},
			map[string]any{"name": "offset", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 0}, "description": "Zero-based row offset."},
		)
	}
	return params
}
