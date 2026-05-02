package importables

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"hitkeep/internal/api"
)

type SimpleAnalyticsProvider struct{}

func NewSimpleAnalyticsProvider() Provider {
	return &SimpleAnalyticsProvider{}
}

func (p *SimpleAnalyticsProvider) Descriptor() api.ImportProviderDescriptor {
	return api.ImportProviderDescriptor{
		Key:                ProviderSimpleAnalytics,
		Name:               "Simple Analytics",
		AcceptedExtensions: []string{".csv"},
		Capabilities:       []string{"traffic_aggregates", "dimension_aggregates"},
	}
}

func (p *SimpleAnalyticsProvider) Validate(ctx context.Context, sources SourceSet) (*api.ImportManifest, error) {
	builder := newSimpleAnalyticsManifestBuilder(sources.SourceHash)
	if err := p.scan(ctx, sources, builder); err != nil {
		return nil, err
	}
	return builder.build(), nil
}

func (p *SimpleAnalyticsProvider) Import(ctx context.Context, sources SourceSet, sink Sink) (*api.ImportManifest, error) {
	if sink == nil {
		return nil, errors.New("import sink is required")
	}
	builder := newSimpleAnalyticsManifestBuilder(sources.SourceHash)
	if err := p.scan(ctx, sources, builder); err != nil {
		return nil, err
	}
	if err := builder.emit(ctx, sink); err != nil {
		return nil, err
	}
	if err := sink.Flush(ctx); err != nil {
		return nil, err
	}
	return builder.build(), nil
}

func (p *SimpleAnalyticsProvider) scan(ctx context.Context, sources SourceSet, builder *simpleAnalyticsManifestBuilder) error {
	if len(sources.Files) == 0 {
		return errors.New("at least one Simple Analytics CSV file is required")
	}
	for _, source := range sources.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !strings.EqualFold(filepath.Ext(source.Name), ".csv") {
			builder.addIgnored(source.Name)
			builder.warn("unsupported_file", "Simple Analytics imports currently support CSV datapoints exports.", source.Name)
			continue
		}
		if err := p.scanCSVFile(ctx, source, sources.SiteDomain, builder); err != nil {
			builder.addIgnored(source.Name)
			builder.warn("unrecognized_csv", err.Error(), source.Name)
		}
	}
	if builder.acceptedFiles == 0 {
		return errors.New("no recognized Simple Analytics datapoints CSV files found")
	}
	return nil
}

func (p *SimpleAnalyticsProvider) scanCSVFile(ctx context.Context, source SourceFile, siteDomain string, builder *simpleAnalyticsManifestBuilder) error {
	file, err := os.Open(source.Path)
	if err != nil {
		return fmt.Errorf("open CSV: %w", err)
	}
	defer file.Close()
	return p.scanCSV(ctx, source.Name, file, siteDomain, builder)
}

func (p *SimpleAnalyticsProvider) scanCSV(ctx context.Context, filename string, input io.Reader, siteDomain string, builder *simpleAnalyticsManifestBuilder) error {
	reader := csv.NewReader(input)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("read CSV header: %w", err)
	}
	headerIndex, err := validateSimpleAnalyticsHeader(header)
	if err != nil {
		return err
	}
	builder.addAcceptedFile(filename)
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			builder.skip("invalid CSV row", filename)
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		builder.scanned()
		row, err := parseSimpleAnalyticsRecord(filename, record, headerIndex, siteDomain)
		if err != nil {
			builder.skip(err.Error(), filename)
			continue
		}
		builder.accept(row)
	}
	return nil
}

type simpleAnalyticsHeaderIndex map[string]int

var simpleAnalyticsHeaders = []string{
	"added_iso",
	"country_code",
	"datapoint",
	"device_type",
	"document_referrer",
	"duration_seconds",
	"is_unique",
	"path",
	"session_id",
	"utm_source",
}

