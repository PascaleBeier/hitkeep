package system

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"hitkeep/internal/exportfmt"
)

func TestOpenAPISpecV1FormatParameterIncludesAllExportFormats(t *testing.T) {
	spec := openAPISpecV1("http://localhost:8080")

	components := requireMap(t, spec, "components")
	parameters := requireMap(t, components, "parameters")
	formatParam := requireMap(t, parameters, "format")
	schema := requireMap(t, formatParam, "schema")

	gotFormats := asStringSlice(t, schema["enum"])
	wantFormats := exportfmt.SupportedFormats()
	if !reflect.DeepEqual(gotFormats, wantFormats) {
		t.Fatalf("unexpected format enum, got %v want %v", gotFormats, wantFormats)
	}

	description, ok := formatParam["description"].(string)
	if !ok {
		t.Fatalf("expected format parameter description to be string, got %T", formatParam["description"])
	}
	if !strings.Contains(description, "json") || !strings.Contains(description, "ndjson") {
		t.Fatalf("format parameter description should mention json/ndjson, got %q", description)
	}
}

func TestOpenAPISpecV1TakeoutAndExportPathsListAllFormats(t *testing.T) {
	spec := openAPISpecV1("http://localhost:8080")
	paths := requireMap(t, spec, "paths")

	tests := []struct {
		path             string
		expectedContains []string
	}{
		{
			path:             "/api/user/takeout",
			expectedContains: []string{"xlsx", "csv", "parquet", "json", "ndjson"},
		},
		{
			path:             "/api/sites/{id}/takeout",
			expectedContains: []string{"xlsx", "csv", "parquet", "json", "ndjson"},
		},
		{
			path:             "/api/sites/{id}/hits/export",
			expectedContains: []string{"xlsx", "csv", "parquet", "json", "ndjson"},
		},
		{
			path:             "/api/share/{token}/sites/{id}/hits/export",
			expectedContains: []string{"xlsx", "csv", "parquet", "json", "ndjson"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			pathItem := requireMap(t, paths, tc.path)
			getOp := requireMap(t, pathItem, "get")

			description, ok := getOp["description"].(string)
			if !ok {
				t.Fatalf("expected description for %s to be string, got %T", tc.path, getOp["description"])
			}

			for _, expected := range tc.expectedContains {
				if !strings.Contains(description, expected) {
					t.Fatalf("expected description for %s to contain %q, got %q", tc.path, expected, description)
				}
			}

			parameters, ok := getOp["parameters"].([]any)
			if !ok {
				t.Fatalf("expected parameters for %s to be []any, got %T", tc.path, getOp["parameters"])
			}
			if !hasFormatParamRef(parameters) {
				t.Fatalf("expected parameters for %s to include format parameter ref", tc.path)
			}
		})
	}
}

func TestOpenAPISpecV1EcommerceSummaryDocumentsGeoNetworkAggregates(t *testing.T) {
	spec := openAPISpecV1("http://localhost:8080")
	paths := requireMap(t, spec, "paths")
	components := requireMap(t, spec, "components")
	schemas := requireMap(t, components, "schemas")

	pathItem := requireMap(t, paths, "/api/sites/{id}/ecommerce")
	getOp := requireMap(t, pathItem, "get")
	description, ok := getOp["description"].(string)
	if !ok {
		t.Fatalf("expected ecommerce description to be string, got %T", getOp["description"])
	}
	for _, want := range []string{"city", "provider", "ASN"} {
		if !strings.Contains(description, want) {
			t.Fatalf("expected ecommerce description to mention %q, got %q", want, description)
		}
	}

	summarySchema := requireMap(t, schemas, "EcommerceSummary")
	properties := requireMap(t, summarySchema, "properties")
	for _, prop := range []string{"top_cities", "top_providers", "top_asns"} {
		if _, ok := properties[prop]; !ok {
			t.Fatalf("expected EcommerceSummary to include %s", prop)
		}
	}
}

func TestOpenAPISpecV1BrowserIngestDocumentsGeoNetworkBoundary(t *testing.T) {
	spec := openAPISpecV1("http://localhost:8080")
	paths := requireMap(t, spec, "paths")

	pathItem := requireMap(t, paths, "/ingest")
	postOp := requireMap(t, pathItem, "post")
	description, ok := postOp["description"].(string)
	if !ok {
		t.Fatalf("expected browser ingest description to be string, got %T", postOp["description"])
	}
	for _, want := range []string{
		"resolved request IP",
		"exclusions, spam filtering, and country, region, city, provider, and ASN lookup",
		"derived country, region, city, provider, and ASN",
		"does not store the raw visitor IP",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("expected browser ingest description to mention %q, got %q", want, description)
		}
	}
	if strings.Contains(description, "geolocation") {
		t.Fatalf("expected browser ingest description to avoid generic geolocation wording, got %q", description)
	}
}

