#!/usr/bin/env bash

set -euo pipefail

image="${1:?usage: docker-smoke.sh IMAGE EXPECTED_VARIANT [--cloud|--recreate]}"
previous_image="${HITKEEP_PREVIOUS_IMAGE:-}"
if [[ "${3:-}" == "--recreate" && -z "$previous_image" ]]; then
  printf 'HITKEEP_PREVIOUS_IMAGE must name an immutable supported 2.x image digest for --recreate\n' >&2
  exit 2
fi
expected_variant="${2:?usage: docker-smoke.sh IMAGE EXPECTED_VARIANT [--cloud|--recreate]}"
mode="${3:-}"
container="hitkeep-smoke-$$"
volume=""
platform=""
temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/hitkeep-smoke.XXXXXX")"
fixture_manifest="tests/fixtures/release-fixtures.json"

fixture() {
  env -u GOROOT go run ./tests/fixtures/upgrade-fixture "$@"
}

cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
  if [[ -n "$volume" ]]; then
    docker volume rm -f "$volume" >/dev/null 2>&1 || true
  fi
  rm -rf "$temp_dir"
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
elif [[ "$mode" == "--recreate" ]]; then
  volume="hitkeep-smoke-data-$$"
  docker volume create "$volume" >/dev/null
  case "$(docker image inspect "$image" --format '{{.Architecture}}')" in
    amd64|arm64) platform="linux/$(docker image inspect "$image" --format '{{.Architecture}}')" ;;
    *) printf 'Unsupported candidate image architecture\n' >&2; exit 2 ;;
  esac
  docker_args+=(
    --platform "$platform"
    --mount "type=volume,src=$volume,dst=/var/lib/hitkeep/data"
    -p 127.0.0.1::8080
  )
elif [[ -n "$mode" ]]; then
  printf 'Unknown option: %s\n' "$mode" >&2
  exit 2
fi

start_container() {
  local selected_image="${1:-$image}"
  docker run -d --name "$container" "${docker_args[@]}" "$selected_image" >/dev/null
}

remove_container() {
  docker stop -t 15 "$container" >/dev/null
  docker rm "$container" >/dev/null
}

container_url() {
  local published
  published="$(docker port "$container" 8080/tcp | head -n 1)"
  printf 'http://%s' "$published"
}

if [[ "$mode" == "--recreate" ]]; then
  if [[ "$previous_image" != *@sha256:* ]]; then
    printf 'HITKEEP_PREVIOUS_IMAGE must be an immutable @sha256 digest: %s\n' "$previous_image" >&2
    exit 2
  fi
  previous_variant="$(docker image inspect "$previous_image" --format '{{ index .Config.Labels "io.hitkeep.variant" }}')"
  if [[ "$previous_variant" != "$expected_variant" ]]; then
    printf 'Expected previous image %s to have variant %s, got %s\n' "$previous_image" "$expected_variant" "$previous_variant" >&2
    exit 1
  fi
fi

await_healthy() {
  for _ in $(seq 1 45); do
    if docker exec "$container" hitkeep -healthcheck >/dev/null 2>&1; then
      return 0
    fi
    if [[ "$(docker inspect "$container" --format '{{.State.Running}}')" != "true" ]]; then
      docker logs "$container" >&2
      return 1
    fi
    sleep 1
  done
  docker logs "$container" >&2
  printf 'Container did not become healthy within 45 seconds: %s\n' "$image" >&2
  return 1
}

if [[ "$mode" == "--recreate" ]]; then
  start_container "$previous_image"
  await_healthy
  fixture --seed --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$(container_url)"
  remove_container
fi
start_container "$image"
await_healthy

if [[ "$mode" == "--recreate" ]]; then
  fixture --verify --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$(container_url)"
  remove_container
  start_container "$image"
  await_healthy
  remove_container
  start_container "$image"
  await_healthy
  fixture --verify --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$(container_url)"
  printf 'Container recreation preserved release fixture data: %s (%s)\n' "$image" "$expected_variant"
else
  printf 'Container smoke check passed: %s (%s)\n' "$image" "$expected_variant"
fi
