// email-preview renders every mailpreview fixture in every locale to a
// directory with a contact sheet. With -mailpit it also delivers each email to
// Mailpit and compares Mailpit's HTML compatibility check with the committed
// baseline.
//
// Usage:
//
//	go run -tags billing ./cmd/email-preview -out /tmp/email-preview
//	go run -tags billing ./cmd/email-preview -out /tmp/email-preview -mailpit http://127.0.0.1:8025 -check
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"

	json "hitkeep/jsonapi"
	"hitkeep/mailer"
	"hitkeep/mailpreview"
)

const (
	previewTag        = "email-preview"
	checkLocale       = "en"
	todoJustification = "TODO: justify"
)

type entry struct {
	Fixture   string   `json:"fixture"`
	Locale    string   `json:"locale"`
	Subject   string   `json:"subject"`
	HTMLFile  string   `json:"html_file"`
	TextFile  string   `json:"text_file"`
	MailpitID string   `json:"mailpit_id,omitempty"`
	Supported *float64 `json:"supported_percent,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

func main() {
	out := flag.String("out", "", "output directory for rendered emails and the contact sheet (required)")
	mailpitURL := flag.String("mailpit", "", "Mailpit base URL, for example http://127.0.0.1:8025")
	check := flag.Bool("check", false, "fail when Mailpit HTML check warnings differ from the baseline (requires -mailpit)")
	update := flag.Bool("update", false, "rewrite the client compatibility baseline (requires -mailpit)")
	baselinePath := flag.String("baseline", filepath.Join("mailpreview", "testdata", "client-compat.json"), "client compatibility baseline")
	flag.Parse()

	if *out == "" || ((*check || *update) && *mailpitURL == "") {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*out, *mailpitURL, *baselinePath, *check, *update); err != nil {
		fmt.Fprintln(os.Stderr, "email-preview:", err)
		os.Exit(1)
	}
}

func run(out, mailpitURL, baselinePath string, check, update bool) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}

	var mp *mailpit
	if mailpitURL != "" {
		mp = &mailpit{baseURL: strings.TrimRight(mailpitURL, "/")}
		version, err := mp.version()
		if err != nil {
			return fmt.Errorf("mailpit unavailable at %s: %w", mailpitURL, err)
		}
		fmt.Printf("Mailpit %s at %s\n", version, mp.baseURL)
		if err := mp.deleteTag(previewTag); err != nil {
			return err
		}
	}

	type job struct {
		fixture  mailpreview.Fixture
		locale   string
		rendered mailer.Rendered
	}
	var jobs []*job
	for _, fixture := range mailpreview.Catalog() {
		for _, locale := range mailer.SupportedLocales() {
			jobs = append(jobs, &job{fixture: fixture, locale: locale})
		}
	}
	// MJML rendering is CPU-bound; Mailpit delivery below stays sequential.
	var group errgroup.Group
	group.SetLimit(runtime.NumCPU())
	for _, j := range jobs {
		group.Go(func() error {
			rendered, err := mailer.Render(j.fixture.Build(j.locale), mailer.RenderOptions{Now: mailpreview.Now})
			if err != nil {
				return fmt.Errorf("render %s/%s: %w", j.fixture.ID, j.locale, err)
			}
			j.rendered = rendered
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}

	var entries []entry
	htmlByKey := map[string]string{}
	warningFixtures := map[string][]string{}
	for _, j := range jobs {
		fixture, locale, rendered := j.fixture, j.locale, j.rendered
		e := entry{
			Fixture:  fixture.ID,
			Locale:   locale,
			Subject:  rendered.Subject,
			HTMLFile: filepath.ToSlash(filepath.Join(fixture.ID, locale+".html")),
			TextFile: filepath.ToSlash(filepath.Join(fixture.ID, locale+".txt")),
		}
		if err := writeFile(filepath.Join(out, e.HTMLFile), rendered.HTML); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(out, e.TextFile), "Subject: "+rendered.Subject+"\n\n"+rendered.Text); err != nil {
			return err
		}
		htmlByKey[fixture.ID+"/"+locale] = rendered.HTML

		if mp != nil {
			id, err := mp.send(rendered, []string{previewTag, "tpl:" + fixture.ID, "loc:" + locale})
			if err != nil {
				return fmt.Errorf("send %s/%s: %w", fixture.ID, locale, err)
			}
			e.MailpitID = id
			if locale == checkLocale {
				result, err := mp.htmlCheck(id)
				if err != nil {
					return fmt.Errorf("html check %s: %w", fixture.ID, err)
				}
				supported := result.Total.Supported
				e.Supported = &supported
				for _, warning := range result.Warnings {
					e.Warnings = append(e.Warnings, warning.Slug)
					warningFixtures[warning.Slug] = append(warningFixtures[warning.Slug], fixture.ID)
				}
				sort.Strings(e.Warnings)
			}
		}
		entries = append(entries, e)
	}

	manifest, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(out, "manifest.json"), string(manifest)); err != nil {
		return err
	}
	if err := writeContactSheet(filepath.Join(out, "index.html"), mailpitURL, entries, htmlByKey); err != nil {
		return err
	}
	fmt.Printf("Rendered %d emails. Contact sheet: %s\n", len(entries), filepath.Join(out, "index.html"))
	if mp != nil {
		fmt.Printf("Mailpit inbox: %s/search?q=tag%%3A%s\n", mp.baseURL, previewTag)
	}

	switch {
	case update:
		return updateBaseline(baselinePath, warningFixtures)
	case check:
		return checkBaseline(baselinePath, warningFixtures)
	}
	return nil
}

// The baseline maps each accepted Mailpit HTML check warning slug to the
// reason it is acceptable for HitKeep's emails.
func readBaseline(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	baseline := map[string]string{}
	return baseline, json.Unmarshal(data, &baseline)
}

func checkBaseline(path string, seen map[string][]string) error {
	baseline, err := readBaseline(path)
	if err != nil {
		return err
	}
	var problems []string
	for _, slug := range sortedKeys(seen) {
		justification, ok := baseline[slug]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("new client compatibility warning %q in %s", slug, strings.Join(slices.Compact(seen[slug]), ", ")))
		case strings.TrimSpace(justification) == "" || strings.HasPrefix(justification, "TODO"):
			problems = append(problems, fmt.Sprintf("warning %q has no justification in %s", slug, path))
		}
	}
	for _, slug := range sortedKeys(baseline) {
		if _, ok := seen[slug]; !ok {
			problems = append(problems, fmt.Sprintf("warning %q no longer occurs; remove it from %s", slug, path))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("client compatibility baseline mismatch (fix the template or rerun with -update and justify):\n  %s", strings.Join(problems, "\n  "))
	}
	fmt.Printf("Client compatibility matches %s (%d accepted warnings)\n", path, len(baseline))
	return nil
}

func updateBaseline(path string, seen map[string][]string) error {
	previous, err := readBaseline(path)
	if err != nil {
		return err
	}
	next := make(map[string]string, len(seen))
	for slug := range seen {
		next[slug] = todoJustification
		if justification, ok := previous[slug]; ok {
			next[slug] = justification
		}
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	fmt.Printf("Wrote %s (%d warnings)\n", path, len(next))
	return writeFile(path, string(data)+"\n")
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