func TestOpenAPISpecV1TeamSchemasExposeUsageAndEntitlements(t *testing.T) {
	spec := openAPISpecV1("http://localhost:8080")
	components := requireMap(t, spec, "components")
	schemas := requireMap(t, components, "schemas")

	teamSchema := requireMap(t, schemas, "Team")
	teamProperties := requireMap(t, teamSchema, "properties")

	if _, ok := teamProperties["usage"]; !ok {
		t.Fatalf("expected Team schema to include usage")
	}
	if _, ok := teamProperties["entitlements"]; !ok {
		t.Fatalf("expected Team schema to include entitlements")
	}
	if _, ok := teamProperties["plan"]; !ok {
		t.Fatalf("expected Team schema to include plan")
	}

	if _, ok := schemas["TeamUsageSummary"]; !ok {
		t.Fatalf("expected TeamUsageSummary schema to exist")
	}
	if _, ok := schemas["TeamEntitlements"]; !ok {
		t.Fatalf("expected TeamEntitlements schema to exist")
	}
	if _, ok := schemas["TeamPlan"]; !ok {
		t.Fatalf("expected TeamPlan schema to exist")
	}
	if _, ok := schemas["CloudStatus"]; !ok {
		t.Fatalf("expected CloudStatus schema to exist")
	}
}

func TestOpenAPISpecV1IncludesCloudSignupPaths(t *testing.T) {
	spec := openAPISpecV1("http://localhost:8080")
	tags, ok := spec["tags"].([]map[string]string)
	if !ok {
		t.Fatalf("expected tags to be []map[string]string, got %T", spec["tags"])
	}
	if !hasTag(tags, "Cloud") {
		t.Fatalf("expected top-level Cloud tag to exist")
	}

	paths := requireMap(t, spec, "paths")

	signupPath, ok := paths["/api/cloud/signup"]
	if !ok {
		t.Fatalf("expected /api/cloud/signup path to exist")
	}
	portalPath, ok := paths["/api/cloud/billing/portal"]
	if !ok {
		t.Fatalf("expected /api/cloud/billing/portal path to exist")
	}
	checkoutPath, ok := paths["/api/cloud/billing/checkout"]
	if !ok {
		t.Fatalf("expected /api/cloud/billing/checkout path to exist")
	}
	webhookPath, ok := paths["/api/cloud/webhooks/stripe"]
	if !ok {
		t.Fatalf("expected /api/cloud/webhooks/stripe path to exist")
	}

	assertCloudOperation(t, requireMap(t, signupPath.(map[string]any), "post"))
	assertCloudOperation(t, requireMap(t, portalPath.(map[string]any), "post"))
	assertCloudOperation(t, requireMap(t, checkoutPath.(map[string]any), "post"))
	assertCloudOperation(t, requireMap(t, webhookPath.(map[string]any), "post"))
}

