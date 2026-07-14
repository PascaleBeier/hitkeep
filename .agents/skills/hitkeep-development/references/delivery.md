# Documentation, integrations, and delivery

Read this reference only when the contribution changes public docs, OpenAPI, distributable integrations, release communication, or infrastructure-adjacent behavior.

## Documentation

- Treat the adjacent `hitkeep-docs` repository as the private source for the public documentation website and follow its local instructions. It is intentionally separate from HitKeep's public MIT-licensed source.
- When that private repository is unavailable, record the exact documentation impact for a maintainer instead of inventing or silently skipping a website change.
- Prefer updating the nearest page. Use tutorials for learning paths, how-to guides for tasks, reference for exact contracts, and explanation for tradeoffs.
- Lead with the reader's job and concrete product behavior. Remove padded introductions, generic claims, unsupported promises, internal task notes, and private operating details.
- Keep runtime OpenAPI and docs OpenAPI synchronized.
- Use current screenshots only when they explain a workflow or state.
- Run the docs repository's own validation and report it separately from app-repository QA.

## Integrations

- Keep distributable CMS plugins and SDKs in their dedicated repositories, not under the main application tree.
- Align integrations with the current tracker, ingest contract, privacy defaults, and public docs.
- Keep adapters thin and platform-native; do not copy site-specific settings, credentials, uploads, or generated files from prototypes.
- Validate through the target repository's own tools and release workflow.

## Release preparation

- Derive commit, changelog, and artifact behavior from current repository configuration rather than a skill-maintained type/scope list.
- Keep unrelated worktree changes out of the release diff.
- Mention docs, OpenAPI, seed, screenshot, migration, and compatibility impact where relevant.
- Do not claim a release is stable until the changelog, manifest, and published release agree.
- Change MCP registry metadata only when the published version or protocol surface changes.

## Infrastructure-adjacent work

`hk` may build and verify deterministic artifacts but does not manage credentials, publish, deploy, or mutate infrastructure. Work in the infrastructure repository only with explicit user authority, inspect its current policy, discover live identifiers instead of hardcoding them, and keep accounts, buckets, instance IDs, profiles, secrets, and deployment evidence out of public skills and reports.
