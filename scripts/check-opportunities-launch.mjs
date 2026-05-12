#!/usr/bin/env node

import { readFileSync } from "node:fs";
import { join } from "node:path";

const repoRoot = new URL("..", import.meta.url).pathname;
const docsRoot = join(repoRoot, "..", "hitkeep-docs");

const files = {
  opportunityGuide: join(docsRoot, "src/content/docs/guides/analytics/opportunities.mdx"),
  mcpGuide: join(docsRoot, "src/content/docs/guides/integrations/mcp.mdx"),
  takeoutGuide: join(docsRoot, "src/content/docs/guides/data/takeout.mdx"),
  publicOpenAPI: join(docsRoot, "public/openapi.yml"),
  runtimeOpenAPIPaths: join(repoRoot, "internal/server/system/api_docs_paths_site.go"),
  screenshotScript: join(repoRoot, "scripts/screenshot.mjs"),
  opportunitiesE2E: join(repoRoot, "frontend/dashboard/e2e/opportunities.seeded.spec.js"),
};

const text = Object.fromEntries(Object.entries(files).map(([key, file]) => [key, readFileSync(file, "utf8")]));

const requiredDetectorKeys = [
  "checkout_conversion",
  "ai_visibility",
  "traffic_quality",
  "search_visibility",
  "conversion_signal",
  "setup_goal_suggestion",
  "setup_funnel_suggestion",
  "tracking_setup",
];

const checks = [
  ["opportunity guide explains the launch promise", text.opportunityGuide.includes("Know what to fix next")],
  ["opportunity guide documents localization-safe API", text.opportunityGuide.includes("Localization-safe API contract")],
  ["opportunity guide uses provider-neutral model placeholder", text.opportunityGuide.includes("HITKEEP_AI_MODEL=your-model-id")],
  ["opportunity guide uses gateway model placeholder", text.opportunityGuide.includes("HITKEEP_AI_MODEL=your-routing-model")],
  ["opportunity guide uses Bedrock model placeholder", text.opportunityGuide.includes("HITKEEP_AI_MODEL=your-bedrock-model-id")],
  ["opportunity guide documents MCP surface", text.opportunityGuide.includes("[MCP](/guides/integrations/mcp/)")],
  ["opportunity guide documents takeout boundary", text.opportunityGuide.includes("[Site takeout](/guides/data/takeout/)")],
  ["MCP guide documents opportunities tool", text.mcpGuide.includes("hitkeep_get_opportunities")],
  ["MCP guide excludes raw prompts", text.mcpGuide.includes("raw prompts")],
  ["takeout guide includes opportunities", text.takeoutGuide.includes("Site takeout includes saved Opportunities")],
  ["takeout guide excludes provider secrets", text.takeoutGuide.includes("provider secrets")],
  ["public OpenAPI lists opportunities", text.publicOpenAPI.includes("/api/sites/{id}/opportunities:")],
  ["public OpenAPI lists digest preview endpoint", text.publicOpenAPI.includes("/api/sites/{id}/opportunities/digest-preview:")],
  ["public OpenAPI lists generate endpoint", text.publicOpenAPI.includes("/api/sites/{id}/opportunities/generate:")],
  ["public OpenAPI lists digest preview schema", text.publicOpenAPI.includes("OpportunityDigestPreviewResponse:")],
  ["runtime OpenAPI lists opportunities", text.runtimeOpenAPIPaths.includes('"/api/sites/{id}/opportunities"')],
  ["runtime OpenAPI lists digest preview", text.runtimeOpenAPIPaths.includes('"/api/sites/{id}/opportunities/digest-preview"')],
  ["seeded Playwright covers opportunities", text.opportunitiesE2E.includes("opportunities inbox supports localized read and manage workflow")],
  ["screenshot pipeline captures opportunities", text.screenshotScript.includes('"analytics-opportunities"')],
  ["screenshot pipeline waits for rendered opportunities", text.screenshotScript.includes("app-opportunity-card")],
];

for (const key of requiredDetectorKeys) {
  checks.push([`opportunity guide documents ${key}`, text.opportunityGuide.includes(key)]);
}

const failures = checks.filter(([, pass]) => !pass).map(([name]) => name);
if (failures.length > 0) {
  console.error("Opportunities launch contract failed:");
  for (const failure of failures) {
    console.error(`- ${failure}`);
  }
  process.exit(1);
}

console.log("Opportunities launch contract passed.");