func TestOpenAPISpecV1IncludesAdminSystemPaths(t *testing.T) {
	spec := openAPISpecV1("http://localhost:8080")
	paths := requireMap(t, spec, "paths")
	components := requireMap(t, spec, "components")
	schemas := requireMap(t, components, "schemas")

	expectedSchemas := []string{
		"SystemFeatureStatus",
		"SystemInfo",
		"SystemHealth",
		"SystemAIStatus",
		"SystemSearchConsoleStatus",
		"SystemStorage",
		"SystemIngestStats",
		"SystemBackupStatus",
		"SystemSpamStatus",
		"SystemCacheStatus",
		"SystemMailStatus",
		"SystemActionResponse",
		"InstanceAuditEntry",
		"InstanceAuditListResponse",
	}
	for _, schemaName := range expectedSchemas {
		if _, ok := schemas[schemaName]; !ok {
			t.Fatalf("expected %s schema to exist", schemaName)
		}
	}
	aiStatusSchema := requireMap(t, schemas, "SystemAIStatus")
	aiStatusProps := requireMap(t, aiStatusSchema, "properties")
	configModeSchema := requireMap(t, aiStatusProps, "config_mode")
	configModeEnum := asStringSlice(t, configModeSchema["enum"])
	for _, want := range []string{"cloud_managed", "self_hosted"} {
		if !slices.Contains(configModeEnum, want) {
			t.Fatalf("expected SystemAIStatus.config_mode enum to include %q, got %v", want, configModeEnum)
		}
	}

	expectedPaths := []string{
		"/api/admin/system",
		"/api/admin/system/health",
		"/api/admin/system/ai",
		"/api/admin/system/search-console",
		"/api/admin/system/storage",
		"/api/admin/system/ingest",
		"/api/admin/system/backups",
		"/api/admin/system/spam-filter",
		"/api/admin/system/spam-filter/refresh",
		"/api/admin/system/caches",
		"/api/admin/system/mail",
		"/api/admin/system/mail/test",
		"/api/admin/system/audit",
		"/api/admin/system/audit/export",
		"/api/admin/teams",
		"/api/admin/teams/{id}",
		"/api/admin/teams/{id}/archive",
	}
	for _, path := range expectedPaths {
		if _, ok := paths[path]; !ok {
			t.Fatalf("expected %s path to exist", path)
		}
	}

	adminTeamsSchema := requireMap(t, schemas, "AdminTeam")
	adminTeamsProps := requireMap(t, adminTeamsSchema, "properties")
	for _, prop := range []string{"id", "name", "is_default", "is_archived", "member_count", "site_count", "created_at"} {
		if _, ok := adminTeamsProps[prop]; !ok {
			t.Fatalf("expected AdminTeam.%s property to exist", prop)
		}
	}

	mailTestPath := requireMap(t, paths, "/api/admin/system/mail/test")
	postOp := requireMap(t, mailTestPath, "post")
	if !strings.Contains(postOp["description"].(string), "real test email") {
		t.Fatalf("expected mail test description to mention real test email, got %q", postOp["description"])
	}

	ingestPath := requireMap(t, paths, "/api/admin/system/ingest")
	ingestOp := requireMap(t, ingestPath, "get")
	if !strings.Contains(ingestOp["description"].(string), "tenant analytics databases") {
		t.Fatalf("expected ingest description to mention tenant databases, got %q", ingestOp["description"])
	}

	auditPath := requireMap(t, paths, "/api/admin/system/audit")
	auditOp := requireMap(t, auditPath, "get")
	auditResponses := requireMap(t, auditOp, "responses")
	if _, ok := auditResponses["400"]; !ok {
		t.Fatalf("expected audit list to document invalid filter response")
	}

	auditExportPath := requireMap(t, paths, "/api/admin/system/audit/export")
	getOp := requireMap(t, auditExportPath, "get")
	params, ok := getOp["parameters"].([]any)
	if !ok {
		t.Fatalf("expected audit export parameters to be []any, got %T", getOp["parameters"])
	}
	if !hasNamedParam(params, "format") {
		t.Fatalf("expected audit export to include format parameter")
	}
	if !hasNamedParam(params, "limit") {
		t.Fatalf("expected audit export to include limit parameter")
	}
	exportResponses := requireMap(t, getOp, "responses")
	if _, ok := exportResponses["400"]; !ok {
		t.Fatalf("expected audit export to document invalid filter response")
	}
	if _, ok := exportResponses["403"]; !ok {
		t.Fatalf("expected audit export to document owner-only response")
	}
}

