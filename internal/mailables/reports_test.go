package mailables

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/mailer"
	opportunitysvc "hitkeep/internal/opportunities"
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

func TestOpportunityDigestMailableUsesSafeLocalizedOpportunityData(t *testing.T) {
	preview := opportunitysvc.DigestPreview{
		Frequency:  api.ReportFrequencyWeekly,
		ShouldSend: true,
		Reason:     opportunitysvc.DigestPreviewReasonReady,
		Items: []opportunitysvc.DigestItem{
			{
				ID:          uuid.NewString(),
				SiteID:      uuid.NewString(),
				TypeKey:     "opportunities.types.checkout_conversion",
				TitleKey:    "opportunities.catalog.checkout_conversion.title",
				ActionKey:   "opportunities.catalog.checkout_conversion.action",
				DigestKey:   "opportunities.catalog.checkout_conversion.digest",
				CopyParams:  map[string]any{"conversion_rate": "42%"},
				ImpactValue: "$1,200",
				Confidence:  "high",
				Score:       91,
				Evidence: []api.OpportunityEvidence{
					{ID: "conversion_rate", LabelKey: "opportunities.evidence.checkout_conversion_rate", Value: "42%"},
				},
				CitedEvidenceIDs: []string{"conversion_rate"},
			},
		},
	}

	digest := NewOpportunityDigestWithSubjectLabel(
		"en",
		"shop.example",
		"May 4-10, 2026",
		"Weekly",
		"weekly",
		"https://app.example/sites/site-1/opportunities",
		"https://app.example/settings/email",
		preview,
	)

	if got := digest.Subject(); got != "1 weekly opportunity for shop.example - May 4-10, 2026" {
		t.Fatalf("unexpected subject %q", got)
	}
	if digest.Template() != "opportunity_digest.mjml" {
		t.Fatalf("unexpected template %q", digest.Template())
	}
	data := digest.Data().(*OpportunityDigest)
	if len(data.Items) != 1 {
		t.Fatalf("expected one opportunity digest item, got %#v", data.Items)
	}
	item := data.Items[0]
	if item.Title != "Review checkout drop-off" || item.Digest != "Checkout conversion is 42%." || item.Action == "" {
		t.Fatalf("expected localized opportunity copy, got %#v", item)
	}
	renderedItem := fmt.Sprintf("%#v", item)
	for _, forbidden := range []string{"TitleKey", "RawPrompt", "ProviderResponse", "opportunities.catalog.checkout_conversion.title"} {
		if strings.Contains(renderedItem, forbidden) {
			t.Fatalf("opportunity digest item leaked raw/internal field %q: %s", forbidden, renderedItem)
		}
	}
	if len(item.Evidence) != 1 || item.Evidence[0].Label != "Checkout conversion rate" || item.Evidence[0].Value != "42%" {
		t.Fatalf("expected localized evidence snippet, got %#v", item.Evidence)
	}
}

func TestOpportunityDigestMailableLocalizesNonCheckoutFamilies(t *testing.T) {
	preview := opportunitysvc.DigestPreview{
		Frequency:  api.ReportFrequencyWeekly,
		ShouldSend: true,
		Reason:     opportunitysvc.DigestPreviewReasonReady,
		Items: []opportunitysvc.DigestItem{
			{
				ID:         uuid.NewString(),
				SiteID:     uuid.NewString(),
				TypeKey:    "opportunities.types.traffic_quality",
				TitleKey:   "opportunities.catalog.traffic_quality.title",
				ActionKey:  "opportunities.catalog.traffic_quality.action",
				DigestKey:  "opportunities.catalog.traffic_quality.digest",
				CopyParams: map[string]any{"source": "Google", "source_hits": "1,200"},
				Score:      82,
			},
		},
	}

	digest := NewOpportunityDigestWithSubjectLabel("en", "shop.example", "May 4-10, 2026", "Weekly", "weekly", "https://app.example/opportunities", "https://app.example/settings", preview)
	item := digest.Data().(*OpportunityDigest).Items[0]

	if item.Title != "Review traffic from Google" || item.Digest != "Review traffic from Google." {
		t.Fatalf("expected localized traffic quality digest copy, got %#v", item)
	}
	if strings.Contains(fmt.Sprintf("%#v", item), "opportunities.catalog.traffic_quality") {
		t.Fatalf("traffic quality digest leaked translation key: %#v", item)
	}
}

func TestOpportunityDigestMailableUsesOnlyCitedLocalizedEvidence(t *testing.T) {
	preview := opportunitysvc.DigestPreview{
		Frequency:  api.ReportFrequencyWeekly,
		ShouldSend: true,
		Reason:     opportunitysvc.DigestPreviewReasonReady,
		Items: []opportunitysvc.DigestItem{
			{
				ID:         uuid.NewString(),
				SiteID:     uuid.NewString(),
				TypeKey:    "opportunities.types.traffic_quality",
				TitleKey:   "opportunities.catalog.traffic_quality.title",
				ActionKey:  "opportunities.catalog.traffic_quality.action",
				DigestKey:  "opportunities.catalog.traffic_quality.digest",
				CopyParams: map[string]any{"source": "Google"},
				Evidence: []api.OpportunityEvidence{
					{ID: "pageviews", LabelKey: "opportunities.evidence.pageviews", Value: "1,200"},
					{ID: "sessions", LabelKey: "opportunities.evidence.sessions", Value: "640"},
					{ID: "top_source", LabelKey: "opportunities.evidence.top_source", Value: "Google"},
					{ID: "orders", LabelKey: "opportunities.evidence.orders", Value: "12"},
				},
				CitedEvidenceIDs: []string{"pageviews", "sessions", "top_source"},
			},
		},
	}

	digest := NewOpportunityDigestWithSubjectLabel("en", "shop.example", "May 4-10, 2026", "Weekly", "weekly", "https://app.example/opportunities", "https://app.example/settings", preview)
	evidence := digest.Data().(*OpportunityDigest).Items[0].Evidence

	if len(evidence) != 3 {
		t.Fatalf("expected only cited evidence snippets, got %#v", evidence)
	}
	for _, item := range evidence {
		if strings.HasPrefix(item.Label, "opportunities.evidence.") {
			t.Fatalf("expected localized evidence label, got %#v", evidence)
		}
		if item.Label == "Orders" || item.Value == "12" {
			t.Fatalf("uncited evidence leaked into digest: %#v", evidence)
		}
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
