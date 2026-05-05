package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

type SearchConsoleFactInput struct {
	SiteID          uuid.UUID
	PropertyURI     string
	Date            time.Time
	Query           string
	Page            string
	Country         string
	Device          string
	Clicks          int
	Impressions     int
	CTR             float64
	Position        float64
	AggregationType string
	DataState       string
	ImportedAt      time.Time
}

func (s *Store) UpsertSearchConsoleFact(ctx context.Context, input SearchConsoleFactInput) error {
	return upsertSearchConsoleFact(ctx, s.db, input)
}

func (s *Store) UpsertSearchConsoleFacts(ctx context.Context, inputs []SearchConsoleFactInput) error {
	return s.Transact(ctx, func(tx *sql.Tx) error {
		for _, input := range inputs {
			if err := upsertSearchConsoleFact(ctx, tx, input); err != nil {
				return err
			}
		}
		return nil
	})
}

func upsertSearchConsoleFact(ctx context.Context, exec sqlExecContext, input SearchConsoleFactInput) error {
	if input.SiteID == uuid.Nil {
		return fmt.Errorf("site id is required")
	}
	propertyURI := strings.TrimSpace(input.PropertyURI)
	if propertyURI == "" {
		return fmt.Errorf("property uri is required")
	}
	if input.Date.IsZero() {
		return fmt.Errorf("date is required")
	}
	if input.ImportedAt.IsZero() {
		input.ImportedAt = time.Now().UTC()
	}
	dataState := strings.TrimSpace(input.DataState)
	if dataState == "" {
		dataState = "final"
	}

	_, err := exec.ExecContext(ctx, `
		INSERT INTO search_console_facts (
			site_id, property_uri, date, query, page, country, device,
			clicks, impressions, ctr, position, aggregation_type, data_state, imported_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (site_id, property_uri, date, query, page, country, device, aggregation_type, data_state) DO UPDATE SET
			clicks = excluded.clicks,
			impressions = excluded.impressions,
			ctr = excluded.ctr,
			position = excluded.position,
			imported_at = excluded.imported_at
	`,
		input.SiteID,
		propertyURI,
		input.Date.UTC(),
		strings.TrimSpace(input.Query),
		strings.TrimSpace(input.Page),
		normalizeSearchConsoleCountry(input.Country),
		strings.ToUpper(strings.TrimSpace(input.Device)),
		input.Clicks,
		input.Impressions,
		input.CTR,
		input.Position,
		strings.TrimSpace(input.AggregationType),
		dataState,
		input.ImportedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("upsert Search Console fact: %w", err)
	}
	return nil
}

func (s *Store) GetSearchConsoleOverview(ctx context.Context, params api.SearchConsoleReportParams) (api.SearchConsoleOverview, error) {
	where, args := searchConsoleReportWhere(params)
	var overview api.SearchConsoleOverview
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(clicks), 0) AS clicks,
			COALESCE(SUM(impressions), 0) AS impressions,
			CASE WHEN COALESCE(SUM(impressions), 0) > 0
				THEN COALESCE(SUM(clicks), 0)::DOUBLE / SUM(impressions)
				ELSE 0
			END AS ctr,
			CASE WHEN COALESCE(SUM(impressions), 0) > 0
				THEN SUM(position * impressions) / SUM(impressions)
				ELSE 0
			END AS average_position
		FROM search_console_facts
		`+where,
		args...,
	).Scan(&overview.Clicks, &overview.Impressions, &overview.CTR, &overview.AveragePosition); err != nil {
		return api.SearchConsoleOverview{}, fmt.Errorf("get Search Console overview: %w", err)
	}
	overview.DataSource = "google_search_console"
	return overview, nil
}

func (s *Store) GetSearchConsoleSeries(ctx context.Context, params api.SearchConsoleReportParams) (api.SearchConsoleSeriesResponse, error) {
	where, args := searchConsoleReportWhere(params)
	// #nosec G202 -- where is built from fixed clauses and parameterized values only.
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			date,
			COALESCE(SUM(clicks), 0) AS clicks,
			COALESCE(SUM(impressions), 0) AS impressions,
			CASE WHEN COALESCE(SUM(impressions), 0) > 0
				THEN COALESCE(SUM(clicks), 0)::DOUBLE / SUM(impressions)
				ELSE 0
			END AS ctr,
			CASE WHEN COALESCE(SUM(impressions), 0) > 0
				THEN SUM(position * impressions) / SUM(impressions)
				ELSE 0
			END AS average_position
		FROM search_console_facts
		`+where+`
		GROUP BY date
		ORDER BY date ASC
	`, args...)
	if err != nil {
		return api.SearchConsoleSeriesResponse{}, fmt.Errorf("query Search Console series: %w", err)
	}
	defer rows.Close()

	response := api.SearchConsoleSeriesResponse{DataSource: "google_search_console", Series: []api.SearchConsoleMetricPoint{}}
	for rows.Next() {
		var date time.Time
		var point api.SearchConsoleMetricPoint
		if err := rows.Scan(&date, &point.Clicks, &point.Impressions, &point.CTR, &point.AveragePosition); err != nil {
			return api.SearchConsoleSeriesResponse{}, fmt.Errorf("scan Search Console series: %w", err)
		}
		point.Date = api.NewDateOnly(date)
		response.Series = append(response.Series, point)
	}
	if err := rows.Err(); err != nil {
		return api.SearchConsoleSeriesResponse{}, fmt.Errorf("read Search Console series: %w", err)
	}
	return response, nil
}