func validateSimpleAnalyticsHeader(header []string) (simpleAnalyticsHeaderIndex, error) {
	index := make(simpleAnalyticsHeaderIndex, len(header))
	for i, column := range header {
		column = strings.TrimPrefix(strings.TrimSpace(column), "\ufeff")
		index[column] = i
	}
	missing := []string{}
	for _, column := range simpleAnalyticsHeaders {
		if _, ok := index[column]; !ok {
			missing = append(missing, column)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing Simple Analytics columns: %s", strings.Join(missing, ", "))
	}
	return index, nil
}

func (h simpleAnalyticsHeaderIndex) value(record []string, name string) string {
	idx, ok := h[name]
	if !ok || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}

type simpleAnalyticsParsedRow struct {
	date          time.Time
	sourceFile    string
	path          string
	source        string
	device        string
	country       string
	browser       string
	browserDetail string
	language      string
	utmSource     string
	visitors      int64
	visits        int64
	pageviews     int64
	visitDuration int64
}

func parseSimpleAnalyticsRecord(filename string, record []string, headerIndex simpleAnalyticsHeaderIndex, sourceDomain string) (simpleAnalyticsParsedRow, error) {
	datapoint := strings.ToLower(headerIndex.value(record, "datapoint"))
	if datapoint != "pageview" {
		return simpleAnalyticsParsedRow{}, fmt.Errorf("unsupported datapoint %q", datapoint)
	}
	added := headerIndex.value(record, "added_iso")
	timestamp, err := time.Parse(time.RFC3339Nano, added)
	if err != nil {
		return simpleAnalyticsParsedRow{}, fmt.Errorf("invalid added_iso")
	}
	path := headerIndex.value(record, "path")
	if path == "" {
		return simpleAnalyticsParsedRow{}, fmt.Errorf("missing path")
	}
	isUnique, err := parseSimpleAnalyticsBool(headerIndex.value(record, "is_unique"))
	if err != nil {
		return simpleAnalyticsParsedRow{}, fmt.Errorf("invalid is_unique")
	}
	duration := int64(0)
	if raw := headerIndex.value(record, "duration_seconds"); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil || parsed < 0 {
			return simpleAnalyticsParsedRow{}, fmt.Errorf("invalid duration_seconds")
		}
		duration = int64(parsed)
	}

	visitors := int64(0)
	if isUnique {
		visitors = 1
	}
	language := strings.ToLower(headerIndex.value(record, "lang_language"))
	language, _, _ = strings.Cut(language, "-")
	language, _, _ = strings.Cut(language, "_")

	return simpleAnalyticsParsedRow{
		date:          simpleAnalyticsDateStart(timestamp),
		sourceFile:    filename,
		path:          path,
		source:        normalizeSimpleAnalyticsReferrer(headerIndex.value(record, "document_referrer"), sourceDomain),
		device:        normalizeSimpleAnalyticsDevice(headerIndex.value(record, "device_type")),
		country:       normalizeSimpleAnalyticsCountry(headerIndex.value(record, "country_code")),
		browser:       headerIndex.value(record, "browser_name"),
		browserDetail: strings.TrimSpace(headerIndex.value(record, "browser_version")),
		language:      language,
		utmSource:     headerIndex.value(record, "utm_source"),
		visitors:      visitors,
		visits:        visitors,
		pageviews:     1,
		visitDuration: duration,
	}, nil
}

func parseSimpleAnalyticsBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool")
	}
}

func normalizeSimpleAnalyticsReferrer(referrer string, sourceDomain string) string {
	referrer = strings.TrimSpace(referrer)
	if referrer == "" {
		return "(Direct)"
	}
	if parsed, err := url.Parse(referrer); err == nil && parsed.Hostname() != "" {
		host := strings.ToLower(strings.TrimPrefix(parsed.Hostname(), "www."))
		sourceDomain = strings.ToLower(strings.TrimPrefix(sourceDomain, "www."))
		if sourceDomain != "" && (host == sourceDomain || strings.HasSuffix(host, "."+sourceDomain)) {
			return ""
		}
		return host
	}
	return referrer
}

func normalizeSimpleAnalyticsDevice(device string) string {
	switch strings.ToLower(strings.TrimSpace(device)) {
	case "desktop":
		return "Desktop"
	case "mobile":
		return "Mobile"
	case "tablet":
		return "Tablet"
	case "":
		return "(Unknown)"
	default:
		return strings.TrimSpace(device)
	}
}

func normalizeSimpleAnalyticsCountry(country string) string {
	country = strings.ToUpper(strings.TrimSpace(country))
	if country == "" {
		return "(Unknown)"
	}
	return country
}

