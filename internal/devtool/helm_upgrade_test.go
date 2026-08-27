package devtool

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		"helm upgrade --install \"$release\" \"$chart\"",
		"--namespace \"$namespace\"",
		"--set-string image.repository=\"$repository\"",
		"--set-string image.digest=\"$digest\"",
		"kubectl -n \"$namespace\" port-forward --address 127.0.0.1 service/\"$release\" 0:8080",
		"fixture --seed",
		"fixture --verify",
		"quiesce_release",
		"kubectl -n \"$namespace\" scale statefulset/\"$release\" --replicas=0",
		"(( current_restarts > restarts ))",
		"kubectl -n \"$namespace\" wait --for=condition=Ready pod/\"${release}-0\" --timeout=5m",
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
	legacySeed := bytes.Index(raw, []byte("fixture --seed"))
	candidate := bytes.Index(raw, []byte(`deploy "$image"`))
	if legacySeed < 0 || candidate < 0 {
		t.Fatal("helm upgrade smoke does not install the candidate image after the legacy fixture seed")
	}
	legacyToCandidate := raw[legacySeed:candidate]
	if bytes.Contains(legacyToCandidate, []byte("helm uninstall")) {
		t.Fatal("helm upgrade smoke must retain the legacy Helm release until the first candidate upgrade")
	}
	quiesce := bytes.Index(legacyToCandidate, []byte("quiesce_release"))
	archive := bytes.Index(legacyToCandidate, []byte(`archive_pvc "$legacy_archive"`))
	if quiesce < 0 || archive < quiesce {
		t.Fatal("helm upgrade smoke must quiesce and snapshot the legacy PVC before the first candidate upgrade")
	}
	if !bytes.Contains(raw[candidate:], []byte("verify_stopped_storage verify-storage")) {
		t.Fatal("helm upgrade smoke must verify candidate storage before rollback")
	}
	rollback := bytes.Index(raw[candidate:], []byte(`deploy "$previous_image"`))
	if rollback < 0 {
		t.Fatal("helm upgrade smoke must install the historical image on the restored PVC")
	}
	if !bytes.Contains(raw[candidate+rollback:], []byte(`deploy "$image"`)) {
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

func TestHelmTemplateRendersImmutableReferencesForLegacyAndCandidateCharts(t *testing.T) {
	if os.Getenv("HITKEEP_HELM_TEMPLATE_CONTRACT") != "1" {
		t.Skip("set HITKEEP_HELM_TEMPLATE_CONTRACT=1 to render the immutable v2.12 chart fixture")
	}
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Fatal(err)
	}
	const repository = "registry.example/hitkeep"
	const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	want := `image: "` + repository + `@sha256:` + digest + `"`

	legacyDir := t.TempDir()
	if output, err := exec.Command(helm, "pull", "oci://ghcr.io/pascalebeier/charts/hitkeep", "--version", "2.12.0", "--destination", legacyDir).CombinedOutput(); err != nil {
		t.Fatalf("pull v2.12 chart: %v\n%s", err, output)
	}
	renders := []struct {
		chart  string
		values []string
	}{
		{
			chart: filepath.Join(legacyDir, "hitkeep-2.12.0.tgz"),
			values: []string{
				"--set-string", "image.repository=" + repository + "@sha256",
				"--set-string", "image.tag=" + digest,
			},
		},
		{
			chart: filepath.Join("..", "..", "charts", "hitkeep"),
			values: []string{
				"--set-string", "image.repository=" + repository,
				"--set-string", "image.digest=sha256:" + digest,
			},
		},
	}
	for _, render := range renders {
		args := append([]string{"template", "hitkeep", render.chart}, render.values...)
		output, err := exec.Command(helm, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("render %s: %v\n%s", render.chart, err, output)
		}
		rendered := string(output)
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered image for %s does not preserve the immutable digest %q\n%s", render.chart, want, rendered)
		}
		if strings.Contains(rendered, repository+":2.12.0:") || strings.Contains(rendered, repository+"@sha256@") {
			t.Fatalf("rendered image for %s is malformed\n%s", render.chart, rendered)
		}
	}
}

func TestReleaseWorkflowGatesHelmUpgradeFromSupportedFloor(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"name: Helm upgrade from v2.12.0",
		"name: Helm upgrade from v2.12.0",
		"go install sigs.k8s.io/kind@v0.29.0",
		"create cluster --name \"$cluster\"",
		"./scripts/helm-smoke.sh \"$CANDIDATE_IMAGE\" self-hosted",
		"- surface: helm",
		"needs.upgrade-from-v2-12.result == 'success'",
	} {
		if !bytes.Contains(raw, []byte(required)) {
			t.Fatalf("release workflow is missing Helm upgrade contract %q", required)
		}
	}
}
