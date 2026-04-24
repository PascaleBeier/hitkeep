package system

import "hitkeep/internal/exportfmt"

func openAPIV1Parameters() map[string]any {
	return map[string]any{
		"siteID":          map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
		"goalID":          map[string]any{"name": "goalID", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
		"funnelID":        map[string]any{"name": "funnelID", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
		"shareID":         map[string]any{"name": "shareID", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
		"ruleID":          map[string]any{"name": "ruleID", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
		"userID":          map[string]any{"name": "userId", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
		"teamID":          map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
		"adminUserID":     map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
		"apiClientID":     map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
		"teamAPIClientID": map[string]any{"name": "clientId", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
		"passkeyID":       map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
		"token":           map[string]any{"name": "token", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
		"domain":          map[string]any{"name": "domain", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
		"from":            map[string]any{"name": "from", "in": "query", "schema": map[string]any{"type": "string", "format": "date-time"}},
		"to":              map[string]any{"name": "to", "in": "query", "schema": map[string]any{"type": "string", "format": "date-time"}},
		"limit":           map[string]any{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}},
		"offset":          map[string]any{"name": "offset", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 0}},
		"query":           map[string]any{"name": "q", "in": "query", "schema": map[string]any{"type": "string"}},
		"sort":            map[string]any{"name": "sort", "in": "query", "schema": map[string]any{"type": "string"}},
		"order":           map[string]any{"name": "order", "in": "query", "schema": map[string]any{"type": "string", "enum": []string{"asc", "desc"}}},
		"filter":          map[string]any{"name": "filter", "in": "query", "description": "Filter in form type:value (repeatable).", "schema": map[string]any{"type": "string"}},
		"filterType":      map[string]any{"name": "filter_type", "in": "query", "schema": map[string]any{"type": "string"}},
		"filterValue":     map[string]any{"name": "filter_value", "in": "query", "schema": map[string]any{"type": "string"}},
		"itemID":          map[string]any{"name": "item_id", "in": "query", "schema": map[string]any{"type": "string"}},
		"itemName":        map[string]any{"name": "item_name", "in": "query", "schema": map[string]any{"type": "string"}},
		"goalIDQuery":     map[string]any{"name": "goal_id", "in": "query", "schema": map[string]any{"type": "string", "format": "uuid"}},
		"funnelIDQuery":   map[string]any{"name": "funnel_id", "in": "query", "schema": map[string]any{"type": "string", "format": "uuid"}},
		"format": map[string]any{
			"name":        "format",
			"in":          "query",
			"description": "Export format. Supported values: xlsx, csv, parquet, json, ndjson. Defaults: xlsx for takeout endpoints, csv for hits export endpoints.",
			"schema":      map[string]any{"type": "string", "enum": exportfmt.SupportedFormats()},
		},
		"avatarSize":   map[string]any{"name": "s", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 32, "maximum": 256}},
		"reportSiteID": map[string]any{"name": "site_id", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}, "description": "Site UUID."},
	}
}
