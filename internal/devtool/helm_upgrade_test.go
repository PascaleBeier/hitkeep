package devtool

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestHelmUpgradeSmokeUsesChartPersistenceAndImmutableImages(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "helm-smoke.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"HITKEEP_PREVIOUS_IMAGE",
		"@sha256:",
		"hitkeep-helm-smoke-$$-${RANDOM}",
		"helm upgrade --install \"$release\" charts/hitkeep",
		"--namespace \"$namespace\"",
		"--set-string image.repository=\"$repository\"",
		"--set-string image.digest=\"$digest\"",
		"kubectl -n \"$namespace\" port-forward --address 127.0.0.1 service/\"$release\" 0:8080",
		"fixture --seed",
		"fixture --verify",
		"verify_stopped_storage verify-storage",
		"verify_stopped_storage verify-legacy-storage",
		"archive_pvc \"$legacy_archive\"",
		"restore_pvc \"$legacy_archive\"",
		"helm uninstall \"$release\" --namespace \"$namespace\" --wait",
		"rollback_helper_image=\"busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662\"",
		"kubectl delete namespace \"$namespace\" --wait=false",
	} {
		if !bytes.Contains(raw, []byte(required)) {
			t.Fatalf("helm upgrade smoke is missing contract %q", required)
		}
	}
	candidate := bytes.Index(raw, []byte(`deploy "$image"`))
	if candidate < 0 {
		t.Fatal("helm upgrade smoke does not install the candidate image")
	}
	if bytes.Index(raw[candidate:], []byte("verify_stopped_storage verify-storage")) < 0 {
		t.Fatal("helm upgrade smoke must verify candidate storage before rollback")
	}
	rollback := bytes.Index(raw[candidate:], []byte(`deploy "$previous_image"`))
	if rollback < 0 {
		t.Fatal("helm upgrade smoke must install the historical image on the restored PVC")
	}
	if bytes.Index(raw[candidate+rollback:], []byte(`deploy "$image"`)) < 0 {
		t.Fatal("helm upgrade smoke must resume the candidate after rollback verification")
	}
}

func TestHelmChartSupportsDigestPinnedSmokeImages(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "charts", "hitkeep", "templates", "statefulset.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`image: "{{ .Values.image.repository }}{{ if .Values.image.digest }}@{{ .Values.image.digest }}{{ else }}:{{ $imageTag }}{{ end }}"`)) {
		t.Fatal("chart must render an immutable image digest when image.digest is set")
	}
	values, err := os.ReadFile(filepath.Join("..", "..", "charts", "hitkeep", "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(values, []byte("  digest: \"\"")) {
		t.Fatal("chart values must expose an empty digest default for tagged deployments")
	}
}

func TestReleaseWorkflowGatesHelmUpgradeFromSupportedFloor(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"upgrade-helm-from-v2-12:",
		"name: Helm upgrade from v2.12.0",
		"go install sigs.k8s.io/kind@v0.29.0",
		"create cluster --name \"$cluster\"",
		"HITKEEP_PREVIOUS_IMAGE=\"$previous\" ./scripts/helm-smoke.sh \"$candidate\" self-hosted",
		"- upgrade-helm-from-v2-12",
		"needs.upgrade-helm-from-v2-12.result == 'success'",
	} {
		if !bytes.Contains(raw, []byte(required)) {
			t.Fatalf("release workflow is missing Helm upgrade contract %q", required)
		}
	}
}