func TestOpenAPISpecV1IncludesOpportunityPaths(t *testing.T) {
	spec := openAPISpecV1("http://localhost:8080")
	paths := requireMap(t, spec, "paths")
	components := requireMap(t, spec, "components")
	schemas := requireMap(t, components, "schemas")

	for _, schemaName := range []string{"Opportunity", "OpportunityEvidence", "OpportunityScoreBreakdown", "OpportunityListResponse", "SharedOpportunity", "SharedOpportunityListResponse", "OpportunityGenerateResponse", "OpportunityDigestPreviewResponse", "OpportunityDigestPreviewItem", "OpportunityStatusUpdateRequest"} {
		if _, ok := schemas[schemaName]; !ok {
			t.Fatalf("expected %s schema to exist", schemaName)
		}
	}
	opportunitySchema := requireMap(t, schemas, "Opportunity")
	opportunityProperties := requireMap(t, opportunitySchema, "properties")
	for _, forbidden := range []string{"title", "summary", "next_action", "impact_label", "route_label", "plan"} {
		if _, ok := opportunityProperties[forbidden]; ok {
			t.Fatalf("Opportunity schema leaked prose field %q", forbidden)
		}
	}
	for _, required := range []string{"type_key", "title_key", "summary_key", "action_key", "copy_params", "impact_label_key", "route_label_key", "score_breakdown", "cited_evidence_ids"} {
		if _, ok := opportunityProperties[required]; !ok {
			t.Fatalf("Opportunity schema missing localization field %q", required)
		}
	}
	evidenceSchema := requireMap(t, schemas, "OpportunityEvidence")
	evidenceProperties := requireMap(t, evidenceSchema, "properties")
	if _, ok := evidenceProperties["label"]; ok {
		t.Fatalf("OpportunityEvidence schema leaked prose label")
	}
	if _, ok := evidenceProperties["label_key"]; !ok {
		t.Fatalf("OpportunityEvidence schema missing label_key")
	}
	for _, forbidden := range []string{"team_id", "ai_run_id"} {
		if _, ok := opportunityProperties[forbidden]; ok {
			t.Fatalf("Opportunity schema leaked internal field %q", forbidden)
		}
	}
	generateResponseSchema := requireMap(t, schemas, "OpportunityGenerateResponse")
	generateResponseProperties := requireMap(t, generateResponseSchema, "properties")
	if _, ok := generateResponseProperties["ai_run_id"]; ok {
		t.Fatalf("OpportunityGenerateResponse schema leaked internal ai_run_id")
	}
	digestPreviewItemSchema := requireMap(t, schemas, "OpportunityDigestPreviewItem")
	digestPreviewItemProperties := requireMap(t, digestPreviewItemSchema, "properties")
	for _, forbidden := range []string{"title", "summary", "digest", "action", "team_id", "ai_run_id", "raw_prompt", "raw_provider_response"} {
		if _, ok := digestPreviewItemProperties[forbidden]; ok {
			t.Fatalf("OpportunityDigestPreviewItem schema leaked forbidden field %q", forbidden)
		}
	}
	for _, required := range []string{"title_key", "action_key", "digest_key", "copy_params", "impact_label_key", "score_breakdown", "evidence", "cited_evidence_ids"} {
		if _, ok := digestPreviewItemProperties[required]; !ok {
			t.Fatalf("OpportunityDigestPreviewItem schema missing localization field %q", required)
		}
	}
	sharedOpportunitySchema := requireMap(t, schemas, "SharedOpportunity")
	sharedOpportunityProperties := requireMap(t, sharedOpportunitySchema, "properties")
	for _, forbidden := range []string{"team_id", "ai_run_id"} {
		if _, ok := sharedOpportunityProperties[forbidden]; ok {
			t.Fatalf("SharedOpportunity schema leaked internal field %q", forbidden)
		}
	}

	expected := map[string]string{
		"/api/sites/{id}/opportunities":                 "get",
		"/api/sites/{id}/opportunities/digest-preview":  "get",
		"/api/sites/{id}/opportunities/generate":        "post",
		"/api/sites/{id}/opportunities/{opportunityID}": "patch",
		"/api/share/{token}/sites/{id}/opportunities":   "get",
	}
	for path, method := range expected {
		pathItem := requireMap(t, paths, path)
		if _, ok := pathItem[method]; !ok {
			t.Fatalf("expected %s %s in OpenAPI paths", method, path)
		}
	}

	generatePath := requireMap(t, paths, "/api/sites/{id}/opportunities/generate")
	postOp := requireMap(t, generatePath, "post")
	if !strings.Contains(postOp["description"].(string), "deterministic opportunity detectors") {
		t.Fatalf("expected generate description to mention deterministic detectors, got %q", postOp["description"])
	}
}

func TestOpenAPISpecV1IncludesAIFetchCorrelationPath(t *testing.T) {
	spec := openAPISpecV1("http://localhost:8080")
	paths := requireMap(t, spec, "paths")
	components := requireMap(t, spec, "components")
	schemas := requireMap(t, components, "schemas")

	pathItem := requireMap(t, paths, "/api/sites/{id}/ai-fetch/correlation")
	getOp := requireMap(t, pathItem, "get")
	responses := requireMap(t, getOp, "responses")
	okResp := requireMap(t, responses, "200")
	content := requireMap(t, okResp, "content")
	jsonContent := requireMap(t, content, "application/json")
	schema := requireMap(t, jsonContent, "schema")

	if ref, _ := schema["$ref"].(string); ref != "#/components/schemas/AIFetchCorrelationReport" {
		t.Fatalf("expected AI fetch correlation schema ref, got %q", ref)
	}
	if _, ok := schemas["AIFetchCorrelationReport"]; !ok {
		t.Fatalf("expected AIFetchCorrelationReport schema to exist")
	}
}

