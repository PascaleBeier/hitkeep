#!/usr/bin/env bash

set -euo pipefail

image="${1:?usage: helm-smoke.sh IMAGE EXPECTED_VARIANT}"
expected_variant="${2:?usage: helm-smoke.sh IMAGE EXPECTED_VARIANT}"
previous_image="${HITKEEP_PREVIOUS_IMAGE:?HITKEEP_PREVIOUS_IMAGE must name an immutable supported 2.x image digest}"
previous_chart="${HITKEEP_PREVIOUS_CHART:?HITKEEP_PREVIOUS_CHART must name the immutable supported 2.12 chart artifact}"
previous_chart_digest="${HITKEEP_PREVIOUS_CHART_DIGEST:?HITKEEP_PREVIOUS_CHART_DIGEST must name the immutable supported 2.12 chart manifest}"
candidate_chart="${HITKEEP_CANDIDATE_CHART:?HITKEEP_CANDIDATE_CHART must name the exact candidate chart artifact}"
candidate_chart_version="${HITKEEP_CANDIDATE_CHART_VERSION:?HITKEEP_CANDIDATE_CHART_VERSION must name the candidate chart version}"
kind_cluster="${HITKEEP_KIND_CLUSTER:?HITKEEP_KIND_CLUSTER must name the disposable Kind cluster}"
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
for chart in "$previous_chart" "$candidate_chart"; do
  if ! helm show chart "$chart" >/dev/null; then
    printf 'Chart artifact %s is invalid\n' "$chart" >&2
    exit 2
  fi
done
if [[ ! "$previous_chart_digest" =~ ^sha256:[a-f0-9]{64}$ ]] ||
  ! helm show chart "$previous_chart" | grep -Fqx 'version: 2.12.0' ||
  ! helm show chart "$candidate_chart" | grep -Fqx "version: $candidate_chart_version"; then
  printf 'Helm chart identity is not the required v2.12/candidate pair\n' >&2
  exit 2
fi

platform="linux/$(docker image inspect "$image" --format '{{.Architecture}}')"
docker pull --platform "$platform" "$previous_image" >/dev/null
kind load docker-image --name "$kind_cluster" "$image"
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
  if [[ ! "$reference" =~ ^[^@]+@sha256:[a-f0-9]{64}$ ]]; then
    printf 'Image %s must be an immutable sha256 digest reference\n' "$reference" >&2
    return 2
  fi
  repository="${reference%@*}"
  digest="${reference#*@}"
}

deploy() {
  local chart="$2"
  local image_args=()
  image_values "$1"
  if helm show chart "$chart" | grep -Fqx 'version: 2.12.0'; then
    image_args=(
      --set-string image.repository="${repository}@sha256"
      --set-string image.tag="${digest#sha256:}"
    )
  else
    image_args=(
      --set-string image.repository="$repository"
      --set-string image.digest="$digest"
    )
  fi
  helm upgrade --install "$release" "$chart" \
    --namespace "$namespace" \
    --create-namespace \
    --wait \
    --timeout 5m \
    "${image_args[@]}" \
    --set-string env.HITKEEP_JWT_SECRET=hitkeep-local-helm-smoke-secret \
    --set-string env.HITKEEP_MAIL_DRIVER=log \
    --set-string env.HITKEEP_SPAM_FILTER_AUTO_UPDATE=false
}

await_healthy() {
  kubectl -n "$namespace" rollout status statefulset/"$release" --timeout=5m
  port_forward_log="$temp_dir/port-forward.log"
  kubectl -n "$namespace" port-forward --address 127.0.0.1 service/"$release" 0:http >"$port_forward_log" 2>&1 &
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
  local restarts current_restarts exit_code
  restarts="$(kubectl -n "$namespace" get pod "${release}-0" -o jsonpath='{.status.containerStatuses[0].restartCount}')"
  kubectl -n "$namespace" exec "${release}-0" -- /bin/sh -c 'kill -TERM 1'
  for _ in {1..60}; do
    current_restarts="$(kubectl -n "$namespace" get pod "${release}-0" -o jsonpath='{.status.containerStatuses[0].restartCount}' 2>/dev/null || true)"
    exit_code="$(kubectl -n "$namespace" get pod "${release}-0" -o jsonpath='{.status.containerStatuses[0].lastState.terminated.exitCode}' 2>/dev/null || true)"
    if [[ "$current_restarts" =~ ^[0-9]+$ ]] && (( current_restarts > restarts )) && [[ "$exit_code" == "0" ]]; then
      kubectl -n "$namespace" wait --for=condition=Ready pod/"${release}-0" --timeout=5m
      return
    fi
    sleep 1
  done
  printf 'HitKeep did not restart cleanly after SIGTERM (initial restart count %s)\n' "$restarts" >&2
  return 1
}

quiesce_release() {
  stop_port_forward
  graceful_shutdown
  kubectl -n "$namespace" scale statefulset/"$release" --replicas=0
  kubectl -n "$namespace" wait --for=delete pod/"${release}-0" --timeout=5m
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

restart_stateful_pod() {
  local pod actual_pvc
  kubectl -n "$namespace" rollout status statefulset/$release --timeout=2m
  pod="$(kubectl -n "$namespace" get pods -l app.kubernetes.io/instance="$release" -o jsonpath='{.items[0].metadata.name}')"
  if [[ -z "$pod" ]]; then
    printf 'StatefulSet %s has no pod to recreate\n' "$release" >&2
    exit 1
  fi
  stop_port_forward
  kubectl -n "$namespace" delete pod "$pod" --wait=true
  kubectl -n "$namespace" rollout status statefulset/$release --timeout=2m
  pod="$(kubectl -n "$namespace" get pods -l app.kubernetes.io/instance="$release" -o jsonpath='{.items[0].metadata.name}')"
  actual_pvc="$(kubectl -n "$namespace" get pod "$pod" -o jsonpath='{.spec.volumes[?(@.persistentVolumeClaim)].persistentVolumeClaim.claimName}')"
  if [[ "$actual_pvc" != "$pvc" ]]; then
    printf 'Expected recreated pod %s to use PVC %s, got %s\n' "$pod" "$pvc" "$actual_pvc" >&2
    exit 1
  fi
  await_healthy
}

deploy "$previous_image" "$previous_chart"
await_healthy
fixture --seed --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$service_url"
quiesce_release
verify_stopped_storage verify-legacy-storage
legacy_archive="$temp_dir/legacy-pvc.tar"
archive_pvc "$legacy_archive"

deploy "$image" "$candidate_chart"
await_healthy
restart_stateful_pod
fixture --verify --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$service_url"
quiesce_release
verify_stopped_storage verify-storage

deploy "$image" "$candidate_chart"
await_healthy
restart_stateful_pod
fixture --verify --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$service_url"
quiesce_release
verify_stopped_storage verify-storage

restore_pvc "$legacy_archive"
deploy "$previous_image" "$previous_chart"
await_healthy
fixture --verify --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$service_url"
quiesce_release
verify_stopped_storage verify-legacy-storage

deploy "$image" "$candidate_chart"
await_healthy
restart_stateful_pod
fixture --verify --manifest "$fixture_manifest" --previous-image "$previous_image" --platform "$platform" --url "$service_url"
quiesce_release
verify_stopped_storage verify-storage

printf 'Helm upgrade, recreation, and rollback preserved release fixture data: %s (%s)\n' "$image" "$expected_variant"
