package database

import (
	"fmt"
	"strings"

	"hitkeep/internal/api"
)

func buildHitFilters(filters []api.Filter, alias string) (string, []any) {
	if len(filters) == 0 {
		return "", nil
	}

	var sql strings.Builder
	var args []any
	for _, filter := range filters {
		clause, clauseArgs := buildHitFilter(filter.Type, filter.Value, alias)
		if clause == "" {
			continue
		}
		sql.WriteString(clause)
		args = append(args, clauseArgs...)
	}

	return sql.String(), args
}

func buildHitFilter(filterType, filterValue, alias string) (string, []any) {
	if filterType == "" || filterValue == "" {
		return "", nil
	}

	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}

	switch filterType {
	case "path":
		return fmt.Sprintf(" AND %spath = ?", prefix), []any{filterValue}
	case "hostname":
		expr := fmt.Sprintf("COALESCE(NULLIF(TRIM(%shostname), ''), '(Unknown Host)')", prefix)
		return " AND " + expr + " = ?", []any{normalizeUnknownHost(filterValue)}
	case "referrer":
		normalized := filterValue
		if isDirectReferrer(filterValue) {
			normalized = "(Direct)"
		}
		expr := fmt.Sprintf("hk_referrer(%sreferrer)", prefix)
		return " AND " + expr + " = ?", []any{normalized}
	case "referrer_host":
		expr := referrerHostExpr(prefix)
		return " AND " + expr + " = ?", []any{normalizeReferrerHostFilter(filterValue)}
	case "device":
		expr := fmt.Sprintf("hk_device(%sviewport_width)", prefix)
		return " AND " + expr + " = ?", []any{filterValue}
	case "country":
		normalized := filterValue
		if isUnknownCountry(filterValue) {
			normalized = "(Unknown)"
		}
		expr := fmt.Sprintf("hk_country(%scountry_code)", prefix)
		return " AND " + expr + " = ?", []any{normalized}
	case "city":
		expr := fmt.Sprintf("COALESCE(NULLIF(TRIM(%scity), ''), '(Unknown)')", prefix)
		return " AND " + expr + " = ?", []any{normalizeUnknown(filterValue)}
	case "provider":
		expr := fmt.Sprintf("COALESCE(NULLIF(TRIM(%sprovider), ''), '(Unknown)')", prefix)
		return " AND " + expr + " = ?", []any{normalizeUnknown(filterValue)}
	case "asn":
		expr := fmt.Sprintf("hk_asn(%sasn, %sasn_org)", prefix, prefix)
		return " AND " + expr + " = ?", []any{normalizeUnknown(filterValue)}
	case "browser":
		expr := fmt.Sprintf("hk_browser(%suser_agent)", prefix)
		return " AND " + expr + " = ?", []any{filterValue}
	case "ai_bot":
		expr := fmt.Sprintf("hk_ai_bot(%suser_agent)", prefix)
		return " AND " + expr + " = ?", []any{filterValue}
	case "ai_source":
		expr := fmt.Sprintf("hk_ai_source(%sreferrer)", prefix)
		return " AND " + expr + " = ?", []any{filterValue}
	case "language":
		expr := fmt.Sprintf("CASE WHEN NULLIF(TRIM(%slanguage), '') IS NULL THEN '(Unspecified)' ELSE lower(split_part(TRIM(%slanguage), '-', 1)) END", prefix, prefix)
		normalized := strings.ToLower(strings.TrimSpace(filterValue))
		if normalized != "" && normalized != "unspecified" && normalized != "(unspecified)" {
			normalized = strings.SplitN(normalized, "-", 2)[0]
		}
		return " AND " + expr + " = ?", []any{normalizeUnspecified(normalized)}
	case "utm_campaign":
		expr := fmt.Sprintf("COALESCE(NULLIF(TRIM(%sutm_campaign), ''), '(Unspecified)')", prefix)
		return " AND " + expr + " = ?", []any{normalizeUnspecified(filterValue)}
	case "utm_content":
		expr := fmt.Sprintf("COALESCE(NULLIF(TRIM(%sutm_content), ''), '(Unspecified)')", prefix)
		return " AND " + expr + " = ?", []any{normalizeUnspecified(filterValue)}
	case "utm_medium":
		expr := fmt.Sprintf("COALESCE(NULLIF(TRIM(%sutm_medium), ''), '(Unspecified)')", prefix)
		return " AND " + expr + " = ?", []any{normalizeUnspecified(filterValue)}
	case "utm_source":
		expr := fmt.Sprintf("COALESCE(NULLIF(TRIM(%sutm_source), ''), '(Unspecified)')", prefix)
		return " AND " + expr + " = ?", []any{normalizeUnspecified(filterValue)}
	case "utm_term":
		expr := fmt.Sprintf("COALESCE(NULLIF(TRIM(%sutm_term), ''), '(Unspecified)')", prefix)
		return " AND " + expr + " = ?", []any{normalizeUnspecified(filterValue)}
	default:
		return "", nil
	}
}

func isDirectReferrer(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "direct" || normalized == "(direct)"
}

func isUnknownCountry(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "unknown" || normalized == "(unknown)"
}

func normalizeUnknown(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "unknown" || normalized == "(unknown)" {
		return "(Unknown)"
	}
	return value
}

func normalizeUnspecified(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "unspecified" || normalized == "(unspecified)" {
		return "(Unspecified)"
	}
	return value
}

func normalizeUnknownHost(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "unknown host" || normalized == "(unknown host)" {
		return "(Unknown Host)"
	}
	return value
}

func normalizeReferrerHostFilter(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "direct" || normalized == "(direct)" {
		return "(Direct)"
	}
	return strings.TrimPrefix(normalized, "www.")
}

func referrerHostExpr(prefix string) string {
	return fmt.Sprintf(`CASE
		WHEN %sreferrer IS NULL OR NULLIF(TRIM(%sreferrer), '') IS NULL THEN '(Direct)'
		WHEN lower(%sreferrer) LIKE 'http%%' THEN regexp_replace(regexp_extract(lower(%sreferrer), 'https?://([^/:?#]+)', 1), '^www\\.', '')
		ELSE regexp_replace(lower(TRIM(%sreferrer)), '^www\\.', '')
	END`, prefix, prefix, prefix, prefix, prefix)
}
