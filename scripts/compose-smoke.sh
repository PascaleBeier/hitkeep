#!/usr/bin/env bash

set -euo pipefail

image="${1:?usage: compose-smoke.sh IMAGE EXPECTED_VARIANT}"
expected_variant="${2:?usage: compose-smoke.sh IMAGE EXPECTED_VARIANT}"
previous_image="${HITKEEP_PREVIOUS_IMAGE:?HITKEEP_PREVIOUS_IMAGE must name an immutable supported 2.x image digest}"
fixture_manifest="tests/fixtures/release-fixtures.json"
rollback_helper_image="busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662"
compose_project="hitkeep-compose-smoke-$$-${RANDOM}"
container="${compose_project}-app"
temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/hitkeep-compose-smoke.XXXXXX")"
override="$temp_dir/compose.smoke.yaml"
platform=""
data_volume=""
archive_volume=""
backups_volume=""

fixture() {
  env -u GOROOT go run ./tests/fixtures/upgrade-fixture "$@"
}

compose() {
  docker compose --project-name "$compose_project" -f compose.yaml -f "$override" "$@"
}

cleanup() {
  compose down -v --remove-orphans >/dev/null 2>&1 || true
  rm -rf "$temp_dir"
}
trap cleanup EXIT

if [[ "$image" != *@sha256:* || "$previous_image" != *@sha256:* ]]; then
  printf 'Both images must be immutable @sha256 digests\n' >&2
  exit 2
fi

case "$(docker image inspect "$image" --format '{{.Architecture}}')" in
  amd64|arm64) platform="linux/$(docker image inspect "$image" --format '{{.Architecture}}')" ;;
  *) printf 'Unsupported candidate image architecture\n' >&2; exit 2 ;;
esac

docker pull --platform "$platform" "$previous_image" >/dev/null
fixture --verify-image --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --image "$previous_image"

for selected_image in "$image" "$previous_image"; do
  variant="$(docker image inspect "$selected_image" --format '{{ index .Config.Labels "io.hitkeep.variant" }}')"
  if [[ "$variant" != "$expected_variant" ]]; then
    printf 'Expected image %s to have variant %s, got %s\n' "$selected_image" "$expected_variant" "$variant" >&2
    exit 1
  fi
done

export HITKEEP_JWT_SECRET=hitkeep-local-compose-smoke-secret
export HITKEEP_PUBLIC_URL=http://localhost:8080
export HITKEEP_MAIL_DRIVER=log
export HITKEEP_SPAM_FILTER_AUTO_UPDATE=false

write_override() {
  local selected_image="$1"
  local suffix="$2"
  data_volume="${compose_project}-${suffix}-data"
  archive_volume="${compose_project}-${suffix}-archive"
  backups_volume="${compose_project}-${suffix}-backups"
  cat > "$override" <<EOF
services:
  hitkeep:
    build: !reset null
    image: ${selected_image}
    container_name: ${container}
    restart: "no"
    platform: ${platform}
    ports: !override
      - "127.0.0.1::8080"
    volumes:
      - data:/var/lib/hitkeep/data
      - archive:/var/lib/hitkeep/archive
      - backups:/var/lib/hitkeep/backups
volumes:
  data:
    name: ${data_volume}
  archive:
    name: ${archive_volume}
  backups:
    name: ${backups_volume}
EOF
}

await_healthy() {
  for _ in $(seq 1 45); do
    if compose exec -T hitkeep hitkeep -healthcheck >/dev/null 2>&1; then
      return 0
    fi
    if [[ "$(docker inspect "$container" --format '{{.State.Running}}' 2>/dev/null || true)" != "true" ]]; then
      compose logs >&2
      return 1
    fi
    sleep 1
  done
  compose logs >&2
  printf 'Compose service did not become healthy within 45 seconds: %s\n' "$image" >&2
  return 1
}

service_url() {
  local published
  published="$(compose port hitkeep 8080 | head -n 1)"
  printf 'http://%s' "$published"
}

stop_and_down() {
  local exit_code
  docker stop -t 15 "$container" >/dev/null
  exit_code="$(docker inspect "$container" --format '{{.State.ExitCode}}')"
  if [[ "$exit_code" != "0" ]]; then
    printf 'Compose service %s exited with %s during graceful stop\n' "$container" "$exit_code" >&2
    return 1
  fi
  compose down --remove-orphans >/dev/null
}

archive_data_volume() {
  local archive="$1"
  docker run --rm --platform "$platform" \
    --mount "type=volume,src=$data_volume,dst=/source/data,readonly" \
    "$rollback_helper_image" tar -C /source -cpf - data > "$archive"
}

verify_stopped_storage() {
  local verifier="$1"
  local snapshot="$temp_dir/$verifier-$RANDOM"
  local tarball="$snapshot.tar"
  mkdir -p "$snapshot"
  archive_data_volume "$tarball"
  tar -C "$snapshot" -xpf "$tarball"
  fixture "--${verifier}" --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --data-path "$snapshot/data" --metadata "$tarball"
}

restore_data_volume() {
  local archive="$1"
  docker volume create \
    --label "com.docker.compose.project=${compose_project}" \
    --label 'com.docker.compose.volume=data' \
    "$data_volume" >/dev/null
  docker run --rm --platform "$platform" \
    --mount "type=volume,src=$data_volume,dst=/target" \
    --mount "type=bind,src=$temp_dir,dst=/backup,readonly" \
    "$rollback_helper_image" tar -C /target --strip-components=1 -xpf "/backup/$(basename "$archive")"
}

write_override "$previous_image" primary
compose up -d --force-recreate
await_healthy
fixture --seed --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$(service_url)"
stop_and_down
verify_stopped_storage verify-legacy-storage
legacy_archive="$temp_dir/legacy-volume.tar"
archive_data_volume "$legacy_archive"

write_override "$image" primary
compose up -d --force-recreate
await_healthy
fixture --verify --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$(service_url)"
stop_and_down
verify_stopped_storage verify-storage

compose up -d --force-recreate
await_healthy
fixture --verify --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$(service_url)"
stop_and_down
verify_stopped_storage verify-storage

compose down -v --remove-orphans >/dev/null
write_override "$previous_image" rollback
restore_data_volume "$legacy_archive"
compose up -d --force-recreate
await_healthy
fixture --verify --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$(service_url)"
stop_and_down
verify_stopped_storage verify-legacy-storage

compose up -d --force-recreate
await_healthy
fixture --verify --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$(service_url)"
stop_and_down
verify_stopped_storage verify-legacy-storage

printf 'Compose upgrade, recreation, and rollback preserved release fixture data: %s (%s)\n' "$image" "$expected_variant"
