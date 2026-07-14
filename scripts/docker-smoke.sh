#!/usr/bin/env bash

set -euo pipefail

image="${1:?usage: docker-smoke.sh IMAGE EXPECTED_VARIANT [--cloud]}"
expected_variant="${2:?usage: docker-smoke.sh IMAGE EXPECTED_VARIANT [--cloud]}"
mode="${3:-}"
container="hitkeep-smoke-$$"

cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
}
trap cleanup EXIT

variant="$(docker image inspect "$image" --format '{{ index .Config.Labels "io.hitkeep.variant" }}')"
if [[ "$variant" != "$expected_variant" ]]; then
  printf 'Expected image %s to have variant %s, got %s\n' "$image" "$expected_variant" "$variant" >&2
  exit 1
fi

docker_args=(
  -e HITKEEP_JWT_SECRET=hitkeep-local-container-smoke-secret
  -e HITKEEP_PUBLIC_URL=http://localhost:8080
  -e HITKEEP_MAIL_DRIVER=log
  -e HITKEEP_SPAM_FILTER_AUTO_UPDATE=false
)

if [[ "$mode" == "--cloud" ]]; then
  docker_args+=(
    -e HITKEEP_CLOUD_HOSTED=true
    -e HITKEEP_CLOUD_SIGNUP_ENABLED=true
    -e HITKEEP_CLOUD_JURISDICTION=EU
    -e HITKEEP_CLOUD_REGION=eu-central-1
  )
elif [[ -n "$mode" ]]; then
  printf 'Unknown option: %s\n' "$mode" >&2
  exit 2
fi

docker run -d --name "$container" "${docker_args[@]}" "$image" >/dev/null

for _ in $(seq 1 45); do
  if docker exec "$container" hitkeep -healthcheck >/dev/null 2>&1; then
    printf 'Container smoke check passed: %s (%s)\n' "$image" "$expected_variant"
    exit 0
  fi
  if [[ "$(docker inspect "$container" --format '{{.State.Running}}')" != "true" ]]; then
    docker logs "$container" >&2
    exit 1
  fi
  sleep 1
done

docker logs "$container" >&2
printf 'Container did not become healthy within 45 seconds: %s\n' "$image" >&2
exit 1
