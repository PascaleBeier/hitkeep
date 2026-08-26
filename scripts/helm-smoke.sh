#!/usr/bin/env bash

set -euo pipefail

image="${1:?usage: helm-smoke.sh IMAGE EXPECTED_VARIANT}"
expected_variant="${2:?usage: helm-smoke.sh IMAGE EXPECTED_VARIANT}"
previous_image="${HITKEEP_PREVIOUS_IMAGE:?HITKEEP_PREVIOUS_IMAGE must name an immutable supported 2.x image digest}"
fixture_manifest="tests/fixtures/release-fixtures.json"
rollback_helper_image="busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662"
namespace="hitkeep-helm-smoke-$$-${RANDOM}"
release="hitkeep"
pvc="data-${release}-0"
temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/hitkeep-helm-smoke.XXXXXX")"
port_forward_pid=""

fixture() {
  go run ./tests/fixtures/upgrade-fixture "$@"
}

cleanup() {
  if [[ -n "$port_forward_pid" ]]; then
    kill "$port_forward_pid" 2>/dev/null || true
    wait "$port_forward_pid" 2>/dev/null || true
  fi
  helm uninstall "$release" --namespace "$namespace" --wait >/dev/null 2>&1 || true
  kubectl delete namespace "$namespace" --wait=false >/dev/null 2>&1 || true
  rm -rf "$temp_dir"
}
trap cleanup EXIT

if [[ "$image" != *@sha256:* || "$previous_image" != *@sha256:* ]]; then
  printf 'Both images must be immutable @sha256 digests\n' >&2
  exit 2
fi

platform="linux/$(docker image inspect "$image" --format '{{.Architecture}}')"
docker pull --platform "$platform" "$previous_image" >/dev/null
fixture --verify-image --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --image "$previous_image"

for selected_image in "$image" "$previous_image"; do
  variant="$(docker image inspect "$selected_image" --format '{{ index .Config.Labels "io.hitkeep.variant" }}')"
  if [[ "$variant" != "$expected_variant" ]]; then
    printf 'Expected image %s to have variant %s, got %s\n' "$selected_image" "$expected_variant" "$variant" >&2
    exit 1
  fi
done

image_values() {
  local reference="$1"
  repository="${reference%@*}"
  digest="${reference#*@}"
}

deploy() {
  image_values "$1"
  helm upgrade --install "$release" charts/hitkeep \
    --namespace "$namespace" \
    --create-namespace \
    --wait \
    --timeout 5m \
    --set-string image.repository="$repository" \
    --set-string image.digest="$digest" \
    --set-string env.HITKEEP_JWT_SECRET=hitkeep-local-helm-smoke-secret \
    --set-string env.HITKEEP_MAIL_DRIVER=log \
    --set-string env.HITKEEP_SPAM_FILTER_AUTO_UPDATE=false
}

await_healthy() {
  kubectl -n "$namespace" rollout status statefulset/"$release" --timeout=5m
  port_forward_log="$temp_dir/port-forward.log"
  kubectl -n "$namespace" port-forward --address 127.0.0.1 service/"$release" 0:8080 >"$port_forward_log" 2>&1 &
  port_forward_pid=$!
  for _ in {1..60}; do
    port="$(sed -nE 's/.*127\.0\.0\.1:([0-9]+).*/\1/p' "$port_forward_log" | head -n1)"
    if [[ -n "${port:-}" ]] && curl -fsS "http://127.0.0.1:${port}/readyz" >/dev/null; then
      service_url="http://127.0.0.1:${port}"
      return
    fi
    sleep 1
  done
  cat "$port_forward_log" >&2 || true
  return 1
}

stop_port_forward() {
  kill "$port_forward_pid" 2>/dev/null || true
  wait "$port_forward_pid" 2>/dev/null || true
  port_forward_pid=""
}

