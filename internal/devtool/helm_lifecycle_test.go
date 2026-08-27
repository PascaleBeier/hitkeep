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
		"if [[ \"$image\" != *@sha256:* || \"$previous_image\" != *@sha256:* ]]",
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
	if got := strings.Count(script, "restart_stateful_pod"); got < 4 {
		t.Errorf("restart_stateful_pod occurrences = %d, want declaration plus three candidate restarts", got)
	}
}

func TestReleaseWorkflowAuthenticatesHelmSmoke(t *testing.T) {
	raw, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}

	workflow := string(raw)
	for _, want := range []string{"docker/login-action", "scripts/helm-smoke.sh", "ghcr.io"} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release workflow must contain %q", want)
		}
	}
}
