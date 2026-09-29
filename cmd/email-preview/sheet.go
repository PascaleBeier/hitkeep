package main

import (
	htmltpl "html/template"
	"os"

	json "hitkeep/jsonapi"
	"hitkeep/mailer"
)

type sheetFixture struct {
	ID       string
	Subject  string
	Warnings []string
	Score    *float64
}

// writeContactSheet renders a self-contained review page. Emails are embedded
// as srcdoc so light and dark previews can be forced independently of the
// viewer's operating system theme.
func writeContactSheet(path, mailpitURL string, entries []entry, htmlByKey map[string]string) error {
	var fixtures []*sheetFixture
	byID := map[string]*sheetFixture{}
	for _, e := range entries {
		fixture, ok := byID[e.Fixture]
		if !ok {
			fixture = &sheetFixture{ID: e.Fixture}
			byID[e.Fixture] = fixture
			fixtures = append(fixtures, fixture)
		}
		if e.Locale == checkLocale {
			fixture.Subject, fixture.Warnings, fixture.Score = e.Subject, e.Warnings, e.Supported
		}
	}
	emails, err := json.Marshal(htmlByKey)
	if err != nil {
		return err
	}
	meta, err := json.Marshal(entries)
	if err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return sheetTemplate.Execute(file, map[string]any{
		"Fixtures": fixtures,
		"Locales":  mailer.SupportedLocales(),
		"Mailpit":  mailpitURL,
		"Emails":   htmltpl.JS(emails), // #nosec G203 -- jsonapi escapes <, >, & and JS line terminators
		"Entries":  htmltpl.JS(meta),   // #nosec G203 -- as above
	})
}

var sheetTemplate = htmltpl.Must(htmltpl.New("sheet").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>HitKeep email preview</title>
<style>
  :root { color-scheme: light dark; --bg: #f8fafc; --fg: #0f172a; --muted: #64748b; --line: #e2e8f0; --card: #fff; --accent: #33d399; }
  @media (prefers-color-scheme: dark) { :root { --bg: #0b1120; --fg: #e2e8f0; --muted: #94a3b8; --line: #1e293b; --card: #111827; } }
  body { margin: 0; font: 14px/1.5 system-ui, sans-serif; background: var(--bg); color: var(--fg); }
  header { position: sticky; top: 0; z-index: 1; display: flex; flex-wrap: wrap; gap: 16px; align-items: center; padding: 12px 16px; background: var(--card); border-bottom: 1px solid var(--line); }
  header h1 { font-size: 16px; margin: 0 16px 0 0; }
  fieldset { border: 0; padding: 0; margin: 0; display: flex; gap: 4px; align-items: center; }
  legend { float: left; color: var(--muted); margin-right: 6px; }
  button { font: inherit; padding: 4px 10px; border: 1px solid var(--line); border-radius: 6px; background: transparent; color: inherit; cursor: pointer; }
  button[aria-pressed="true"] { background: var(--accent); border-color: var(--accent); color: #18181a; }
  main { padding: 16px; display: grid; gap: 24px; }
  section { background: var(--card); border: 1px solid var(--line); border-radius: 10px; padding: 16px; }
  section h2 { font-size: 15px; margin: 0; }
  .meta { color: var(--muted); margin: 4px 0 12px; display: flex; flex-wrap: wrap; gap: 12px; }
  .meta a { color: inherit; }
  .frames { display: flex; gap: 16px; overflow-x: auto; align-items: flex-start; }
  figure { margin: 0; }
  figcaption { color: var(--muted); font-size: 12px; margin-bottom: 4px; }
  iframe { border: 1px solid var(--line); border-radius: 6px; height: 720px; background: #fff; }
  .warn { font-family: ui-monospace, monospace; font-size: 12px; }
</style>
</head>
<body>
<header>
  <h1>HitKeep email preview</h1>
  <fieldset data-control="locale"><legend>Locale</legend>{{range .Locales}}<button type="button" data-value="{{.}}">{{.}}</button>{{end}}</fieldset>
  <fieldset data-control="width"><legend>Width</legend><button type="button" data-value="600">600</button><button type="button" data-value="375">375</button><button type="button" data-value="320">320</button></fieldset>
  <fieldset data-control="theme"><legend>Theme</legend><button type="button" data-value="both">Both</button><button type="button" data-value="light">Light</button><button type="button" data-value="dark">Dark</button></fieldset>
</header>
<main>
{{range .Fixtures}}
  <section data-fixture="{{.ID}}">
    <h2>{{.ID}}</h2>
    <div class="meta">
      <span data-subject>{{.Subject}}</span>
      {{if .Score}}<span>Client support {{printf "%.1f" .Score}}%</span>{{end}}
      <a data-link="html" href="#">HTML</a><a data-link="text" href="#">Text</a>{{if $.Mailpit}}<a data-link="mailpit" href="#" target="_blank" rel="noopener">Mailpit</a>{{end}}
    </div>
    {{if .Warnings}}<div class="meta warn">{{range .Warnings}}<span>{{.}}</span>{{end}}</div>{{end}}
    <div class="frames">
      <figure data-theme="light"><figcaption>Light</figcaption><iframe title="{{.ID}} light" loading="lazy"></iframe></figure>
      <figure data-theme="dark"><figcaption>Dark (forced prefers-color-scheme)</figcaption><iframe title="{{.ID}} dark" loading="lazy"></iframe></figure>
    </div>
  </section>
{{end}}
</main>
<script>
const emails = {{.Emails}};
const entries = {{.Entries}};
const mailpit = {{.Mailpit}};
const state = { locale: "en", width: "600", theme: "both" };
try { Object.assign(state, JSON.parse(localStorage.getItem("hk-email-preview") || "{}")); } catch {}

// Force a color scheme regardless of the viewer's OS by rewriting the media query.
function variant(html, theme) {
  const query = /@media\s*\(\s*prefers-color-scheme\s*:\s*dark\s*\)/g;
  return html.replace(query, theme === "dark" ? "@media all" : "@media not all");
}

function render() {
  try { localStorage.setItem("hk-email-preview", JSON.stringify(state)); } catch {}
  for (const fieldset of document.querySelectorAll("fieldset[data-control]")) {
    for (const button of fieldset.querySelectorAll("button")) {
      button.setAttribute("aria-pressed", String(button.dataset.value === state[fieldset.dataset.control]));
    }
  }
  for (const section of document.querySelectorAll("section[data-fixture]")) {
    const key = section.dataset.fixture + "/" + state.locale;
    const entry = entries.find((e) => e.fixture === section.dataset.fixture && e.locale === state.locale);
    if (!entry) continue;
    section.querySelector("[data-subject]").textContent = entry.subject;
    section.querySelector('[data-link="html"]').href = entry.html_file;
    section.querySelector('[data-link="text"]').href = entry.text_file;
    const link = section.querySelector('[data-link="mailpit"]');
    if (link && entry.mailpit_id) link.href = mailpit + "/view/" + entry.mailpit_id;
    for (const figure of section.querySelectorAll("figure")) {
      const theme = figure.dataset.theme;
      figure.hidden = state.theme !== "both" && state.theme !== theme;
      const frame = figure.querySelector("iframe");
      frame.style.width = state.width + "px";
      const doc = variant(emails[key], theme);
      if (frame.srcdoc !== doc) frame.srcdoc = doc;
    }
  }
}

document.querySelector("header").addEventListener("click", (event) => {
  const button = event.target.closest("button[data-value]");
  if (!button) return;
  state[button.closest("fieldset").dataset.control] = button.dataset.value;
  render();
});
render();
</script>
</body>
</html>
`))