func TestOpenAPISpecV1IncludesAIFetchEndpointsAndSchemas(t *testing.T) {
	spec := openAPISpecV1("http://localhost:8080")
	paths := requireMap(t, spec, "paths")
	components := requireMap(t, spec, "components")
	schemas := requireMap(t, components, "schemas")

	for _, schemaName := range []string{
		"AIFetch",
		"AIFetchIngestPayload",
		"AIFetchOverview",
		"AIFetchSeriesPoint",
	} {
		if _, ok := schemas[schemaName]; !ok {
			t.Fatalf("expected %s schema to exist", schemaName)
		}
	}

	ingestPath := requireMap(t, paths, "/api/sites/{id}/ingest/ai-fetch")
	postOp := requireMap(t, ingestPath, "post")
	requestBody := requireMap(t, postOp, "requestBody")
	content := requireMap(t, requestBody, "content")
	jsonContent := requireMap(t, content, "application/json")
	requestSchema := requireMap(t, jsonContent, "schema")
	if ref, _ := requestSchema["$ref"].(string); ref != "#/components/schemas/AIFetchIngestPayload" {
		t.Fatalf("expected AI fetch ingest payload schema ref, got %q", ref)
	}

	overviewPath := requireMap(t, paths, "/api/sites/{id}/ai-fetch/overview")
	overviewOp := requireMap(t, overviewPath, "get")
	overviewResponses := requireMap(t, overviewOp, "responses")
	overviewOK := requireMap(t, overviewResponses, "200")
	overviewContent := requireMap(t, overviewOK, "content")
	overviewJSON := requireMap(t, overviewContent, "application/json")
	overviewSchema := requireMap(t, overviewJSON, "schema")
	if ref, _ := overviewSchema["$ref"].(string); ref != "#/components/schemas/AIFetchOverview" {
		t.Fatalf("expected AI fetch overview schema ref, got %q", ref)
	}

	timeseriesPath := requireMap(t, paths, "/api/sites/{id}/ai-fetch/timeseries")
	timeseriesOp := requireMap(t, timeseriesPath, "get")
	timeseriesResponses := requireMap(t, timeseriesOp, "responses")
	timeseriesOK := requireMap(t, timeseriesResponses, "200")
	timeseriesContent := requireMap(t, timeseriesOK, "content")
	timeseriesJSON := requireMap(t, timeseriesContent, "application/json")
	timeseriesSchema := requireMap(t, timeseriesJSON, "schema")
	items := requireMap(t, timeseriesSchema, "items")
	if ref, _ := items["$ref"].(string); ref != "#/components/schemas/AIFetchSeriesPoint" {
		t.Fatalf("expected AI fetch timeseries item schema ref, got %q", ref)
	}
}

func TestOpenAPISpecV1IncludesWebVitalsEndpointsAndSchemas(t *testing.T) {
	spec := openAPISpecV1("http://localhost:8080")
	paths := requireMap(t, spec, "paths")
	components := requireMap(t, spec, "components")
	schemas := requireMap(t, components, "schemas")

	for _, schemaName := range []string{"WebVitalIngestPayload", "WebVitalSummaryMetric", "WebVitalSeriesPoint", "WebVitalPageRow", "WebVitalMetricBreakdown", "WebVitalDimensionRow"} {
		if _, ok := schemas[schemaName]; !ok {
			t.Fatalf("expected %s schema to exist", schemaName)
		}
	}
	for _, path := range []string{
		"/ingest/web-vitals",
		"/api/sites/{id}/web-vitals/summary",
		"/api/sites/{id}/web-vitals/timeseries",
		"/api/sites/{id}/web-vitals/pages",
		"/api/sites/{id}/web-vitals/breakdown",
		"/api/share/{token}/sites/{id}/web-vitals/summary",
		"/api/share/{token}/sites/{id}/web-vitals/timeseries",
		"/api/share/{token}/sites/{id}/web-vitals/pages",
		"/api/share/{token}/sites/{id}/web-vitals/breakdown",
	} {
		if _, ok := paths[path]; !ok {
			t.Fatalf("expected %s path to exist", path)
		}
	}

	timeseriesPath := requireMap(t, paths, "/api/sites/{id}/web-vitals/timeseries")
	timeseriesOp := requireMap(t, timeseriesPath, "get")
	params, _ := timeseriesOp["parameters"].([]any)
	if !hasParamRef(params, "#/components/parameters/webVitalMetric") {
		t.Fatalf("expected web vital metric parameter on timeseries")
	}
}

func TestOpenAPISpecV1IncludesAIChatbotExportPath(t *testing.T) {
	spec := openAPISpecV1("http://localhost:8080")
	paths := requireMap(t, spec, "paths")

	exportPath := requireMap(t, paths, "/api/sites/{id}/ai-chatbots/export")
	getOp := requireMap(t, exportPath, "get")
	params, ok := getOp["parameters"].([]any)
	if !ok {
		t.Fatalf("expected parameters slice on AI chatbot export path")
	}
	if !hasFormatParamRef(params) {
		t.Fatalf("expected AI chatbot export path to include shared format parameter")
	}
	if !hasParamRef(params, "#/components/parameters/filter") {
		t.Fatalf("expected AI chatbot export path to document repeatable filter parameter")
	}
}

