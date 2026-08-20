package mailer

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"

	json "hitkeep/internal/jsonapi"
)

const defaultMailLocale = "en"

//go:embed locales/*.json
var localeFS embed.FS

type monthCatalog struct {
	Full  []string `json:"full"`
	Short []string `json:"short"`
}

type translationCatalog struct {
	Messages map[string]string `json:"messages"`
	Months   monthCatalog      `json:"months"`
}

var mailCatalogs = mustLoadMailCatalogs()

func mustLoadMailCatalogs() map[string]translationCatalog {
	matches, err := fs.Glob(localeFS, "locales/*.json")
	if err != nil {
		panic(fmt.Sprintf("mailer: glob locale files: %v", err))
	}
	if len(matches) == 0 {
		panic("mailer: no locale files embedded")
	}

	catalogs := make(map[string]translationCatalog, len(matches))
	for _, match := range matches {
		data, err := localeFS.ReadFile(match)
		if err != nil {
			panic(fmt.Sprintf("mailer: read locale file %q: %v", match, err))
		}

		var catalog translationCatalog
		if err := json.Unmarshal(data, &catalog); err != nil {
			panic(fmt.Sprintf("mailer: parse locale file %q: %v", match, err))
		}

		locale := strings.TrimSuffix(path.Base(match), path.Ext(match))
		validateMonthCatalog(locale, catalog.Months)
		if len(catalog.Messages) == 0 {
			panic(fmt.Sprintf("mailer: locale %q has no messages", locale))
		}

		catalogs[locale] = catalog
	}

	validateCatalogs(catalogs)
	return catalogs
}

func validateMonthCatalog(locale string, months monthCatalog) {
	if len(months.Full) != 12 || len(months.Short) != 12 {
		panic(fmt.Sprintf("mailer: locale %q must define 12 full and 12 short month names", locale))
	}
}

func validateCatalogs(catalogs map[string]translationCatalog) {
	base, ok := catalogs[defaultMailLocale]
	if !ok {
		panic(fmt.Sprintf("mailer: default locale %q is missing", defaultMailLocale))
	}

	baseKeys := sortedMapKeys(base.Messages)
	for locale, catalog := range catalogs {
		missing := missingKeys(base.Messages, catalog.Messages)
		extra := missingKeys(catalog.Messages, base.Messages)
		if len(missing) == 0 && len(extra) == 0 {
			continue
		}

		panic(fmt.Sprintf(
			"mailer: locale %q key mismatch (missing=%v extra=%v, base_keys=%d)",
			locale,
			missing,
			extra,
			len(baseKeys),
		))
	}
}

func missingKeys(want, got map[string]string) []string {
	keys := make([]string, 0)
	for key := range want {
		if _, ok := got[key]; !ok {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

func SupportedLocales() []string {
	return sortedMapKeys(mailCatalogs)
}

func NormalizeLocale(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return defaultMailLocale
	}

	locale = strings.ReplaceAll(locale, "_", "-")
	parts := strings.Split(locale, "-")
	base := strings.ToLower(strings.TrimSpace(parts[0]))
	if _, ok := mailCatalogs[base]; ok {
		return base
	}

	return defaultMailLocale
}

func Translate(locale, key string) string {
	locale = NormalizeLocale(locale)
	if translated, ok := mailCatalogs[locale].Messages[key]; ok {
		return translated
	}
	if translated, ok := mailCatalogs[defaultMailLocale].Messages[key]; ok {
		return translated
	}
	return key
}

func Translatef(locale, key string, args ...any) string {
	return fmt.Sprintf(Translate(locale, key), args...)
}

func RoleLabel(locale, role string) string {
	return Translate(locale, "role."+strings.ToLower(strings.TrimSpace(role)))
}

func MonthName(locale string, month time.Month, abbreviated bool) string {
	locale = NormalizeLocale(locale)
	catalog, ok := mailCatalogs[locale]
	if !ok {
		catalog = mailCatalogs[defaultMailLocale]
	}

	idx := int(month) - 1
	if idx < 0 || idx >= len(catalog.Months.Full) {
		return month.String()
	}
	if abbreviated {
		return catalog.Months.Short[idx]
	}
	return catalog.Months.Full[idx]
}

func sortedMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
