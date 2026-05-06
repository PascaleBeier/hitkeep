package mailables

import (
	"testing"

	"hitkeep/internal/mailer"
)

func TestLocalizedFrequencyLabelsUseMailContext(t *testing.T) {
	tests := []struct {
		locale        string
		freq          string
		reportLabel   string
		digestLabel   string
		subjectLabel  string
		digestTitle   string
		digestSubject string
	}{
		{
			locale:        "de",
			freq:          "daily",
			reportLabel:   "Täglicher",
			digestLabel:   "Tägliche",
			subjectLabel:  "tägliche",
			digestTitle:   "Tägliche Analytics-Übersicht",
			digestSubject: "Deine tägliche Analytics-Übersicht - 16. April 2026",
		},
		{
			locale:        "de",
			freq:          "weekly",
			reportLabel:   "Wöchentlicher",
			digestLabel:   "Wöchentliche",
			subjectLabel:  "wöchentliche",
			digestTitle:   "Wöchentliche Analytics-Übersicht",
			digestSubject: "Deine wöchentliche Analytics-Übersicht - 16. April 2026",
		},
		{
			locale:        "de",
			freq:          "monthly",
			reportLabel:   "Monatlicher",
			digestLabel:   "Monatliche",
			subjectLabel:  "monatliche",
			digestTitle:   "Monatliche Analytics-Übersicht",
			digestSubject: "Deine monatliche Analytics-Übersicht - 16. April 2026",
		},
		{
			locale:        "fr",
			freq:          "weekly",
			reportLabel:   "hebdomadaire",
			digestLabel:   "hebdomadaire",
			subjectLabel:  "hebdomadaire",
			digestTitle:   "Résumé d'analytique hebdomadaire",
			digestSubject: "Votre résumé hebdomadaire d'analytique - 16 avril 2026",
		},
		{
			locale:        "es",
			freq:          "weekly",
			reportLabel:   "semanal",
			digestLabel:   "semanal",
			subjectLabel:  "semanal",
			digestTitle:   "Resumen de analítica semanal",
			digestSubject: "Tu resumen semanal de analítica - 16 de abril de 2026",
		},
		{
			locale:        "it",
			freq:          "weekly",
			reportLabel:   "settimanale",
			digestLabel:   "settimanale",
			subjectLabel:  "settimanale",
			digestTitle:   "Riepilogo di analitica settimanale",
			digestSubject: "Il tuo riepilogo settimanale di analitica - 16 aprile 2026",
		},
		{
			locale:        "nl",
			freq:          "weekly",
			reportLabel:   "Wekelijks",
			digestLabel:   "Wekelijks",
			subjectLabel:  "wekelijkse",
			digestTitle:   "Wekelijks Analytics-overzicht",
			digestSubject: "Je wekelijkse Analytics-overzicht - 16 april 2026",
		},
	}

	for _, tt := range tests {
		t.Run(tt.locale+"_"+tt.freq, func(t *testing.T) {
			if got := LocalizedReportFrequencyLabel(tt.locale, tt.freq); got != tt.reportLabel {
				t.Fatalf("report label = %q, want %q", got, tt.reportLabel)
			}
			if got := LocalizedDigestFrequencyLabel(tt.locale, tt.freq); got != tt.digestLabel {
				t.Fatalf("digest label = %q, want %q", got, tt.digestLabel)
			}
			if got := LocalizedDigestSubjectFrequencyLabel(tt.locale, tt.freq); got != tt.subjectLabel {
				t.Fatalf("digest subject label = %q, want %q", got, tt.subjectLabel)
			}
			if got := mailer.Translatef(tt.locale, "reports.digest_title", tt.digestLabel); got != tt.digestTitle {
				t.Fatalf("digest title = %q, want %q", got, tt.digestTitle)
			}
			digest := NewAnalyticsDigestWithSubjectLabel(tt.locale, localizedExampleDate(tt.locale), tt.digestLabel, tt.subjectLabel, "https://example.com/dashboard", "https://example.com/settings", nil)
			if got := digest.Subject(); got != tt.digestSubject {
				t.Fatalf("digest subject = %q, want %q", got, tt.digestSubject)
			}
		})
	}
}

func localizedExampleDate(locale string) string {
	switch locale {
	case "de":
		return "16. April 2026"
	case "fr":
		return "16 avril 2026"
	case "es":
		return "16 de abril de 2026"
	case "it":
		return "16 aprile 2026"
	case "nl":
		return "16 april 2026"
	default:
		return "April 16, 2026"
	}
}
