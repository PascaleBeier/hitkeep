package devtool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestGitHubReleasePromotionVerifiesPublishedAndLatest(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is required to execute the release promotion script")
	}
	raw, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct{ ID, Run string }
		}
	}
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	var script string
	for _, step := range workflow.Jobs["finalize-release"].Steps {
		if step.ID == "promote_github_release" {
			script = step.Run
		}
	}
	if script == "" {
		t.Fatal("missing GitHub release promotion script")
	}
	for _, scenario := range []string{"published", "draft", "wrong-tag", "wrong-latest", "api-failure"} {
		t.Run(scenario, func(t *testing.T) {
			outputPath := filepath.Join(t.TempDir(), "output")
			stub := `gh() {
  case "$*" in
    'release edit v2.99.0 --draft=false --latest') return 0 ;;
    'release view v2.99.0 --json isDraft,tagName')
      case "$SCENARIO" in
        draft) echo '{"isDraft":true,"tagName":"v2.99.0"}' ;;
        wrong-tag) echo '{"isDraft":false,"tagName":"v2.98.0"}' ;;
        *) echo '{"isDraft":false,"tagName":"v2.99.0"}' ;;
      esac ;;
    'api repos/test/hitkeep/releases/latest')
      case "$SCENARIO" in
        wrong-latest) echo '{"tag_name":"v2.98.0"}' ;;
        api-failure) return 1 ;;
        *) echo '{"tag_name":"v2.99.0"}' ;;
      esac ;;
    *) echo "unsupported gh invocation: $*" >&2; return 1 ;;
  esac
}
`
			command := exec.CommandContext(t.Context(), "bash", "-c", stub+script)
			command.Env = append(os.Environ(), "TAG=v2.99.0", "GITHUB_REPOSITORY=test/hitkeep", "GITHUB_OUTPUT="+outputPath, "SCENARIO="+scenario)
			output, err := command.CombinedOutput()
			if (err == nil) != (scenario == "published") {
				t.Fatalf("promotion error = %v: %s", err, output)
			}
			result, _ := os.ReadFile(outputPath)
			if strings.Contains(string(result), "status=verified") != (scenario == "published") {
				t.Fatalf("promotion output = %q", result)
			}
		})
	}
}
