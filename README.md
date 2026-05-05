# HitKeep

> Privacy-first analytics for humans and AI agents, self-hosted or in EU/US cloud.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.26.2-00ADD8?logo=go)](https://go.dev/)
[![Docker Image (GHCR)](https://img.shields.io/badge/Docker-ghcr.io-blue?logo=docker)](https://github.com/pascalebeier/hitkeep/pkgs/container/hitkeep)
[![Docker Image (Hub)](https://img.shields.io/badge/Docker-Docker_Hub-2496ED?logo=docker)](https://hub.docker.com/r/pascalebeier/hitkeep)
[![Documentation](https://img.shields.io/badge/Docs-hitkeep.com-0ea5e9)](https://hitkeep.com)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/11990/badge)](https://www.bestpractices.dev/projects/11990)

HitKeep is an open source analytics platform for teams that need clear product reporting for humans and governed analytics access for AI-assisted workflows, without adopting the usual PostgreSQL, Redis, ClickHouse, and reverse-proxy pileup.

- Single binary runtime
- Embedded DuckDB and NSQ with batched ingest writes
- Privacy-first tracking
- AI visibility analytics for server-side crawler fetches and AI-referred visits
- Traffic, goals, funnels, ecommerce, AI chatbot analytics, and email reports
- Scoped API clients and read-only MCP access for agents, assistants, and internal reporting tools
- Self-hosted or managed cloud with EU/US region choice

[AI Performance](https://hitkeep.com/ai-performance/) · [Website](https://hitkeep.com) · [Cloud](https://hitkeep.com/cloud) · [Live Demo](https://demo.hitkeep.com/share/7a55968bb42df256512fbe7ff73ab88f29dd45c236eddc818bd66420b4ffbaad) · [Docs](https://hitkeep.com/guides/introduction/) · [API](https://hitkeep.com/api/) · [Releases](https://github.com/PascaleBeier/hitkeep/releases)

![HitKeep analytics dashboard — traffic overview, geographic breakdown, goals, funnels, and UTM attribution](./.github/assets/dashboard-overview.png)

## Why HitKeep

HitKeep is for teams that want product analytics without adopting a full analytics platform stack.

- **Simple to run:** one binary, one data directory, no external database required
- **Efficient write path:** NSQ buffers ingest bursts and DuckDB appender batches smooth out disk-heavy per-row inserts
- **Privacy-first by default:** cookie-less tracking, Do Not Track support, focused data collection
- **Ready for AI visibility work:** server-side AI crawler fetch analytics, AI-referred visits from the browser tracker, and correlation reports for pages that get crawler interest but weak downstream traffic
- **Useful out of the box:** traffic analytics with countries/languages audience toggles, top/landing/exit page views, custom events, goals, funnels, ecommerce, UTM attribution, and scheduled email reports
- **Built for teams:** passkeys, TOTP, site and team permissions, API clients, share links, and audit visibility
- **Flexible deployment:** self-host it yourself or use HitKeep Cloud and still keep the migration path open

## For SEO Agencies

HitKeep can run a 14-day AI visibility pilot for one client site. Install `hk.js` for normal pageviews and AI-referred human visits, then forward server-side crawler fetches from CloudFront, nginx, Caddy, or an app server into HitKeep. The AI Visibility dashboard shows which pages GPTBot, ClaudeBot, PerplexityBot, and other crawlers request, which fetched pages later receive AI-referred visits, and where crawler errors create SEO work.

- [AI performance landing page](https://hitkeep.com/ai-performance/)
- [AI visibility analytics for SEO agencies](https://hitkeep.com/use-cases/ai-visibility-seo-agencies/)
- [CloudFront AI crawler tracking](https://hitkeep.com/guides/tracking/cloudfront-ai-crawler-tracking/)
- [AI Fetch on AWS setup guide](https://hitkeep.com/guides/tracking/ai-fetch-aws/)

## Quick Start

### Binary

Download the latest release for your system:

```bash
wget https://github.com/PascaleBeier/hitkeep/releases/latest/download/hitkeep-linux-arm64
chmod +x hitkeep-linux-arm64
export HITKEEP_JWT_SECRET="replace-this-with-a-long-random-string"
./hitkeep-linux-arm64 -public-url="http://localhost:8080"
```

Open `http://localhost:8080` and create your first account.

### Docker

```yaml
services:
  hitkeep:
    image: pascalebeier/hitkeep:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - hitkeep_data:/var/lib/hitkeep/data
    environment:
      HITKEEP_JWT_SECRET: replace-this-with-a-long-random-string
    command:
      - "-public-url=http://localhost:8080"

volumes:
  hitkeep_data: {}
```

```bash
docker compose up -d
```

For production setup, reverse proxies, SMTP, systemd, Kubernetes, S3 archiving, and every configuration flag, use the docs instead of this README:

- [Installation guides](https://hitkeep.com/guides/installation/)
- [Configuration reference](https://hitkeep.com/reference/configuration/)
- [Cloud documentation](https://hitkeep.com/cloud)

## Tracking Snippet

Once your instance is running and a site is created, add:

```html
<script async src="https://your-hitkeep-instance.com/hk.js"></script>
```

Custom event example:

```html
<script>
  window.hk = window.hk || {};
  window.hk.event?.("signup", { plan: "pro", source: "landing-page" });
</script>
```

Tracker options, ecommerce events, custom events, and advanced tracking examples live here:

- [Tracking docs](https://hitkeep.com/guides/tracking/)
- [Custom events](https://hitkeep.com/guides/tracking/custom-events/)
- [Ecommerce analytics](https://hitkeep.com/guides/analytics/ecommerce/)
- [AI visibility analytics](https://hitkeep.com/guides/analytics/ai-visibility/)
- [CloudFront AI crawler tracking](https://hitkeep.com/guides/tracking/cloudfront-ai-crawler-tracking/)
- [AI chatbot analytics](https://hitkeep.com/guides/analytics/ai-chatbot-analytics/)

## Product Tour

<details>
<summary>See more screenshots</summary>

### Comparison
![HitKeep period-over-period comparison with delta badges and overlay charts](./.github/assets/dashboard-comparison.png)

### Events
![HitKeep custom event analytics with timeseries chart and property breakdown](./.github/assets/analytics-events.png)

### Goals
![HitKeep goals and conversion tracking](./.github/assets/analytics-goals.png)

### Funnels
![HitKeep multi-step funnel analytics](./.github/assets/analytics-funnels.png)

### Ecommerce
![HitKeep ecommerce analytics with revenue KPIs, chart, top products, and revenue sources](./.github/assets/analytics-ecommerce.png)

### AI Chatbots
![HitKeep AI chatbot analytics with KPI cards, conversation activity chart, and chatbot breakdowns](./.github/assets/analytics-ai-chatbots.png)

### AI Visibility
![HitKeep AI visibility analytics with fetch KPIs, assistant filters, and fetch volume chart](./.github/assets/analytics-ai-visibility.png)

### AI Visibility Correlation
![HitKeep AI visibility correlation with summary KPIs and tabbed citation yield, opportunity pages, and failure hotspots tables](./.github/assets/analytics-ai-visibility-correlation.png)

### Email Reports
![HitKeep scheduled email reports](./.github/assets/feature-email-reports.png)

### API Clients
![HitKeep API client management](./.github/assets/security-api-clients.png)

### Create Team
![HitKeep create team dialog with name and logo setup](./.github/assets/feature-create-team.png)

### Admin Users
![HitKeep administration users view with roles and security controls](./.github/assets/admin-users.png)

### Team Overview
![HitKeep team administration overview](./.github/assets/admin-team-overview.png)

</details>

## Documentation

The maintained reference lives on `hitkeep.com`.

- [Getting started](https://hitkeep.com/guides/introduction/)
- [Installation](https://hitkeep.com/guides/installation/)
- [Configuration](https://hitkeep.com/reference/configuration/)
- [REST API reference](https://hitkeep.com/api/)
- [Compliance](https://hitkeep.com/compliance/overview/)
- [Privacy policy for cloud](https://hitkeep.com/legal/privacy-policy/)
- [Terms of service](https://hitkeep.com/legal/terms-of-service/)
- [Comparison pages](https://hitkeep.com/vs/)

## Cloud

HitKeep Cloud is live.

If you want the same product without running it yourself, start here:

- [Start in the EU](https://cloud.hitkeep.eu/signup)
- [Start in the US](https://cloud.hitkeep.com/signup)
- [Cloud overview](https://hitkeep.com/cloud)

## Development

Prerequisites:

- Go 1.26+
- Node.js 24+
- Make
- A working C toolchain for DuckDB builds

Build from source:

```bash
git clone https://github.com/pascalebeier/hitkeep.git
cd hitkeep
make build
./hitkeep
```

For day-to-day development:

```bash
make dev
```

This starts the Go backend with live reload and the Angular dashboard on `http://localhost:4200`.

For a seeded local workspace with demo data:

```bash
make dev-seed
```

Contributor docs and local development guides:

- [Contributing guide](./CONTRIBUTING.md)
- [Dashboard development notes](./frontend/dashboard/README.md)
- [Changelog](./CHANGELOG.md)

## License

Distributed under the MIT License. See [LICENSE](./LICENSE).
