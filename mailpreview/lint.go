package mailpreview

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/html"

	"hitkeep/mailer"
)

// GmailClipBytes is the HTML size above which Gmail clips a message.
const GmailClipBytes = 102 * 1024

// Problem is one lint finding for a rendered email.
type Problem struct {
	Rule   string
	Detail string
}

func (p Problem) String() string { return p.Rule + ": " + p.Detail }

var rawKeyPattern = func() *regexp.Regexp {
	var prefixes []string
	for _, key := range mailer.MessageKeys() {
		prefix, _, _ := strings.Cut(key, ".")
		if !slices.Contains(prefixes, prefix) {
			prefixes = append(prefixes, prefix)
		}
	}
	return regexp.MustCompile(`\b(?:` + strings.Join(prefixes, "|") + `)\.[a-z0-9_]+(?:\.[a-z0-9_]+)*\b`)
}()

// Lint checks a rendered email for content, accessibility, privacy, and
// client-safety problems. It expects production (minified) HTML.
func Lint(r mailer.Rendered) []Problem {
	var problems []Problem
	add := func(rule, format string, args ...any) {
		problems = append(problems, Problem{Rule: rule, Detail: fmt.Sprintf(format, args...)})
	}

	if strings.TrimSpace(r.Subject) == "" {
		add("empty-subject", "subject is empty")
	}
	if len(r.HTML) > GmailClipBytes {
		add("size", "HTML is %d bytes, Gmail clips above %d", len(r.HTML), GmailClipBytes)
	}

	doc, err := html.Parse(strings.NewReader(r.HTML))
	if err != nil {
		add("parse", "%v", err)
		return problems
	}

	var visible, links []string
	lang := ""
	hasPreheader := false
	for n := range doc.Descendants() {
		switch {
		case n.Type == html.TextNode && n.Parent != nil && n.Parent.Data != "style" && n.Parent.Data != "script":
			visible = append(visible, n.Data)
		case n.Type != html.ElementNode:
		case n.Data == "html":
			lang = attr(n, "lang")
		case n.Data == "img" && !hasAttr(n, "alt"):
			add("img-alt", "<img src=%q> has no alt attribute", attr(n, "src"))
		case n.Data == "a":
			href := attr(n, "href")
			if !strings.HasPrefix(href, "https://") && !strings.HasPrefix(href, "mailto:") {
				add("insecure-link", "link %q is not absolute https or mailto", href)
			} else if strings.HasPrefix(href, "https://") && !slices.Contains(links, href) {
				links = append(links, href)
			}
		case n.Data == "link" && strings.HasPrefix(attr(n, "href"), "http"):
			add("external-resource", "stylesheet %q loads from a third party", attr(n, "href"))
		case n.Data == "style" && n.FirstChild != nil && strings.Contains(n.FirstChild.Data, "@import"):
			add("external-resource", "<style> uses @import")
		case n.Data == "div" && isPreheader(n):
			hasPreheader = true
		}
	}

	if lang != r.Locale {
		add("lang", "<html lang=%q>, want %q", lang, r.Locale)
	}
	if !hasPreheader {
		add("preheader", "no hidden preheader text")
	}

	content := r.Subject + "\n" + strings.Join(visible, " ") + "\n" + r.Text
	if strings.Contains(content, "<no value>") {
		add("no-value", "template rendered <no value>")
	}
	if strings.Contains(content, "%!") {
		add("format-verb", "broken fmt verb in output")
	}
	for _, key := range rawKeyPattern.FindAllString(content, -1) {
		add("raw-key", "untranslated key %q", key)
	}
	for _, link := range links {
		if !strings.Contains(r.Text, link) {
			add("text-missing-link", "text part lacks %q", link)
		}
	}
	return problems
}

func isPreheader(n *html.Node) bool {
	style := strings.ReplaceAll(attr(n, "style"), " ", "")
	if !strings.Contains(style, "display:none") {
		return false
	}
	var text strings.Builder
	for c := range n.Descendants() {
		if c.Type == html.TextNode {
			text.WriteString(c.Data)
		}
	}
	return strings.TrimSpace(text.String()) != ""
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	return slices.ContainsFunc(n.Attr, func(a html.Attribute) bool { return a.Key == key })
}