func TestOpenAPISpecV1IncludesAIFetchExportPath(t *testing.T) {
	spec := openAPISpecV1("http://localhost:8080")
	paths := requireMap(t, spec, "paths")

	exportPath := requireMap(t, paths, "/api/sites/{id}/ai-fetch/export")
	getOp := requireMap(t, exportPath, "get")
	params, ok := getOp["parameters"].([]any)
	if !ok {
		t.Fatalf("expected parameters slice on AI fetch export path")
	}
	if !hasFormatParamRef(params) {
		t.Fatalf("expected AI fetch export path to include shared format parameter")
	}
}

func TestOpenAPISpecV1IncludesEventAnalyticsPaths(t *testing.T) {
	spec := openAPISpecV1("http://localhost:8080")
	paths := requireMap(t, spec, "paths")
	components := requireMap(t, spec, "components")
	schemas := requireMap(t, components, "schemas")

	for _, schemaName := range []string{"EventSeriesPoint", "EventAudience"} {
		if _, ok := schemas[schemaName]; !ok {
			t.Fatalf("expected %s schema to exist", schemaName)
		}
	}

	for _, path := range []string{
		"/api/sites/{id}/events/names",
		"/api/sites/{id}/events/properties",
		"/api/sites/{id}/events/breakdown",
		"/api/sites/{id}/events/timeseries",
		"/api/sites/{id}/events/audience",
		"/api/share/{token}/sites/{id}/events/names",
		"/api/share/{token}/sites/{id}/events/properties",
		"/api/share/{token}/sites/{id}/events/breakdown",
		"/api/share/{token}/sites/{id}/events/timeseries",
		"/api/share/{token}/sites/{id}/events/audience",
	} {
		pathItem := requireMap(t, paths, path)
		getOp := requireMap(t, pathItem, "get")
		if _, ok := getOp["parameters"].([]any); !ok {
			t.Fatalf("expected parameters for %s to be []any", path)
		}
	}

	timeseriesPath := requireMap(t, paths, "/api/sites/{id}/events/timeseries")
	timeseriesOp := requireMap(t, timeseriesPath, "get")
	timeseriesParams, ok := timeseriesOp["parameters"].([]any)
	if !ok {
		t.Fatalf("expected parameters to be []any, got %T", timeseriesOp["parameters"])
	}
	if !hasParamRef(timeseriesParams, "#/components/parameters/filter") {
		t.Fatalf("expected event timeseries to document repeatable filter parameter")
	}
	if !hasParamRef(timeseriesParams, "#/components/parameters/eventDimensionKey") {
		t.Fatalf("expected event timeseries to document deprecated dimension_key parameter")
	}
}

func TestOpenAPISpecV1EventAudienceDocumentsGeoNetworkAggregates(t *testing.T) {
	spec := openAPISpecV1("http://localhost:8080")
	paths := requireMap(t, spec, "paths")

	for _, path := range []string{
		"/api/sites/{id}/events/audience",
		"/api/share/{token}/sites/{id}/events/audience",
	} {
		pathItem := requireMap(t, paths, path)
		getOp := requireMap(t, pathItem, "get")
		description, _ := getOp["description"].(string)
		for _, want := range []string{"cities", "providers", "ASNs"} {
			if !strings.Contains(description, want) {
				t.Fatalf("expected %s description to mention %q, got %q", path, want, description)
			}
		}
	}
}

func TestOpenAPISpecV1IncludesGoogleSearchConsoleConnectionPaths(t *testing.T) {
	spec := OpenAPISpecV1("https://hitkeep.test")
	paths := requireMap(t, spec, "paths")

	expected := map[string]string{
		"/api/user/teams/{id}/integrations/google-search-console/status":     "get",
		"/api/user/teams/{id}/integrations/google-search-console/connect":    "post",
		"/api/user/teams/{id}/integrations/google-search-console/properties": "get",
		"/api/sites/{id}/integrations/google-search-console":                 "get",
		"/api/sites/{id}/integrations/google-search-console/property":        "put",
		"/api/sites/{id}/integrations/google-search-console/sync":            "post",
		"/api/user/teams/{id}/integrations/google-search-console":            "delete",
		"/api/integrations/google-search-console/oauth/callback":             "get",
	}
	for path, method := range expected {
		pathItem := requireMap(t, paths, path)
		if _, ok := pathItem[method]; !ok {
			t.Fatalf("expected %s %s in OpenAPI paths", method, path)
		}
	}

	mappingPath := requireMap(t, paths, "/api/sites/{id}/integrations/google-search-console")
	getOp := requireMap(t, mappingPath, "get")
	responses := requireMap(t, getOp, "responses")
	okResp := requireMap(t, responses, "200")
	content := requireMap(t, okResp, "content")
	jsonContent := requireMap(t, content, "application/json")
	schema := requireMap(t, jsonContent, "schema")
	properties := requireMap(t, schema, "properties")
	if _, ok := properties["sync_status"]; !ok {
		t.Fatalf("expected site mapping schema to expose sync_status")
	}
}

