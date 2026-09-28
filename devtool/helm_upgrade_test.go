package devtool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"hitkeep/jsonapi"
)

func TestHelmTemplateRendersImmutableReferencesForLegacyAndCandidateCharts(t *testing.T) {
	if os.Getenv("HITKEEP_HELM_TEMPLATE_CONTRACT") != "1" {
		t.Skip("set HITKEEP_HELM_TEMPLATE_CONTRACT=1 to render the immutable supported-floor chart fixture")
	}
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Fatal(err)
	}
	const repository = "registry.example/hitkeep"
	const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	manifestBytes, err := os.ReadFile(filepath.Join("..", "tests", "fixtures", "release-fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SupportedUpgradeFloor string `json:"supported_upgrade_floor"`
		Fixtures              []struct {
			PreviousVersion string `json:"previous_version"`
			PreviousChart   string `json:"previous_chart"`
		} `json:"fixtures"`
	}
	if err := jsonapi.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SupportedUpgradeFloor == "" {
		t.Fatal("release fixture has no supported upgrade floor")
	}
	previousChart := ""
	for _, fixture := range manifest.Fixtures {
		if fixture.PreviousVersion == manifest.SupportedUpgradeFloor {
			if previousChart != "" && previousChart != fixture.PreviousChart {
				t.Fatalf("release fixture has conflicting charts for supported floor %q", manifest.SupportedUpgradeFloor)
			}
			previousChart = fixture.PreviousChart
		}
	}
	if !strings.HasPrefix(previousChart, "oci://") {
		t.Fatalf("release fixture has invalid chart for supported floor: %q", previousChart)
	}
	want := `image: "` + repository + `@sha256:` + digest + `"`

	legacyDir := t.TempDir()
	if output, err := exec.Command(helm, "pull", previousChart, "--version", manifest.SupportedUpgradeFloor, "--destination", legacyDir).CombinedOutput(); err != nil {
		t.Fatalf("pull supported-floor chart: %v\n%s", err, output)
	}
	renders := []struct {
		chart  string
		values []string
	}{
		{
			chart: filepath.Join(legacyDir, "hitkeep-"+manifest.SupportedUpgradeFloor+".tgz"),
			values: []string{
				"--set-string", "image.repository=" + repository + "@sha256",
				"--set-string", "image.tag=" + digest,
			},
		},
		{
			chart: filepath.Join("..", "charts", "hitkeep"),
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
		if strings.Contains(rendered, repository+":"+manifest.SupportedUpgradeFloor+":") || strings.Contains(rendered, repository+"@sha256@") {
			t.Fatalf("rendered image for %s is malformed\n%s", render.chart, rendered)
		}
	}
}
