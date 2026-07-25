#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"
go run -tags "$("$ROOT_DIR/hk" ci go-config tags --output plain)" ./cmd/hitkeep/main.go update-ai-agent-lists -output internal/aianalytics/default_ai_agents.json
