#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCHEMA_URL="${MCP_REGISTRY_SCHEMA_URL:-https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

curl -fsSL "$SCHEMA_URL" -o "$TMP_DIR/server.schema.json"
npx -y -p ajv-cli@5 -p ajv-formats ajv validate \
  --strict=false \
  -c ajv-formats \
  -s "$TMP_DIR/server.schema.json" \
  -d "$ROOT_DIR/server.json"

if [[ "${1:-}" == "--schema-only" ]]; then
  exit 0
fi

if [[ -z "${HITKEEP_MCP_URL:-}" || -z "${HITKEEP_MCP_TOKEN:-}" ]]; then
  echo "Skipping live MCP audit: set HITKEEP_MCP_URL and HITKEEP_MCP_TOKEN to audit a running server."
  exit 0
fi

rpc() {
  local method="$1"
  curl -fsSL "$HITKEEP_MCP_URL" \
    -H "Accept: application/json, text/event-stream" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $HITKEEP_MCP_TOKEN" \
    --data "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"$method\"}"
}

TOOLS_JSON="$(rpc "tools/list")"
RESOURCES_JSON="$(rpc "resources/list")"
TEMPLATES_JSON="$(rpc "resources/templates/list")"

TOOLS_JSON="$TOOLS_JSON" RESOURCES_JSON="$RESOURCES_JSON" TEMPLATES_JSON="$TEMPLATES_JSON" node <<'NODE'
const tools = JSON.parse(process.env.TOOLS_JSON);
const resources = JSON.parse(process.env.RESOURCES_JSON);
const templates = JSON.parse(process.env.TEMPLATES_JSON);

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const expectedTools = new Set([
  "hitkeep_list_sites",
  "hitkeep_get_site_overview",
  "hitkeep_get_event_names",
  "hitkeep_get_event_breakdown",
  "hitkeep_get_ecommerce",
  "hitkeep_get_ai_visibility",
  "hitkeep_get_search_console_status",
  "hitkeep_get_search_console",
  "hitkeep_search_docs",
  "hitkeep_get_doc",
  "hitkeep_get_api_reference",
  "hitkeep_get_mcp_help",
]);
const openWorldTools = new Set([
  "hitkeep_search_docs",
  "hitkeep_get_doc",
  "hitkeep_get_api_reference",
]);
const forbidden = [
  "create",
  "delete",
  "update",
  "mutate",
  "write",
  "export_hits",
  "raw_hits",
  "billing",
  "takeout",
  "exclusion",
];

assert(!tools.error, `tools/list failed: ${JSON.stringify(tools.error)}`);
assert(Array.isArray(tools.result?.tools), "tools/list did not return result.tools");
for (const name of expectedTools) {
  assert(tools.result.tools.some((tool) => tool.name === name), `missing tool ${name}`);
}
for (const tool of tools.result.tools) {
  assert(tool.name?.startsWith("hitkeep_"), `unexpected tool namespace ${tool.name}`);
  assert(tool.title && tool.description, `tool ${tool.name} needs title and description`);
  assert(tool.annotations?.readOnlyHint === true, `tool ${tool.name} must be read-only`);
  if (!openWorldTools.has(tool.name)) {
    assert(tool.annotations?.openWorldHint === false, `tool ${tool.name} must be closed-world`);
  }
  assert(!forbidden.some((part) => tool.name.includes(part)), `forbidden tool surface ${tool.name}`);
}

assert(!resources.error, `resources/list failed: ${JSON.stringify(resources.error)}`);
const resourceUris = new Set((resources.result?.resources ?? []).map((resource) => resource.uri));
for (const uri of ["hitkeep://help/mcp", "hitkeep://help/metrics", "hitkeep://docs/llms"]) {
  assert(resourceUris.has(uri), `missing resource ${uri}`);
}

assert(!templates.error, `resources/templates/list failed: ${JSON.stringify(templates.error)}`);
assert(
  (templates.result?.resourceTemplates ?? []).some((template) => template.uriTemplate === "hitkeep://docs/{+path}"),
  "missing docs resource template",
);

console.log("Live MCP audit passed");
NODE