func (s *Store) GetSearchConsoleDimension(ctx context.Context, params api.SearchConsoleReportParams, dimension string) (api.SearchConsoleDimensionResponse, error) {
	column, ok := searchConsoleDimensionColumn(dimension)
	if !ok {
		return api.SearchConsoleDimensionResponse{}, fmt.Errorf("unsupported Search Console dimension %q", dimension)
	}
	if params.Limit <= 0 {
		params.Limit = 10
	}
	if params.Limit > 100 {
		params.Limit = 100
	}
	where, args := searchConsoleReportWhere(params)
	args = append(args, params.Limit)
	// #nosec G202 -- column is selected through searchConsoleDimensionColumn; where values stay parameterized.
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			`+column+` AS value,
			COALESCE(SUM(clicks), 0) AS clicks,
			COALESCE(SUM(impressions), 0) AS impressions,
			CASE WHEN COALESCE(SUM(impressions), 0) > 0
				THEN COALESCE(SUM(clicks), 0)::DOUBLE / SUM(impressions)
				ELSE 0
			END AS ctr,
			CASE WHEN COALESCE(SUM(impressions), 0) > 0
				THEN SUM(position * impressions) / SUM(impressions)
				ELSE 0
			END AS average_position
		FROM search_console_facts
		`+where+`
		GROUP BY `+column+`
		ORDER BY clicks DESC, impressions DESC, value ASC
		LIMIT ?
	`, args...)
	if err != nil {
		return api.SearchConsoleDimensionResponse{}, fmt.Errorf("query Search Console %s rows: %w", dimension, err)
	}
	defer rows.Close()

	response := api.SearchConsoleDimensionResponse{DataSource: "google_search_console", Dimension: dimension, Rows: []api.SearchConsoleDimensionRow{}}
	for rows.Next() {
		var row api.SearchConsoleDimensionRow
		if err := rows.Scan(&row.Value, &row.Clicks, &row.Impressions, &row.CTR, &row.AveragePosition); err != nil {
			return api.SearchConsoleDimensionResponse{}, fmt.Errorf("scan Search Console %s rows: %w", dimension, err)
		}
		response.Rows = append(response.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return api.SearchConsoleDimensionResponse{}, fmt.Errorf("read Search Console %s rows: %w", dimension, err)
	}
	return response, nil
}

func searchConsoleDimensionColumn(dimension string) (string, bool) {
	switch dimension {
	case "query":
		return "query", true
	case "page":
		return "page", true
	case "country":
		return "country", true
	case "device":
		return "device", true
	default:
		return "", false
	}
}

func searchConsoleReportWhere(params api.SearchConsoleReportParams) (string, []any) {
	clauses := []string{
		"site_id = ?",
		"property_uri = ?",
		"data_state = 'final'",
		"date BETWEEN ? AND ?",
	}
	args := []any{
		params.SiteID,
		strings.TrimSpace(params.PropertyURI),
		searchConsoleReportDate(params.Start),
		searchConsoleReportDate(params.End),
	}
	if page := strings.TrimSpace(params.Page); page != "" {
		clauses = append(clauses, "page = ?")
		args = append(args, page)
	} else if path := strings.TrimSpace(params.Path); path != "" {
		clauses = append(clauses, searchConsolePagePathExpr()+" = ?")
		args = append(args, normalizeSearchConsolePagePath(path))
	}
	if country := strings.TrimSpace(params.Country); country != "" {
		clauses = append(clauses, "country = ?")
		args = append(args, normalizeSearchConsoleCountry(country))
	}
	if device := strings.TrimSpace(params.Device); device != "" {
		clauses = append(clauses, "device = ?")
		args = append(args, strings.ToUpper(device))
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func searchConsoleReportDate(value time.Time) time.Time {
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func searchConsolePagePathExpr() string {
	return `CASE
		WHEN page LIKE 'http://%' OR page LIKE 'https://%'
			THEN COALESCE(NULLIF(regexp_extract(page, '^https?://[^/?#]+([^?#]*)', 1), ''), '/')
		ELSE COALESCE(NULLIF(regexp_extract(page, '^([^?#]*)', 1), ''), '/')
	END`
}

func normalizeSearchConsolePagePath(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "/"
	}
	if parsed, err := url.Parse(trimmed); err == nil && parsed.IsAbs() {
		if parsed.Path == "" {
			return "/"
		}
		return parsed.Path
	}
	withoutQuery, _, _ := strings.Cut(trimmed, "?")
	withoutFragment, _, _ := strings.Cut(withoutQuery, "#")
	withoutFragment = strings.TrimSpace(withoutFragment)
	if withoutFragment == "" {
		return "/"
	}
	if !strings.HasPrefix(withoutFragment, "/") {
		return "/" + withoutFragment
	}
	return withoutFragment
}

func normalizeSearchConsoleCountry(value string) string {
	code := strings.ToUpper(strings.TrimSpace(value))
	if len(code) == 2 {
		if alpha3, ok := searchConsoleAlpha2ToAlpha3[code]; ok {
			return alpha3
		}
	}
	return code
}

var searchConsoleAlpha2ToAlpha3 = map[string]string{
	"AD": "AND", "AE": "ARE", "AF": "AFG", "AG": "ATG", "AI": "AIA", "AL": "ALB", "AM": "ARM", "AO": "AGO", "AQ": "ATA", "AR": "ARG",
	"AS": "ASM", "AT": "AUT", "AU": "AUS", "AW": "ABW", "AX": "ALA", "AZ": "AZE", "BA": "BIH", "BB": "BRB", "BD": "BGD", "BE": "BEL",
	"BF": "BFA", "BG": "BGR", "BH": "BHR", "BI": "BDI", "BJ": "BEN", "BL": "BLM", "BM": "BMU", "BN": "BRN", "BO": "BOL", "BQ": "BES",
	"BR": "BRA", "BS": "BHS", "BT": "BTN", "BV": "BVT", "BW": "BWA", "BY": "BLR", "BZ": "BLZ", "CA": "CAN", "CC": "CCK", "CD": "COD",
	"CF": "CAF", "CG": "COG", "CH": "CHE", "CI": "CIV", "CK": "COK", "CL": "CHL", "CM": "CMR", "CN": "CHN", "CO": "COL", "CR": "CRI",
	"CU": "CUB", "CV": "CPV", "CW": "CUW", "CX": "CXR", "CY": "CYP", "CZ": "CZE", "DE": "DEU", "DJ": "DJI", "DK": "DNK", "DM": "DMA",
	"DO": "DOM", "DZ": "DZA", "EC": "ECU", "EE": "EST", "EG": "EGY", "EH": "ESH", "ER": "ERI", "ES": "ESP", "ET": "ETH", "FI": "FIN",
	"FJ": "FJI", "FK": "FLK", "FM": "FSM", "FO": "FRO", "FR": "FRA", "GA": "GAB", "GB": "GBR", "GD": "GRD", "GE": "GEO", "GF": "GUF",
	"GG": "GGY", "GH": "GHA", "GI": "GIB", "GL": "GRL", "GM": "GMB", "GN": "GIN", "GP": "GLP", "GQ": "GNQ", "GR": "GRC", "GS": "SGS",
	"GT": "GTM", "GU": "GUM", "GW": "GNB", "GY": "GUY", "HK": "HKG", "HM": "HMD", "HN": "HND", "HR": "HRV", "HT": "HTI", "HU": "HUN",
	"ID": "IDN", "IE": "IRL", "IL": "ISR", "IM": "IMN", "IN": "IND", "IO": "IOT", "IQ": "IRQ", "IR": "IRN", "IS": "ISL", "IT": "ITA",
	"JE": "JEY", "JM": "JAM", "JO": "JOR", "JP": "JPN", "KE": "KEN", "KG": "KGZ", "KH": "KHM", "KI": "KIR", "KM": "COM", "KN": "KNA",
	"KP": "PRK", "KR": "KOR", "KW": "KWT", "KY": "CYM", "KZ": "KAZ", "LA": "LAO", "LB": "LBN", "LC": "LCA", "LI": "LIE", "LK": "LKA",
	"LR": "LBR", "LS": "LSO", "LT": "LTU", "LU": "LUX", "LV": "LVA", "LY": "LBY", "MA": "MAR", "MC": "MCO", "MD": "MDA", "ME": "MNE",
	"MF": "MAF", "MG": "MDG", "MH": "MHL", "MK": "MKD", "ML": "MLI", "MM": "MMR", "MN": "MNG", "MO": "MAC", "MP": "MNP", "MQ": "MTQ",
	"MR": "MRT", "MS": "MSR", "MT": "MLT", "MU": "MUS", "MV": "MDV", "MW": "MWI", "MX": "MEX", "MY": "MYS", "MZ": "MOZ", "NA": "NAM",
	"NC": "NCL", "NE": "NER", "NF": "NFK", "NG": "NGA", "NI": "NIC", "NL": "NLD", "NO": "NOR", "NP": "NPL", "NR": "NRU", "NU": "NIU",
	"NZ": "NZL", "OM": "OMN", "PA": "PAN", "PE": "PER", "PF": "PYF", "PG": "PNG", "PH": "PHL", "PK": "PAK", "PL": "POL", "PM": "SPM",
	"PN": "PCN", "PR": "PRI", "PS": "PSE", "PT": "PRT", "PW": "PLW", "PY": "PRY", "QA": "QAT", "RE": "REU", "RO": "ROU", "RS": "SRB",
	"RU": "RUS", "RW": "RWA", "SA": "SAU", "SB": "SLB", "SC": "SYC", "SD": "SDN", "SE": "SWE", "SG": "SGP", "SH": "SHN", "SI": "SVN",
	"SJ": "SJM", "SK": "SVK", "SL": "SLE", "SM": "SMR", "SN": "SEN", "SO": "SOM", "SR": "SUR", "SS": "SSD", "ST": "STP", "SV": "SLV",
	"SX": "SXM", "SY": "SYR", "SZ": "SWZ", "TC": "TCA", "TD": "TCD", "TF": "ATF", "TG": "TGO", "TH": "THA", "TJ": "TJK", "TK": "TKL",
	"TL": "TLS", "TM": "TKM", "TN": "TUN", "TO": "TON", "TR": "TUR", "TT": "TTO", "TV": "TUV", "TW": "TWN", "TZ": "TZA", "UA": "UKR",
	"UG": "UGA", "UM": "UMI", "US": "USA", "UY": "URY", "UZ": "UZB", "VA": "VAT", "VC": "VCT", "VE": "VEN", "VG": "VGB", "VI": "VIR",
	"VN": "VNM", "VU": "VUT", "WF": "WLF", "WS": "WSM", "YE": "YEM", "YT": "MYT", "ZA": "ZAF", "ZM": "ZMB", "ZW": "ZWE",
}
