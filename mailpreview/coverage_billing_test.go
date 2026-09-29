//go:build billing

package mailpreview

import (
	"testing"

	"hitkeep/mailer"
)

// Billing builds include every mailable, so every template must have a fixture.
func TestCatalogCoversEveryTemplate(t *testing.T) {
	covered := map[string]bool{}
	for _, fixture := range Catalog() {
		covered[fixture.Build("en").Template()] = true
	}
	for _, name := range mailer.TemplateNames() {
		if !covered[name] {
			t.Errorf("template %s has no fixture in mailpreview", name)
		}
	}
}