func simpleAnalyticsDateStart(date time.Time) time.Time {
	year, month, day := date.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

type simpleAnalyticsDailyAgg struct {
	date          time.Time
	visitors      int64
	visits        int64
	pageviews     int64
	visitDuration int64
	sourceFile    string
}

type simpleAnalyticsDimensionKey struct {
	date      string
	dimension string
	name      string
	detail    string
}

type simpleAnalyticsDimensionAgg struct {
	date          time.Time
	dimension     string
	name          string
	detail        string
	visitors      int64
	visits        int64
	pageviews     int64
	visitDuration int64
	sourceFile    string
}

type simpleAnalyticsManifestBuilder struct {
	manifest      *api.ImportManifest
	dataset       *api.ImportDatasetSummary
	acceptedFiles int
	daily         map[string]*simpleAnalyticsDailyAgg
	dimensions    map[simpleAnalyticsDimensionKey]*simpleAnalyticsDimensionAgg
	warnings      map[string]bool
}

func newSimpleAnalyticsManifestBuilder(sourceHash string) *simpleAnalyticsManifestBuilder {
	return &simpleAnalyticsManifestBuilder{
		manifest: &api.ImportManifest{
			Provider:     ProviderSimpleAnalytics,
			SourceHash:   sourceHash,
			Files:        []string{},
			IgnoredFiles: []string{},
			MissingFiles: []string{},
			Datasets:     []api.ImportDatasetSummary{},
			EventDimensionCoverage: api.ImportEventDimensionCoverage{
				Available:   []string{},
				Unavailable: []string{},
				Reason:      "Simple Analytics datapoints exports contain pageviews, not custom event rows.",
			},
			Overlap:  api.ImportOverlapSummary{Policy: "skip_native_day"},
			Warnings: []api.ImportWarning{},
			OverlapCandidates: &api.ImportOverlapCandidates{
				TrafficByDate:            map[string]api.ImportOverlapMetrics{},
				DimensionByDate:          map[string]api.ImportOverlapMetrics{},
				EventByDateName:          map[string]api.ImportOverlapMetrics{},
				EventDimensionByDateName: map[string]api.ImportOverlapMetrics{},
				EventPropertyByDateName:  map[string]api.ImportOverlapMetrics{},
			},
		},
		dataset:    &api.ImportDatasetSummary{Key: "datapoints", Name: "Datapoints", Files: []string{}},
		daily:      map[string]*simpleAnalyticsDailyAgg{},
		dimensions: map[simpleAnalyticsDimensionKey]*simpleAnalyticsDimensionAgg{},
		warnings:   map[string]bool{},
	}
}

func (b *simpleAnalyticsManifestBuilder) addAcceptedFile(filename string) {
	b.acceptedFiles++
	b.manifest.Files = append(b.manifest.Files, filename)
	b.dataset.Files = append(b.dataset.Files, filename)
}

func (b *simpleAnalyticsManifestBuilder) addIgnored(filename string) {
	b.manifest.IgnoredFiles = append(b.manifest.IgnoredFiles, filename)
}

func (b *simpleAnalyticsManifestBuilder) scanned() {
	b.manifest.RowsScanned++
	b.dataset.RowsScanned++
}

func (b *simpleAnalyticsManifestBuilder) accept(row simpleAnalyticsParsedRow) {
	b.manifest.RowsAccepted++
	b.dataset.RowsAccepted++
	b.dataset.Visitors += row.visitors
	b.dataset.Visits += row.visits
	b.dataset.Pageviews += row.pageviews
	b.updateDateRange(row.date)

	dateKey := dateOverlapKey(row.date)
	day := b.daily[dateKey]
	if day == nil {
		day = &simpleAnalyticsDailyAgg{date: row.date, sourceFile: row.sourceFile}
		b.daily[dateKey] = day
	}
	day.visitors += row.visitors
	day.visits += row.visits
	day.pageviews += row.pageviews
	day.visitDuration += row.visitDuration

	b.addDimension(row, "page", row.path, "")
	b.addDimension(row, "source", row.source, "")
	b.addDimension(row, "device", row.device, "")
	b.addDimension(row, "country", row.country, "")
	b.addDimension(row, "browser", row.browser, row.browserDetail)
	b.addDimension(row, "language", row.language, "")
	b.addDimension(row, "utm_source", row.utmSource, "")
}

func (b *simpleAnalyticsManifestBuilder) addDimension(row simpleAnalyticsParsedRow, dimension string, name string, detail string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	key := simpleAnalyticsDimensionKey{
		date:      dateOverlapKey(row.date),
		dimension: dimension,
		name:      name,
		detail:    strings.TrimSpace(detail),
	}
	agg := b.dimensions[key]
	if agg == nil {
		agg = &simpleAnalyticsDimensionAgg{date: row.date, dimension: dimension, name: name, detail: key.detail, sourceFile: row.sourceFile}
		b.dimensions[key] = agg
	}
	agg.visitors += row.visitors
	agg.visits += row.visits
	agg.pageviews += row.pageviews
	agg.visitDuration += row.visitDuration
}

func (b *simpleAnalyticsManifestBuilder) skip(reason string, filename string) {
	b.manifest.RowsSkipped++
	b.dataset.RowsSkipped++
	b.warn("row_skipped", "A row was skipped: "+reason, filename)
}

func (b *simpleAnalyticsManifestBuilder) warn(code, message, filename string) {
	key := code + "|" + message + "|" + filename
	if b.warnings[key] {
		return
	}
	b.warnings[key] = true
	b.manifest.Warnings = append(b.manifest.Warnings, api.ImportWarning{Code: code, Message: message, File: filename})
}

func (b *simpleAnalyticsManifestBuilder) updateDateRange(date time.Time) {
	if b.manifest.DateStart == nil || date.Before(*b.manifest.DateStart) {
		d := date
		b.manifest.DateStart = &d
	}
	if b.manifest.DateEnd == nil || date.After(*b.manifest.DateEnd) {
		d := date
		b.manifest.DateEnd = &d
	}
}

func (b *simpleAnalyticsManifestBuilder) emit(ctx context.Context, sink Sink) error {
	if err := b.emitTraffic(ctx, sink); err != nil {
		return err
	}
	return b.emitDimensions(ctx, sink)
}

func (b *simpleAnalyticsManifestBuilder) emitTraffic(ctx context.Context, sink Sink) error {
	dateKeys := make([]string, 0, len(b.daily))
	for key := range b.daily {
		dateKeys = append(dateKeys, key)
	}
	sort.Strings(dateKeys)
	for _, key := range dateKeys {
		day := b.daily[key]
		if err := sink.PutTraffic(ctx, TrafficRow{
			Date:          day.date,
			Visitors:      day.visitors,
			Visits:        day.visits,
			Pageviews:     day.pageviews,
			VisitDuration: day.visitDuration,
			SourceFile:    day.sourceFile,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (b *simpleAnalyticsManifestBuilder) emitDimensions(ctx context.Context, sink Sink) error {
	dimensionKeys := make([]simpleAnalyticsDimensionKey, 0, len(b.dimensions))
	for key := range b.dimensions {
		dimensionKeys = append(dimensionKeys, key)
	}
	sort.Slice(dimensionKeys, func(i, j int) bool {
		a := dimensionKeys[i]
		c := dimensionKeys[j]
		if a.date != c.date {
			return a.date < c.date
		}
		if a.dimension != c.dimension {
			return a.dimension < c.dimension
		}
		if a.name != c.name {
			return a.name < c.name
		}
		return a.detail < c.detail
	})
	for _, key := range dimensionKeys {
		dim := b.dimensions[key]
		if err := sink.PutDimension(ctx, DimensionRow{
			Date:          dim.date,
			Dimension:     dim.dimension,
			Name:          dim.name,
			Detail:        dim.detail,
			Visitors:      dim.visitors,
			Visits:        dim.visits,
			Pageviews:     dim.pageviews,
			VisitDuration: dim.visitDuration,
			SourceFile:    dim.sourceFile,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (b *simpleAnalyticsManifestBuilder) build() *api.ImportManifest {
	sort.Strings(b.manifest.Files)
	sort.Strings(b.manifest.IgnoredFiles)
	sort.Strings(b.dataset.Files)
	b.manifest.Datasets = []api.ImportDatasetSummary{*b.dataset}
	b.populateOverlapCandidates()
	if b.manifest.RowsAccepted > 0 {
		b.warn("bounce_rate_unavailable", "Simple Analytics datapoints exports do not include bounce counts; imported bounce rate is left unchanged for historical rows.", "")
	}
	return b.manifest
}

func (b *simpleAnalyticsManifestBuilder) populateOverlapCandidates() {
	candidates := &api.ImportOverlapCandidates{
		TrafficByDate:            map[string]api.ImportOverlapMetrics{},
		DimensionByDate:          map[string]api.ImportOverlapMetrics{},
		EventByDateName:          map[string]api.ImportOverlapMetrics{},
		EventDimensionByDateName: map[string]api.ImportOverlapMetrics{},
		EventPropertyByDateName:  map[string]api.ImportOverlapMetrics{},
	}
	for dateKey, day := range b.daily {
		candidates.TrafficByDate[dateKey] = api.ImportOverlapMetrics{Rows: 1, Pageviews: day.pageviews}
	}
	for key := range b.dimensions {
		current := candidates.DimensionByDate[key.date]
		current.Rows++
		candidates.DimensionByDate[key.date] = current
	}
	b.manifest.OverlapCandidates = candidates
}
