package user

import (
	"errors"
	"regexp"
	"strings"

	"golang.org/x/text/language"

	"hitkeep/api"
)

const (
	defaultLocaleFallback = "en-US"
)

var localeTagPattern = regexp.MustCompile(`^[A-Za-z]{1,8}(-[A-Za-z0-9]{1,8})*$`)

func defaultPreferencesFromHeader(header string) api.UserPreferences {
	header = strings.TrimSpace(header)
	if header != "" {
		tags, _, err := language.ParseAcceptLanguage(header)
		if err == nil {
			for _, tag := range tags {
				base, _ := tag.Base()
				switch base.String() {
				case "", language.Und.String(), "mul":
					continue
				}
				return api.UserPreferences{
					DefaultLocale: base.String(),
				}
			}
		}
	}

	return api.UserPreferences{
		DefaultLocale: defaultLocaleFallback,
	}
}

func validateUserPreferences(defaultLocale string) (api.UserPreferences, error) {
	normalizedDefault := normalizeLocaleTag(defaultLocale)
	base := baseLocaleTag(normalizedDefault)
	if base == "" {
		return api.UserPreferences{}, errors.New("default locale is required")
	}
	return api.UserPreferences{
		DefaultLocale: base,
	}, nil
}

func normalizeLocaleTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	tag = strings.ReplaceAll(tag, "_", "-")
	if !localeTagPattern.MatchString(tag) {
		return ""
	}

	parts := strings.Split(tag, "-")
	for i, part := range parts {
		if part == "" {
			return ""
		}
		switch {
		case i == 0:
			parts[i] = strings.ToLower(part)
		case len(part) == 2:
			parts[i] = strings.ToUpper(part)
		case len(part) == 4:
			parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		default:
			parts[i] = strings.ToLower(part)
		}
	}
	return strings.Join(parts, "-")
}

func baseLocaleTag(tag string) string {
	if tag == "" {
		return ""
	}
	parts := strings.Split(tag, "-")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}
