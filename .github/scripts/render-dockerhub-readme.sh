#!/usr/bin/env bash
set -euo pipefail

ref="${1:-${GITHUB_SHA:-main}}"
repo="pascalebeier/hitkeep"
blob_base="${HITKEEP_README_BLOB_BASE:-https://github.com/${repo}/blob/${ref}}"
raw_base="${HITKEEP_README_RAW_BASE:-https://raw.githubusercontent.com/${repo}/${ref}}"

HITKEEP_BLOB_BASE="$blob_base" HITKEEP_RAW_BASE="$raw_base" perl -0pe '
BEGIN {
  $blob = $ENV{"HITKEEP_BLOB_BASE"};
  $raw = $ENV{"HITKEEP_RAW_BASE"};
}

s{(!\[[^\]]*\]\()\./([^)]+)\)}{
  my $path = $2;
  $path =~ s#^\./##;
  $path =~ s# #\%20#g;
  "$1$raw/$path)";
}gex;

s{(?<!!)(\[[^\]]+\]\()\./([^)]+)\)}{
  my $path = $2;
  $path =~ s#^\./##;
  $path =~ s# #\%20#g;
  "$1$blob/$path)";
}gex;
' README.md