func TestOpenAPISpecV1IncludesSearchConsoleReportPaths(t *testing.T) {
	spec := openAPISpecV1("http://localhost:8080")
	paths := requireMap(t, spec, "paths")
	components := requireMap(t, spec, "components")
	schemas := requireMap(t, components, "schemas")

	for _, schemaName := range []string{
		"SearchConsoleOverview",
		"SearchConsoleSeriesResponse",
		"SearchConsoleDimensionResponse",
	} {
		if _, ok := schemas[schemaName]; !ok {
			t.Fatalf("expected %s schema to exist", schemaName)
		}
	}

	overviewPath := requireMap(t, paths, "/api/sites/{id}/search-console/overview")
	overviewOp := requireMap(t, overviewPath, "get")
	overviewResponses := requireMap(t, overviewOp, "responses")
	overviewOK := requireMap(t, overviewResponses, "200")
	overviewContent := requireMap(t, overviewOK, "content")
	overviewJSON := requireMap(t, overviewContent, "application/json")
	overviewSchema := requireMap(t, overviewJSON, "schema")
	if ref, _ := overviewSchema["$ref"].(string); ref != "#/components/schemas/SearchConsoleOverview" {
		t.Fatalf("expected Search Console overview schema ref, got %q", ref)
	}

	breakdownPath := requireMap(t, paths, "/api/sites/{id}/search-console/breakdowns")
	breakdownOp := requireMap(t, breakdownPath, "get")
	breakdownParams, ok := breakdownOp["parameters"].([]any)
	if !ok {
		t.Fatalf("expected parameters to be []any, got %T", breakdownOp["parameters"])
	}
	if !hasNamedParam(breakdownParams, "dimension") {
		t.Fatalf("expected Search Console breakdowns to document dimension parameter")
	}
}

func TestOpenAPISpecV1IncludesServerSideIngestPaths(t *testing.T) {
	spec := openAPISpecV1("http://localhost:8080")
	paths := requireMap(t, spec, "paths")

	for _, path := range []string{"/api/ingest/server/pageview", "/api/ingest/server/event"} {
		t.Run(path, func(t *testing.T) {
			pathItem := requireMap(t, paths, path)
			postOp := requireMap(t, pathItem, "post")

			assertServerSideIngestDescription(t, path, postOp)
			assertAPIClientOnlyOperation(t, path, postOp)
			assertServerSideIngestSchema(t, path, requestJSONSchema(t, postOp))
		})
	}
}

func assertServerSideIngestDescription(t *testing.T, path string, postOp map[string]any) {
	t.Helper()
	description, ok := postOp["description"].(string)
	if !ok || !strings.Contains(description, "trusted server-side") || !strings.Contains(description, "API client") {
		t.Fatalf("expected trusted API client description, got %q", description)
	}
	if path == "/api/ingest/server/pageview" && (!strings.Contains(description, "UTM values are read from the query string in url") || !strings.Contains(description, "not from top-level JSON fields")) {
		t.Fatalf("expected server-side pageview description to document URL-based UTM extraction, got %q", description)
	}
	if !strings.Contains(description, "derived country, region, city, provider, and ASN") || !strings.Contains(description, "does not store the raw visitor IP") {
		t.Fatalf("expected server-side ingest description to document derived geo/network metadata and raw IP boundary, got %q", description)
	}
	if !strings.Contains(description, "exclusions, spam filtering, and country, region, city, provider, and ASN lookup") {
		t.Fatalf("expected server-side ingest description to document explicit visitor_ip metadata lookup, got %q", description)
	}
	if strings.Contains(description, "geolocation") {
		t.Fatalf("expected server-side ingest description to avoid generic geolocation wording, got %q", description)
	}
}

func assertAPIClientOnlyOperation(t *testing.T, path string, postOp map[string]any) {
	t.Helper()
	if !operationHasAPIClientOnlySecurity(postOp) {
		t.Fatalf("expected %s to use API-client-only security, got %+v", path, postOp["security"])
	}
	responses := requireMap(t, postOp, "responses")
	if _, ok := responses["429"]; !ok {
		t.Fatalf("expected %s to document 429 rate limit response", path)
	}
}

