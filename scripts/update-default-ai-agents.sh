#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"
go run -trimpath ./cmd/data-refresh ai-agents -output internal/aianalytics/default_ai_agents.json "$@"
