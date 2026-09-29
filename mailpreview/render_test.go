package mailpreview

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/Boostport/mjml-go"

	"hitkeep/mailer"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata")

// knownViolations are lint rules current templates still break. Remove a rule
// here once the templates are fixed; the list may only shrink.
var knownViolations = map[string]bool{}

func TestFixturesRenderCleanInEveryLocale(t *testing.T) {
	for _, fixture := range Catalog() {
		for _, locale := range mailer.SupportedLocales() {
			t.Run(fixture.ID+"/"+locale, func(t *testing.T) {
				t.Parallel()
				rendered, err := mailer.Render(fixture.Build(locale), mailer.RenderOptions{Now: Now, Validation: mjml.Strict})
				if err != nil {
					t.Fatalf("render: %v", err)
				}
				for _, problem := range Lint(rendered) {
					if !knownViolations[problem.Rule] {
						t.Error(problem)
					}
				}
				golden(t, filepath.Join("testdata", fixture.ID, locale+".txt"), "Subject: "+rendered.Subject+"\n\n"+rendered.Text)
			})
		}
	}
}

func TestFixturesMatchGoldenHTML(t *testing.T) {
	for _, fixture := range Catalog() {
		t.Run(fixture.ID, func(t *testing.T) {
			t.Parallel()
			rendered, err := mailer.Render(fixture.Build("en"), mailer.RenderOptions{Now: Now, Beautify: true})
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			golden(t, filepath.Join("testdata", fixture.ID, "en.html"), rendered.HTML)
		})
	}
}

func golden(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if string(want) != got {
		t.Errorf("%s differs from rendered output; review and rerun with -update", path)
	}
}
