package devtool

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestHelmSmokeDeployPassesChartAndFlags(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/helm-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}

	script := string(raw)
	command := strings.Join([]string{
		"release=hitkeep",
		"namespace=smoke",
		shellFunction(t, script, "image_values"),
		shellFunction(t, script, "deploy"),
		"helm() { printf '%s\\n' \"$@\"; }",
		"deploy registry.example/hitkeep@sha256:candidate candidate.tgz",
	}, "\n")
	output, err := exec.Command("bash", "-c", command).CombinedOutput()
	if err != nil {
		t.Fatalf("deploy shell execution failed: %v\n%s", err, output)
	}

	got := strings.Split(strings.TrimSpace(string(output)), "\n")
	want := []string{
		"upgrade", "--install", "hitkeep", "candidate.tgz",
		"--namespace", "smoke", "--create-namespace", "--wait", "--timeout", "5m",
		"--set-string", "image.repository=registry.example/hitkeep",
		"--set-string", "image.digest=sha256:candidate",
		"--set-string", "env.HITKEEP_JWT_SECRET=hitkeep-local-helm-smoke-secret",
		"--set-string", "env.HITKEEP_MAIL_DRIVER=log",
		"--set-string", "env.HITKEEP_SPAM_FILTER_AUTO_UPDATE=false",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("helm argv = %#v, want %#v", got, want)
	}
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
