---
name: hitkeep-analytics
description: 'Official parent skill for analyzing HitKeep data through HitKeep MCP and official docs. Use for broad traffic, event, ecommerce, Web Vitals, AI visibility, Search Console, opportunity, tracking-health, MCP setup, or analytics-surface questions; use a narrower HitKeep analytics skill when the task clearly matches one.'
---

# HitKeep Analytics

Read [procedure.md](references/procedure.md) before analyzing data. It is the canonical, transport-neutral reasoning procedure also used by HitKeep Ask AI.

## External-agent adapter

- Use the production HitKeep MCP for scoped, read-only aggregate analytics and official docs lookup.
- Inspect live MCP tool schemas and help resources instead of copying the tool catalog, filters, limits, or defaults into this skill.
- If MCP is unavailable, state what cannot be verified and use official docs or user-provided data.
- Use the dashboard for human setup, administration, and visual review; REST APIs for supported application automation; and exports or takeout for portable owned data.
- Never use the local `hk` developer MCP for customer analytics.

Use `$hitkeep-traffic-diagnosis`, `$hitkeep-ai-visibility-analyst`, `$hitkeep-ecommerce-analyst`, or `$hitkeep-tracking-verifier` when installed and the task clearly matches one specialist. The parent remains sufficient when installed alone.

Do not request broader credentials, dashboard cookies, raw visitor rows, raw hit exports, billing changes, administration, token management, or mutations through MCP.
