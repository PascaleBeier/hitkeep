package devtool

import (
	"os"
	"strings"
	"testing"
)

func TestHelmSmokeLifecycleContract(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/helm-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}

	script := string(raw)
	for _, want := range []string{
		"HITKEEP_PREVIOUS_IMAGE:?HITKEEP_PREVIOUS_IMAGE must name an immutable supported 2.x image digest",
		"HITKEEP_PREVIOUS_CHART:?HITKEEP_PREVIOUS_CHART must name the immutable supported 2.12 chart artifact",
		"HITKEEP_PREVIOUS_CHART_DIGEST:?HITKEEP_PREVIOUS_CHART_DIGEST must name the immutable supported 2.12 chart manifest",
		"HITKEEP_CANDIDATE_CHART:?HITKEEP_CANDIDATE_CHART must name the exact candidate chart artifact",
		"HITKEEP_CANDIDATE_CHART_VERSION:?HITKEEP_CANDIDATE_CHART_VERSION must name the candidate chart version",
		"grep -Fqx 'version: 2.12.0'",
		"grep -Fqx \"version: $candidate_chart_version\"",
		"if [[ \"$image\" != *@sha256:* || \"$previous_image\" != *@sha256:* ]]",
		"docker pull --platform \"$platform\" \"$previous_image\"",
		"helm upgrade --install \"$release\" \"$chart\"",
		"fixture --verify-image",
		"fixture --seed",
		"statefulset/$release",
		"kubectl -n \"$namespace\" delete pod \"$pod\" --wait=true",
		"kubectl -n \"$namespace\" get pod \"$pod\" -o jsonpath='{.spec.volumes[?(@.persistentVolumeClaim)].persistentVolumeClaim.claimName}'",
		"Expected recreated pod %s to use PVC %s, got %s",
		"restore_pvc \"$legacy_archive\"\ndeploy \"$previous_image\"",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("helm smoke script must contain %q", want)
		}
	}
	for _, forbidden := range []string{"HITKEEP_KIND_CLUSTER", "kind load docker-image"} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("helm smoke script must not contain %q", forbidden)
		}
	}
	if got := strings.Count(script, "restart_stateful_pod"); got < 4 {
		t.Errorf("restart_stateful_pod occurrences = %d, want declaration plus three candidate restarts", got)
	}
}

func TestReleaseWorkflowsUseManifestUpgradeFloor(t *testing.T) {
	for _, path := range []string{"../../.github/workflows/release.yml", "../../.github/workflows/release-validation.yml"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		workflow := string(raw)
		requiredFragments := []string{"supported_upgrade_floor"}
		if strings.HasSuffix(path, "release.yml") {
			requiredFragments = append(requiredFragments, "PREVIOUS_VERSION: ${{ steps.fixture.outputs.previous_version }}")
		}
		for _, required := range requiredFragments {
			if !strings.Contains(workflow, required) {
				t.Errorf("%s must propagate manifest floor as %q", path, required)
			}
		}
		for _, forbidden := range []string{
			`select(.previous_version == "2.12.0"`,
			`--version 2.12.0`,
			`hitkeep-2.12.0.tgz`,
			`Upgrade from v2.12.0`,
			`upgrade-from-v2-12`,
		} {
			if strings.Contains(workflow, forbidden) {
				t.Errorf("%s must derive predecessor from manifest, found %q", path, forbidden)
			}
		}
	}
}

func TestReleaseWorkflowAuthenticatesHelmSmoke(t *testing.T) {
	raw, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}

	workflow := string(raw)
	for _, want := range []string{
		"docker/login-action",
		"previous_chart_digest",
		"HITKEEP_PREVIOUS_CHART:",
		"HITKEEP_PREVIOUS_CHART=\"$previous_chart\"",
		"HITKEEP_CANDIDATE_CHART=\"$candidate_chart\"",
		"HITKEEP_CANDIDATE_CHART_VERSION=\"$CANDIDATE_VERSION\"",
		"helm package charts/hitkeep",
		"helm pull \"$HITKEEP_PREVIOUS_CHART\"",
		"scripts/helm-smoke.sh",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release workflow must contain %q", want)
		}
	}
	if strings.Contains(workflow, "HITKEEP_KIND_CLUSTER=") {
		t.Fatal("release workflow must not configure the removed Kind image import")
	}
	if strings.Contains(workflow, "GHCR_TOKEN: ${{ github.token }}") || strings.Contains(workflow, "printf '%s' \"$GHCR_TOKEN\" | helm registry login") {
		t.Fatal("public legacy Helm chart pull must not require a registry secret")
	}
}
