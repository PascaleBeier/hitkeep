# Official HitKeep Analytics Skills

This directory is the canonical product analytics skill pack. Its five skills are published for external assistants, and their transport-neutral procedure references are compiled into HitKeep Ask AI.

The pack contains:

- `hitkeep-analytics`: broad analytics routing, scope, evidence, and privacy judgment.
- `hitkeep-traffic-diagnosis`: traffic drops, spikes, source shifts, and measurement checks.
- `hitkeep-ai-visibility-analyst`: crawler fetches, AI referrals, correlation, and failure analysis.
- `hitkeep-ecommerce-analyst`: revenue, products, acquisition quality, and conversion movement.
- `hitkeep-tracking-verifier`: installation and automatic-event verification.

Each `SKILL.md` is a thin external-agent adapter for production HitKeep MCP. Each `references/procedure.md` contains the transport-neutral reasoning shared with Ask AI. Ask AI uses its internal analytics tool bridge, not MCP, and never embeds contributor skills.

## Install

List only the analytics pack:

```bash
npx skills add https://github.com/PascaleBeier/hitkeep/tree/main/skills --list
```

Install the complete analytics pack for Codex:

```bash
npx skills add https://github.com/PascaleBeier/hitkeep/tree/main/skills --skill '*' --agent codex --copy -y
```

Install one specialist by replacing the skill name:

```bash
npx skills add https://github.com/PascaleBeier/hitkeep/tree/main/skills --skill hitkeep-traffic-diagnosis --agent codex --copy -y
```

For live analytics, pair the installed skill with HitKeep's scoped, read-only production MCP endpoint. Skills contain no credentials or customer data.

Contributor skills for changing HitKeep itself are canonical under `.agents/skills` and documented in `CONTRIBUTING.md`. Do not install the repository root with `--skill '*'` when only one audience is intended.
