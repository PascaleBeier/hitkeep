package devtool

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestHelmSmokeDeployPassesCandidateChartDigestFlags(t *testing.T) {
	got := helmDeployArgs(t, "2.13.12", "registry.example/hitkeep@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "candidate.tgz")
	want := []string{
		"upgrade", "--install", "hitkeep", "candidate.tgz",
		"--namespace", "smoke", "--create-namespace", "--wait", "--timeout", "5m",
		"--set-string", "image.repository=registry.example/hitkeep",
		"--set-string", "image.digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"--set-string", "env.HITKEEP_JWT_SECRET=hitkeep-local-helm-smoke-secret",
		"--set-string", "env.HITKEEP_MAIL_DRIVER=log",
		"--set-string", "env.HITKEEP_SPAM_FILTER_AUTO_UPDATE=false",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("helm argv = %#v, want %#v", got, want)
	}
}

func TestHelmSmokeDeployPassesLegacyChartDigestCompatibleTag(t *testing.T) {
	got := helmDeployArgs(t, "2.12.0", "registry.example/hitkeep@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "legacy.tgz")
	want := []string{
		"upgrade", "--install", "hitkeep", "legacy.tgz",
		"--namespace", "smoke", "--create-namespace", "--wait", "--timeout", "5m",
		"--set-string", "image.repository=registry.example/hitkeep@sha256",
		"--set-string", "image.tag=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"--set-string", "env.HITKEEP_JWT_SECRET=hitkeep-local-helm-smoke-secret",
		"--set-string", "env.HITKEEP_MAIL_DRIVER=log",
		"--set-string", "env.HITKEEP_SPAM_FILTER_AUTO_UPDATE=false",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("helm argv = %#v, want %#v", got, want)
	}
	if slices.Contains(got, "image.digest=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb") {
		t.Fatal("legacy chart must receive image.tag, not unsupported image.digest")
	}
}

func TestHelmSmokeRejectsMutableImageReferences(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/helm-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	command := strings.Join([]string{
		shellFunction(t, string(raw), "image_values"),
		"image_values registry.example/hitkeep:latest",
	}, "\n")
	if output, err := exec.Command("bash", "-c", command).CombinedOutput(); err == nil {
		t.Fatalf("mutable image reference was accepted: %s", output)
	}
}

func helmDeployArgs(t *testing.T, chartVersion, image, chart string) []string {
	t.Helper()
	raw, err := os.ReadFile("../../scripts/helm-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	command := strings.Join([]string{
		"release=hitkeep",
		"namespace=smoke",
		shellFunction(t, string(raw), "image_values"),
		shellFunction(t, string(raw), "deploy"),
		"helm() { if [[ \"$1\" == show ]]; then printf \"version: %s\\n\" \"$chart_version\"; else printf \"%s\\n\" \"$@\"; fi; }",
		"chart_version=" + chartVersion,
		"deploy " + image + " " + chart,
	}, "\n")
	output, err := exec.Command("bash", "-c", command).CombinedOutput()
	if err != nil {
		t.Fatalf("deploy shell execution failed: %v\n%s", err, output)
	}
	return strings.Split(strings.TrimSpace(string(output)), "\n")
}

func TestReleaseWorkflowRunsHelmSmokeWithBash(t *testing.T) {
	raw, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "bash ./scripts/helm-smoke.sh \"$CANDIDATE_IMAGE\" self-hosted") {
		t.Fatal("release workflow must run non-executable helm-smoke.sh through bash")
	}
}

func shellFunction(t *testing.T, script, name string) string {
	t.Helper()
	declaration := name + "() {\n"
	start := strings.Index(script, declaration)
	if start < 0 {
		t.Fatalf("%s is missing", name)
	}
	end := strings.Index(script[start:], "\n}\n\n")
	if end < 0 {
		t.Fatalf("%s is unterminated", name)
	}
	return script[start : start+end+3]
}
