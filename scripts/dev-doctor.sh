#!/usr/bin/env bash

set -u

docker_ready=1
native_ready=1

available() {
  command -v "$1" >/dev/null 2>&1
}

ok() {
  printf '  ok  %s\n' "$1"
}

missing() {
  printf '  --  %s\n' "$1"
}

printf 'Docker development\n'
if available docker; then
  ok "$(docker --version)"
else
  missing 'Docker CLI is not installed'
  docker_ready=0
fi

if (( docker_ready )) && docker info >/dev/null 2>&1; then
  ok 'Docker daemon is reachable'
else
  missing 'Docker daemon is not reachable'
  docker_ready=0
fi

if available docker && docker compose version >/dev/null 2>&1; then
  ok "$(docker compose version)"
else
  missing 'Docker Compose is not available'
  docker_ready=0
fi

printf '\nNative development\n'
if available go; then
  go_version="$(go env GOVERSION)"
  go_minor="${go_version#go1.}"
  go_minor="${go_minor%%.*}"
  if [[ "$go_version" =~ ^go1\.[0-9]+([.].*)?$ ]] && (( go_minor >= 26 )); then
    ok "$(go version)"
  else
    missing "$(go version) (Go 1.26+ is required)"
    native_ready=0
  fi
else
  missing 'go is not installed'
  native_ready=0
fi

if available node; then
  node_major="$(node -p 'process.versions.node.split(".")[0]')"
  if (( node_major >= 24 )); then
    ok "Node $(node --version)"
  else
    missing "Node $(node --version) is installed (Node 24+ is required)"
    native_ready=0
  fi
else
  missing 'node is not installed'
  native_ready=0
fi

if available npm; then
  ok "npm $(npm --version)"
else
  missing 'npm is not installed'
  native_ready=0
fi

if available cc; then
  ok "CGo compiler: $(cc --version 2>/dev/null | sed -n '1p')"
else
  missing 'CGo compiler (cc) is not installed'
  native_ready=0
fi

if available mailpit; then
  ok 'Mailpit is installed'
else
  missing 'Mailpit is not installed'
  native_ready=0
fi

printf '\nReady workflows\n'
if (( docker_ready )); then
  ok 'Docker: make dev-docker-seed or make dev-docker-cloud-seed'
else
  missing 'Docker workflow is unavailable'
fi
if (( native_ready )); then
  ok 'Native: make dev-seed or make dev-cloud-seed'
else
  missing 'Native workflow is unavailable'
fi

if (( ! docker_ready && ! native_ready )); then
  exit 1
fi
