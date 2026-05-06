#!/usr/bin/env bash

set -euo pipefail

COMMON_TAGS="${HITKEEP_GO_BUILD_TAGS:-hashicorpmetrics timetzdata}"
CLOUD_TAGS="${HITKEEP_CLOUD_GO_BUILD_TAGS:-s3 billing tenancy}"

join_tags() {
  printf '%s\n' "$@" | awk '
    {
      for (i = 1; i <= NF; i++) {
        if (!seen[$i]++) {
          tags[++count] = $i
        }
      }
    }
    END {
      for (i = 1; i <= count; i++) {
        printf "%s%s", sep, tags[i]
        sep = " "
      }
      printf "\n"
    }
  '
}

comma_tags() {
  join_tags "$@" | tr ' ' ','
}

mode="${1:-default}"
case "$mode" in
  default|common)
    shift || true
    join_tags "$COMMON_TAGS" "$@"
    ;;
  cloud)
    shift || true
    if [[ "$#" -gt 0 ]]; then
      join_tags "$COMMON_TAGS" "$@"
    else
      join_tags "$COMMON_TAGS" "$CLOUD_TAGS"
    fi
    ;;
  csv)
    shift || true
    comma_tags "$COMMON_TAGS" "$@"
    ;;
  cloud-csv)
    shift || true
    if [[ "$#" -gt 0 ]]; then
      comma_tags "$COMMON_TAGS" "$@"
    else
      comma_tags "$COMMON_TAGS" "$CLOUD_TAGS"
    fi
    ;;
  goflags)
    shift || true
    printf -- '-tags=%s\n' "$(comma_tags "$COMMON_TAGS" "$@")"
    ;;
  golangci)
    shift || true
    printf -- '--build-tags=%s\n' "$(comma_tags "$COMMON_TAGS" "$@")"
    ;;
  *)
    cat >&2 <<'EOF'
Usage: scripts/go-build-tags.sh [default|common|cloud|csv|cloud-csv|goflags|golangci] [extra tags...]
EOF
    exit 2
    ;;
esac
