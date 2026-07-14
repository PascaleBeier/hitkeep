#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

mode="${1:-default}"
case "$mode" in
  default|common)
    variant=self-hosted
    field=tags
    ;;
  cloud)
    variant=cloud
    field=tags
    ;;
  csv)
    variant=self-hosted
    field=csv
    ;;
  cloud-csv)
    variant=cloud
    field=csv
    ;;
  goflags)
    variant=self-hosted
    field=goflags
    ;;
  golangci)
    variant=self-hosted
    field=golangci
    ;;
  *)
    cat >&2 <<'EOF'
Usage: scripts/go-build-tags.sh [default|common|cloud|csv|cloud-csv|goflags|golangci] [extra tags...]
EOF
    exit 2
    ;;
esac

shift || true
args=("$ROOT_DIR/hk" ci go-config "$field" --variant "$variant" --output plain)
for tag in "$@"; do
  args+=(--extra-tag "$tag")
done
exec "${args[@]}"