graceful_shutdown() {
  local restarts exit_code
  restarts="$(kubectl -n "$namespace" get pod "${release}-0" -o jsonpath='{.status.containerStatuses[0].restartCount}')"
  kubectl -n "$namespace" exec "${release}-0" -- /bin/sh -c 'kill -TERM 1'
  for _ in {1..60}; do
    exit_code="$(kubectl -n "$namespace" get pod "${release}-0" -o jsonpath='{.status.containerStatuses[0].lastState.terminated.exitCode}' 2>/dev/null || true)"
    if [[ "$exit_code" == "0" ]]; then
      kubectl -n "$namespace" rollout status statefulset/"$release" --timeout=5m
      return
    fi
    sleep 1
  done
  printf 'HitKeep did not exit cleanly after SIGTERM (initial restart count %s)\n' "$restarts" >&2
  return 1
}

stop_release() {
  stop_port_forward
  graceful_shutdown
  helm uninstall "$release" --namespace "$namespace" --wait
}

mount_pvc() {
  local pod="$1"
  kubectl -n "$namespace" delete pod "$pod" --ignore-not-found --wait=true >/dev/null
  cat <<YAML | kubectl -n "$namespace" apply -f - >/dev/null
apiVersion: v1
kind: Pod
metadata:
  name: ${pod}
spec:
  restartPolicy: Never
  containers:
    - name: files
      image: ${rollback_helper_image}
      command: ["sh", "-c", "sleep 600"]
      volumeMounts:
        - name: data
          mountPath: /data
  volumes:
    - name: data
      persistentVolumeClaim:
        claimName: ${pvc}
YAML
  kubectl -n "$namespace" wait --for=condition=Ready pod/"$pod" --timeout=2m
}

verify_stopped_storage() {
  local mode="$1" pod="storage-check" data_path="$temp_dir/$1-data" metadata="$temp_dir/$1.tar"
  mkdir -p "$data_path"
  mount_pvc "$pod"
  kubectl -n "$namespace" cp "$pod:/data/." "$data_path"
  kubectl -n "$namespace" exec "$pod" -- tar -C /data -cf - . >"$metadata"
  kubectl -n "$namespace" delete pod "$pod" --wait=true >/dev/null
  fixture --"$mode" --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --data-path "$data_path" --metadata "$metadata"
}

archive_pvc() {
  local archive="$1"
  mount_pvc archive-source
  kubectl -n "$namespace" exec archive-source -- tar -C /data -cf - . >"$archive"
  kubectl -n "$namespace" delete pod archive-source --wait=true >/dev/null
}

restore_pvc() {
  local archive="$1" storage_class
  storage_class="$(kubectl -n "$namespace" get pvc "$pvc" -o jsonpath='{.spec.storageClassName}')"
  kubectl -n "$namespace" delete pvc "$pvc" --wait=true
  cat <<YAML | kubectl -n "$namespace" apply -f - >/dev/null
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ${pvc}
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: ${storage_class}
YAML
  kubectl -n "$namespace" wait --for=jsonpath='{.status.phase}'=Bound pvc/"$pvc" --timeout=2m
  mount_pvc archive-restore
  kubectl -n "$namespace" exec -i archive-restore -- tar -C /data -xpf - <"$archive"
  kubectl -n "$namespace" delete pod archive-restore --wait=true >/dev/null
}

deploy "$previous_image"
await_healthy
fixture --seed --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$service_url"
stop_release
verify_stopped_storage verify-legacy-storage
legacy_archive="$temp_dir/legacy-pvc.tar"
archive_pvc "$legacy_archive"

deploy "$image"
await_healthy
fixture --verify --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$service_url"
stop_release
verify_stopped_storage verify-storage

deploy "$image"
await_healthy
fixture --verify --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$service_url"
stop_release
verify_stopped_storage verify-storage

restore_pvc "$legacy_archive"
deploy "$previous_image"
await_healthy
fixture --verify --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$service_url"
stop_release
verify_stopped_storage verify-legacy-storage

deploy "$image"
await_healthy
fixture --verify --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$service_url"
stop_release
verify_stopped_storage verify-storage

printf 'Helm upgrade, recreation, and rollback preserved release fixture data: %s (%s)\n' "$image" "$expected_variant"
