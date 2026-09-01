package localization

import (
	"fmt"
	"strings"
)

type defaultTeamNameCopy struct {
	defaultName   string
	withGivenName string
}

var defaultTeamNamesByLocale = map[string]defaultTeamNameCopy{
	"en": {
		defaultName:   "Default Tenant",
		withGivenName: "%s's Team",
	},
	"de": {
		defaultName:   "Standard-Team",
		withGivenName: "Team von %s",
	},
	"es": {
		defaultName:   "Equipo predeterminado",
		withGivenName: "Equipo de %s",
	},
	"fr": {
		defaultName:   "Équipe par défaut",
		withGivenName: "Équipe de %s",
	},
	"it": {
		defaultName:   "Team predefinito",
		withGivenName: "Team di %s",
	},
	"nl": {
		defaultName:   "Standaardteam",
		withGivenName: "Team van %s",
	},
	"pt": {
		defaultName:   "Equipe Padrão",
		withGivenName: "Equipe de %s",
	},
}

func DefaultTeamName(locale string, givenName string) string {
	labels := defaultTeamNamesByLocale[NormalizeLocale(locale)]
	givenName = strings.TrimSpace(givenName)
	if givenName == "" {
		return labels.defaultName
	}
	return fmt.Sprintf(labels.withGivenName, givenName)
}

func NormalizeLocale(locale string) string {
	normalized := strings.TrimSpace(strings.ToLower(strings.ReplaceAll(locale, "_", "-")))
	if normalized == "" || normalized == "auto" {
		return "en"
	}
	base, _, _ := strings.Cut(normalized, "-")
	if _, ok := defaultTeamNamesByLocale[base]; ok {
		return base
	}
	return "en"
}
