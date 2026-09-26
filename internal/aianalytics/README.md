# AI Agent Master List

HitKeep classifies AI agent traffic (crawlers, assistants, autonomous agents)
out of the box using an embedded master list of user-agent tokens assembled
from multiple open-source upstreams:

- [ai.robots.txt](https://github.com/ai-robots-txt/ai.robots.txt) (MIT) — the
  primary source; AI-focused with purpose categories.
- [Device Detector bots.yml](https://github.com/matomo-org/device-detector)
  (LGPL-3.0-or-later, attributed in the repository `NOTICE`) — entries in the
  `AI *` categories with literal regexes.
- [crawler-user-agents](https://github.com/monperrus/crawler-user-agents)
  (MIT) — entries tagged `ai-crawler` with literal patterns.
- A curated overlay (`curated.go`) preserving HitKeep's original
  classifications, plus the curated AI referrer surfaces (ChatGPT, Perplexity,
  ...) used for the AI referral dimension.

The embedded bundle (`default_ai_agents.json`) is the single source of truth
for both the Go classifier (`ClassifyBot`) and the generated DuckDB macros
(`hk_ai_bot`, `hk_ai_bot_category`, `hk_ai_bot_category_from_name`,
`hk_ai_source`), which the database layer
recreates at every store open. Historical hits are therefore reclassified
automatically when a release ships a fresher list.

## Refresh

The list is refresh-on-release (like `internal/ipmeta`), not runtime-updated.
The scheduled `.github/workflows/data-refresh.yml` workflow runs the `ai-agents`
target twice a month and opens a pull request when the data changes.

Manual refresh from the repository root:

```sh
go run ./cmd/data-refresh ai-agents
```

The existing `./scripts/update-default-ai-agents.sh` and `hitkeep update-ai-agent-lists` commands remain available. A valid fetch with unchanged list content keeps the file's existing generation timestamp and does not rewrite it; source metadata changes still produce an update.

`ValidateEmbeddedAIAgentData` gates the bundle: minimum totals per source,
token hygiene (length, generic-term and dual-use denylists, LIKE-safety), the
curated tokens, and the required referrer surfaces must all hold, otherwise
the CLI refuses to write the file.

## Matching semantics

Tokens are lowercase substrings matched longest-first — the only semantic
implementable identically in Go (`strings.Contains`) and SQL (`LIKE`). Upstream
regex patterns are admitted only when they are literal strings after
unescaping. Conflicts between sources resolve by priority: curated >
ai.robots.txt > device-detector > crawler-user-agents.
