#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"
go run -trimpath -tags "$("$ROOT_DIR/hk" ci go-config tags --output plain)" ./cmd/hitkeep/main.go update-spam-lists -output internal/blocking/default_spam_filter.json