func requestJSONSchema(t *testing.T, postOp map[string]any) map[string]any {
	t.Helper()
	requestBody := requireMap(t, postOp, "requestBody")
	content := requireMap(t, requestBody, "content")
	jsonContent := requireMap(t, content, "application/json")
	return requireMap(t, jsonContent, "schema")
}

func assertServerSideIngestSchema(t *testing.T, path string, schema map[string]any) {
	t.Helper()
	required := asStringSlice(t, schema["required"])
	for _, field := range []string{"url", "timestamp", "visitor_ip", "user_agent"} {
		if !containsString(required, field) {
			t.Fatalf("expected %s request to require %q, got %v", path, field, required)
		}
	}
	properties := requireMap(t, schema, "properties")
	assertServerSideIngestProperties(t, path, properties)
}

func assertServerSideIngestProperties(t *testing.T, path string, properties map[string]any) {
	t.Helper()
	if _, ok := properties["dnt"]; !ok {
		t.Fatalf("expected %s request schema to include optional dnt", path)
	}
	visitorIP := requireMap(t, properties, "visitor_ip")
	visitorIPDescription, ok := visitorIP["description"].(string)
	if !ok || !strings.Contains(visitorIPDescription, "country, region, city, provider, and ASN") || !strings.Contains(visitorIPDescription, "does not store the raw visitor IP") {
		t.Fatalf("expected %s visitor_ip schema to document derived geo/network metadata and raw IP boundary, got %q", path, visitorIPDescription)
	}
	for _, field := range []string{"is_unique", "tracker_source", "tracker_version"} {
		if _, ok := properties[field]; ok {
			t.Fatalf("did not expect %s request schema to expose %s", path, field)
		}
	}
	if path != "/api/ingest/server/pageview" {
		return
	}
	for _, field := range []string{"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content"} {
		if _, ok := properties[field]; ok {
			t.Fatalf("did not expect %s request schema to expose %s; UTM values come from url", path, field)
		}
	}
}

func hasFormatParamRef(params []any) bool {
	return hasParamRef(params, "#/components/parameters/format")
}

func hasParamRef(params []any, want string) bool {
	for _, p := range params {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		ref, ok := pm["$ref"].(string)
		if !ok {
			continue
		}
		if ref == want {
			return true
		}
	}
	return false
}

func hasNamedParam(params []any, name string) bool {
	for _, p := range params {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if pm["name"] == name {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	return slices.Contains(values, want)
}

func operationHasAPIClientOnlySecurity(op map[string]any) bool {
	security, ok := op["security"].([]any)
	if !ok || len(security) != 2 {
		return false
	}
	var bearer, apiKey bool
	for _, item := range security {
		entry, ok := item.(map[string]any)
		if !ok || len(entry) != 1 {
			return false
		}
		if _, ok := entry["bearerAuth"]; ok {
			bearer = true
		}
		if _, ok := entry["apiKeyAuth"]; ok {
			apiKey = true
		}
		if _, ok := entry["cookieAuth"]; ok {
			return false
		}
	}
	return bearer && apiKey
}

func requireMap(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	raw, ok := m[key]
	if !ok {
		t.Fatalf("expected key %q to exist", key)
	}
	out, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected key %q to be map[string]any, got %T", key, raw)
	}
	return out
}

func asStringSlice(t *testing.T, v any) []string {
	t.Helper()
	switch values := v.(type) {
	case []string:
		out := make([]string, len(values))
		copy(out, values)
		return out
	case []any:
		out := make([]string, 0, len(values))
		for _, item := range values {
			str, ok := item.(string)
			if !ok {
				t.Fatalf("expected enum item to be string, got %T", item)
			}
			out = append(out, str)
		}
		return out
	default:
		t.Fatalf("expected []string or []any, got %T", v)
		return nil
	}
}

func hasTag(tags []map[string]string, name string) bool {
	for _, tag := range tags {
		if tag["name"] == name {
			return true
		}
	}
	return false
}

func assertCloudOperation(t *testing.T, op map[string]any) {
	t.Helper()

	gotAvailability, ok := op["x-hitkeep-availability"].(string)
	if !ok || gotAvailability != "cloud" {
		t.Fatalf("expected x-hitkeep-availability=cloud, got %#v", op["x-hitkeep-availability"])
	}

	buildTags := asStringSlice(t, op["x-hitkeep-build-tags"])
	if !reflect.DeepEqual(buildTags, []string{"billing"}) {
		t.Fatalf("unexpected cloud build tags, got %v", buildTags)
	}
}
