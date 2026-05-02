package importables

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"hitkeep/internal/api"
)

const manifestValueLimit = 200

type PlausibleProvider struct{}

func NewPlausibleProvider() Provider {
	return &PlausibleProvider{}
}

func (p *PlausibleProvider) Descriptor() api.ImportProviderDescriptor {
	return api.ImportProviderDescriptor{
		Key:                ProviderPlausible,
		Name:               "Plausible",
		AcceptedExtensions: []string{".zip", ".csv"},
		Capabilities:       []string{"traffic_aggregates", "dimension_aggregates", "event_aggregates", "event_properties"},
	}
}

func (p *PlausibleProvider) Validate(ctx context.Context, sources SourceSet) (*api.ImportManifest, error) {
	builder := newPlausibleManifestBuilder(sources.SourceHash)
	if err := p.scan(ctx, sources, builder, nil); err != nil {
		return nil, err
	}
	return builder.build(), nil
}

func (p *PlausibleProvider) Import(ctx context.Context, sources SourceSet, sink Sink) (*api.ImportManifest, error) {
	if sink == nil {
		return nil, errors.New("import sink is required")
	}
	builder := newPlausibleManifestBuilder(sources.SourceHash)
	if err := p.scan(ctx, sources, builder, sink); err != nil {
		return nil, err
	}
	if err := sink.Flush(ctx); err != nil {
		return nil, err
	}
	return builder.build(), nil
}

func (p *PlausibleProvider) scan(ctx context.Context, sources SourceSet, builder *plausibleManifestBuilder, sink Sink) error {
	if len(sources.Files) == 0 {
		return errors.New("at least one Plausible ZIP or CSV file is required")
	}
	zipCount := 0
	csvCount := 0
	for _, source := range sources.Files {
		switch strings.ToLower(filepath.Ext(source.Name)) {
		case ".zip":
			zipCount++
		case ".csv":
			csvCount++
		default:
			builder.addIgnored(source.Name)
		}
	}
	if zipCount > 0 && (csvCount > 0 || len(sources.Files) > 1) {
		return errors.New("provide either one Plausible ZIP or one or more Plausible CSV files")
	}

	for _, source := range sources.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch strings.ToLower(filepath.Ext(source.Name)) {
		case ".zip":
			if err := p.scanZip(ctx, source, builder, sink); err != nil {
				return err
			}
		case ".csv":
			if err := p.scanCSVFile(ctx, source, builder, sink); err != nil {
				return err
			}
		}
	}

	if builder.acceptedFiles == 0 {
		return errors.New("no recognized Plausible CSV schemas found")
	}
	builder.addMissingFiles()
	return nil
}

func (p *PlausibleProvider) scanZip(ctx context.Context, source SourceFile, builder *plausibleManifestBuilder, sink Sink) error {
	reader, err := zip.OpenReader(source.Path)
	if err != nil {
		return fmt.Errorf("open zip %s: %w", source.Name, err)
	}
	defer reader.Close()

	for _, entry := range reader.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(entry.Name)
		if strings.ToLower(filepath.Ext(name)) != ".csv" {
			builder.addIgnored(entry.Name)
			continue
		}
		entryReader, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %s: %w", entry.Name, err)
		}
		err = p.scanCSV(ctx, name, entryReader, builder, sink)
		closeErr := entryReader.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return fmt.Errorf("close zip entry %s: %w", entry.Name, closeErr)
		}
	}
	return nil
}

func (p *PlausibleProvider) scanCSVFile(ctx context.Context, source SourceFile, builder *plausibleManifestBuilder, sink Sink) error {
	file, err := os.Open(source.Path)
	if err != nil {
		return fmt.Errorf("open csv %s: %w", source.Name, err)
	}
	defer file.Close()
	return p.scanCSV(ctx, source.Name, file, builder, sink)
}

func (p *PlausibleProvider) scanCSV(ctx context.Context, filename string, input io.Reader, builder *plausibleManifestBuilder, sink Sink) error {
	reader := csv.NewReader(input)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			builder.warn("empty_file", "CSV file is empty.", filename)
			return nil
		}
		return fmt.Errorf("read header from %s: %w", filename, err)
	}
	dataset, headerIndex, ok := plausibleDatasetForHeader(header)
	if !ok {
		builder.addIgnored(filename)
		builder.warn("unsupported_schema", "CSV header does not match a supported Plausible export schema.", filename)
		return nil
	}

	builder.addAcceptedFile(filename, dataset)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read %s row %d: %w", filename, reader.InputOffset(), err)
		}
		if len(record) != len(header) {
			builder.skip(dataset, "row has unexpected column count", filename)
			continue
		}
		if err := p.handleRecord(ctx, dataset, filename, record, headerIndex, builder, sink); err != nil {
			return err
		}
	}
	return nil
}

func (p *PlausibleProvider) handleRecord(ctx context.Context, dataset plausibleDataset, filename string, record []string, headerIndex plausibleHeaderIndex, builder *plausibleManifestBuilder, sink Sink) error {
	builder.scanned(dataset)
	row, err := parsePlausibleRecord(dataset, filename, record, headerIndex)
	if err != nil {
		builder.skip(dataset, err.Error(), filename)
		return nil
	}

	builder.accept(dataset, row)
	if sink == nil {
		return nil
	}

	return writePlausibleRow(ctx, sink, dataset.key, row)
}

func writePlausibleRow(ctx context.Context, sink Sink, datasetKey string, row plausibleParsedRow) error {
	switch datasetKey {
	case "visitors":
		return sink.PutTraffic(ctx, row.traffic)
	case "custom_events":
		return writePlausibleEventRow(ctx, sink, row.event)
	case "custom_props":
		return sink.PutEventProperty(ctx, row.eventProperty)
	default:
		return sink.PutDimension(ctx, row.dimension)
	}
}

func writePlausibleEventRow(ctx context.Context, sink Sink, row EventRow) error {
	if err := sink.PutEvent(ctx, row); err != nil {
		return err
	}
	if dimSink, ok := sink.(EventDimensionSink); ok {
		if err := writePlausibleEventDimension(ctx, dimSink, row, "url", row.LinkURL); err != nil {
			return err
		}
		if err := writePlausibleEventDimension(ctx, dimSink, row, "path", row.Path); err != nil {
			return err
		}
	}
	if err := writePlausibleEventProperty(ctx, sink, row, "url", row.LinkURL); err != nil {
		return err
	}
	return writePlausibleEventProperty(ctx, sink, row, "path", row.Path)
}

func writePlausibleEventDimension(ctx context.Context, sink EventDimensionSink, row EventRow, dimension string, name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	return sink.PutEventDimension(ctx, EventDimensionRow{
		Date:       row.Date,
		EventName:  row.EventName,
		Dimension:  dimension,
		Name:       name,
		Visitors:   row.Visitors,
		Events:     row.Events,
		SourceFile: row.SourceFile,
	})
}

func writePlausibleEventProperty(ctx context.Context, sink Sink, row EventRow, key string, value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return sink.PutEventProperty(ctx, EventPropertyRow{
		Date:          row.Date,
		EventName:     row.EventName,
		PropertyKey:   key,
		PropertyValue: value,
		Visitors:      row.Visitors,
		Events:        row.Events,
		SourceFile:    row.SourceFile,
	})
}

type plausibleDataset struct {
	key  string
	name string
}

var plausibleDatasets = []plausibleDataset{
	{key: "visitors", name: "Visitors"},
	{key: "pages", name: "Pages"},
	{key: "entry_pages", name: "Entry pages"},
	{key: "exit_pages", name: "Exit pages"},
	{key: "sources", name: "Sources"},
	{key: "locations", name: "Locations"},
	{key: "devices", name: "Devices"},
	{key: "browsers", name: "Browsers"},
	{key: "operating_systems", name: "Operating systems"},
	{key: "custom_events", name: "Custom events"},
	{key: "custom_props", name: "Custom properties"},
}

var plausibleHeaders = map[string][][]string{
	"visitors": headerVariants("date", "visitors", "pageviews", "bounces", "visits", "visit_duration"),
	"pages": {
		{"date", "hostname", "page", "visits", "visitors", "pageviews", "total_scroll_depth", "total_scroll_depth_visits", "total_time_on_page", "total_time_on_page_visits"},
		{"date", "hostname", "page", "visits", "visitors", "pageviews", "total_scroll_depth", "total_scroll_depth_visits"},
	},
	"entry_pages":       headerVariants("date", "entry_page", "visitors", "entrances", "visit_duration", "bounces", "pageviews"),
	"exit_pages":        headerVariants("date", "exit_page", "visitors", "visit_duration", "exits", "bounces", "pageviews"),
	"sources":           headerVariants("date", "source", "referrer", "utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term", "pageviews", "visitors", "visits", "visit_duration", "bounces"),
	"locations":         headerVariants("date", "country", "region", "city", "visitors", "visits", "visit_duration", "bounces", "pageviews"),
	"devices":           headerVariants("date", "device", "visitors", "visits", "visit_duration", "bounces", "pageviews"),
	"browsers":          headerVariants("date", "browser", "browser_version", "visitors", "visits", "visit_duration", "bounces", "pageviews"),
	"operating_systems": headerVariants("date", "operating_system", "operating_system_version", "visitors", "visits", "visit_duration", "bounces", "pageviews"),
	"custom_events":     headerVariants("date", "name", "link_url", "path", "visitors", "events"),
	"custom_props":      headerVariants("date", "property", "value", "visitors", "events"),
}

func plausibleDatasetForHeader(header []string) (plausibleDataset, plausibleHeaderIndex, bool) {
	for _, dataset := range plausibleDatasets {
		headerIndex, err := validateCSVHeader(header, plausibleHeaders[dataset.key])
		if err == nil {
			return dataset, headerIndex, true
		}
	}
	return plausibleDataset{}, nil, false
}

type plausibleHeaderIndex map[string]int

func headerVariants(columns ...string) [][]string {
	return [][]string{columns}
}

func validateCSVHeader(got []string, variants [][]string) (plausibleHeaderIndex, error) {
	for _, want := range variants {
		if len(got) != len(want) {
			continue
		}
		matches := true
		for i := range want {
			if strings.TrimSpace(got[i]) != want[i] {
				matches = false
				break
			}
		}
		if matches {
			index := make(plausibleHeaderIndex, len(want))
			for i, name := range want {
				index[name] = i
			}
			return index, nil
		}
	}
	if len(variants) == 0 {
		return nil, fmt.Errorf("unknown Plausible dataset header")
	}
	want := variants[0]
	if len(got) != len(want) {
		if len(variants) == 1 {
			return nil, fmt.Errorf("expected %d columns, got %d", len(want), len(got))
		}
		return nil, fmt.Errorf("expected one of %d Plausible header variants, got %d columns", len(variants), len(got))
	}
	for i := range want {
		if strings.TrimSpace(got[i]) != want[i] {
			return nil, fmt.Errorf("expected column %d to be %q, got %q", i+1, want[i], got[i])
		}
	}
	return nil, fmt.Errorf("header did not match any known Plausible variant")
}

func (h plausibleHeaderIndex) value(record []string, name string) string {
	index, ok := h[name]
	if !ok || index < 0 || index >= len(record) {
		return ""
	}
	return record[index]
}

func (h plausibleHeaderIndex) int(record []string, name string) (int64, error) {
	value, ok := h[name]
	if !ok || value < 0 || value >= len(record) {
		return 0, fmt.Errorf("missing %s", name)
	}
	return parseNonNegativeInt(record[value])
}

type plausibleParsedRow struct {
	date          time.Time
	visitors      int64
	visits        int64
	pageviews     int64
	bounces       int64
	visitDuration int64
	events        int64
	traffic       TrafficRow
	dimension     DimensionRow
	event         EventRow
	eventProperty EventPropertyRow
}

func parsePlausibleRecord(dataset plausibleDataset, filename string, record []string, headerIndex plausibleHeaderIndex) (plausibleParsedRow, error) {
	date, err := time.Parse(time.DateOnly, strings.TrimSpace(headerIndex.value(record, "date")))
	if err != nil {
		return plausibleParsedRow{}, fmt.Errorf("invalid date")
	}

	row := plausibleParsedRow{date: date}
	switch dataset.key {
	case "visitors":
		return parsePlausibleTraffic(row, filename, record, headerIndex)
	case "pages":
		return parsePlausiblePageDimension(row, filename, record, headerIndex)
	case "entry_pages":
		return parsePlausibleEntryPageDimension(row, filename, record, headerIndex)
	case "exit_pages":
		return parsePlausibleExitPageDimension(row, filename, record, headerIndex)
	case "sources":
		return parsePlausibleSourceDimension(row, filename, record, headerIndex)
	case "locations":
		return parsePlausibleLocationDimension(row, filename, record, headerIndex)
	case "devices":
		return parsePlausibleStandardDimension(row, filename, record, headerIndex, "device", "device", "missing device", "")
	case "browsers":
		return parsePlausibleStandardDimension(row, filename, record, headerIndex, "browser", "browser", "missing browser", "browser_version")
	case "operating_systems":
		return parsePlausibleStandardDimension(row, filename, record, headerIndex, "operating_system", "operating_system", "missing operating system", "operating_system_version")
	case "custom_events":
		return parsePlausibleEvent(row, filename, record, headerIndex)
	case "custom_props":
		return parsePlausibleEventProperty(row, filename, record, headerIndex)
	}
	return row, nil
}

func parsePlausibleTraffic(row plausibleParsedRow, filename string, record []string, headerIndex plausibleHeaderIndex) (plausibleParsedRow, error) {
	visitors, pageviews, bounces, visits, duration, err := intsByName(record, headerIndex, "visitors", "pageviews", "bounces", "visits", "visit_duration")
	if err != nil {
		return row, err
	}
	row.visitors = visitors
	row.visits = visits
	row.pageviews = pageviews
	row.bounces = bounces
	row.visitDuration = duration
	row.traffic = TrafficRow{Date: row.date, Visitors: visitors, Visits: visits, Pageviews: pageviews, Bounces: bounces, VisitDuration: duration, SourceFile: filename}
	return row, nil
}

func parsePlausiblePageDimension(row plausibleParsedRow, filename string, record []string, headerIndex plausibleHeaderIndex) (plausibleParsedRow, error) {
	visits, visitors, pageviews, err := ints3ByName(record, headerIndex, "visits", "visitors", "pageviews")
	if err != nil {
		return row, err
	}
	name := requiredValue(headerIndex.value(record, "page"))
	if name == "" {
		return row, fmt.Errorf("missing page")
	}
	row.visitors = visitors
	row.visits = visits
	row.pageviews = pageviews
	row.dimension = DimensionRow{Date: row.date, Dimension: "page", Name: name, Detail: strings.TrimSpace(headerIndex.value(record, "hostname")), Visitors: visitors, Visits: visits, Pageviews: pageviews, SourceFile: filename}
	return row, nil
}

func parsePlausibleEntryPageDimension(row plausibleParsedRow, filename string, record []string, headerIndex plausibleHeaderIndex) (plausibleParsedRow, error) {
	visitors, entrances, duration, bounces, pageviews, err := intsByName(record, headerIndex, "visitors", "entrances", "visit_duration", "bounces", "pageviews")
	if err != nil {
		return row, err
	}
	name := requiredValue(headerIndex.value(record, "entry_page"))
	if name == "" {
		return row, fmt.Errorf("missing entry page")
	}
	row.visitors = visitors
	row.pageviews = pageviews
	row.dimension = DimensionRow{Date: row.date, Dimension: "entry_page", Name: name, Visitors: visitors, Entrances: entrances, VisitDuration: duration, Bounces: bounces, Pageviews: pageviews, SourceFile: filename}
	return row, nil
}

func parsePlausibleExitPageDimension(row plausibleParsedRow, filename string, record []string, headerIndex plausibleHeaderIndex) (plausibleParsedRow, error) {
	visitors, duration, exits, bounces, pageviews, err := intsByName(record, headerIndex, "visitors", "visit_duration", "exits", "bounces", "pageviews")
	if err != nil {
		return row, err
	}
	name := requiredValue(headerIndex.value(record, "exit_page"))
	if name == "" {
		return row, fmt.Errorf("missing exit page")
	}
	row.visitors = visitors
	row.pageviews = pageviews
	row.dimension = DimensionRow{Date: row.date, Dimension: "exit_page", Name: name, Visitors: visitors, Exits: exits, VisitDuration: duration, Bounces: bounces, Pageviews: pageviews, SourceFile: filename}
	return row, nil
}

func parsePlausibleSourceDimension(row plausibleParsedRow, filename string, record []string, headerIndex plausibleHeaderIndex) (plausibleParsedRow, error) {
	pageviews, visitors, visits, duration, bounces, err := intsByName(record, headerIndex, "pageviews", "visitors", "visits", "visit_duration", "bounces")
	if err != nil {
		return row, err
	}
	name := requiredValue(headerIndex.value(record, "source"))
	if name == "" {
		name = "(Direct)"
	}
	detail := strings.Join(nonEmpty(
		headerIndex.value(record, "referrer"),
		headerIndex.value(record, "utm_source"),
		headerIndex.value(record, "utm_medium"),
		headerIndex.value(record, "utm_campaign"),
		headerIndex.value(record, "utm_content"),
		headerIndex.value(record, "utm_term"),
	), " | ")
	row.visitors = visitors
	row.visits = visits
	row.pageviews = pageviews
	row.dimension = DimensionRow{Date: row.date, Dimension: "source", Name: name, Detail: detail, Visitors: visitors, Visits: visits, Pageviews: pageviews, VisitDuration: duration, Bounces: bounces, SourceFile: filename}
	return row, nil
}

func parsePlausibleLocationDimension(row plausibleParsedRow, filename string, record []string, headerIndex plausibleHeaderIndex) (plausibleParsedRow, error) {
	visitors, visits, duration, bounces, pageviews, err := intsByName(record, headerIndex, "visitors", "visits", "visit_duration", "bounces", "pageviews")
	if err != nil {
		return row, err
	}
	name := requiredValue(headerIndex.value(record, "country"))
	if name == "" {
		name = "(Unknown)"
	}
	detail := strings.Join(nonEmpty(headerIndex.value(record, "region"), headerIndex.value(record, "city")), " | ")
	row.visitors = visitors
	row.visits = visits
	row.pageviews = pageviews
	row.dimension = DimensionRow{Date: row.date, Dimension: "country", Name: name, Detail: detail, Visitors: visitors, Visits: visits, Pageviews: pageviews, VisitDuration: duration, Bounces: bounces, SourceFile: filename}
	return row, nil
}

func parsePlausibleStandardDimension(row plausibleParsedRow, filename string, record []string, headerIndex plausibleHeaderIndex, column string, dimension string, missing string, detailColumn string) (plausibleParsedRow, error) {
	visitors, visits, duration, bounces, pageviews, err := intsByName(record, headerIndex, "visitors", "visits", "visit_duration", "bounces", "pageviews")
	if err != nil {
		return row, err
	}
	name := requiredValue(headerIndex.value(record, column))
	if name == "" {
		return row, errors.New(missing)
	}
	detail := ""
	if detailColumn != "" {
		detail = strings.TrimSpace(headerIndex.value(record, detailColumn))
	}
	row.visitors = visitors
	row.visits = visits
	row.pageviews = pageviews
	row.dimension = DimensionRow{Date: row.date, Dimension: dimension, Name: name, Detail: detail, Visitors: visitors, Visits: visits, Pageviews: pageviews, VisitDuration: duration, Bounces: bounces, SourceFile: filename}
	return row, nil
}

func parsePlausibleEvent(row plausibleParsedRow, filename string, record []string, headerIndex plausibleHeaderIndex) (plausibleParsedRow, error) {
	visitors, events, err := ints2ByName(record, headerIndex, "visitors", "events")
	if err != nil {
		return row, err
	}
	name := normalizePlausibleEventName(requiredValue(headerIndex.value(record, "name")))
	if name == "" {
		return row, fmt.Errorf("missing event name")
	}
	row.visitors = visitors
	row.events = events
	row.event = EventRow{Date: row.date, EventName: name, LinkURL: strings.TrimSpace(headerIndex.value(record, "link_url")), Path: strings.TrimSpace(headerIndex.value(record, "path")), Visitors: visitors, Events: events, SourceFile: filename}
	return row, nil
}

func parsePlausibleEventProperty(row plausibleParsedRow, filename string, record []string, headerIndex plausibleHeaderIndex) (plausibleParsedRow, error) {
	visitors, events, err := ints2ByName(record, headerIndex, "visitors", "events")
	if err != nil {
		return row, err
	}
	key := requiredValue(headerIndex.value(record, "property"))
	value := requiredValue(headerIndex.value(record, "value"))
	if key == "" || value == "" {
		return row, fmt.Errorf("missing property key or value")
	}
	row.visitors = visitors
	row.events = events
	row.eventProperty = EventPropertyRow{Date: row.date, PropertyKey: key, PropertyValue: value, Visitors: visitors, Events: events, SourceFile: filename}
	return row, nil
}

func intsByName(record []string, headerIndex plausibleHeaderIndex, names ...string) (int64, int64, int64, int64, int64, error) {
	if len(names) != 5 {
		return 0, 0, 0, 0, 0, fmt.Errorf("invalid integer parser setup")
	}
	values := make([]int64, 5)
	for i, name := range names {
		value, err := headerIndex.int(record, name)
		if err != nil {
			return 0, 0, 0, 0, 0, err
		}
		values[i] = value
	}
	return values[0], values[1], values[2], values[3], values[4], nil
}

func ints2ByName(record []string, headerIndex plausibleHeaderIndex, a, b string) (int64, int64, error) {
	first, err := headerIndex.int(record, a)
	if err != nil {
		return 0, 0, err
	}
	second, err := headerIndex.int(record, b)
	if err != nil {
		return 0, 0, err
	}
	return first, second, nil
}

func ints3ByName(record []string, headerIndex plausibleHeaderIndex, a, b, c string) (int64, int64, int64, error) {
	first, err := headerIndex.int(record, a)
	if err != nil {
		return 0, 0, 0, err
	}
	second, err := headerIndex.int(record, b)
	if err != nil {
		return 0, 0, 0, err
	}
	third, err := headerIndex.int(record, c)
	if err != nil {
		return 0, 0, 0, err
	}
	return first, second, third, nil
}

func parseNonNegativeInt(raw string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid non-negative integer")
	}
	return value, nil
}

func requiredValue(raw string) string {
	return strings.TrimSpace(raw)
}

func normalizePlausibleEventName(name string) string {
	switch strings.TrimSpace(name) {
	case "Outbound Link: Click":
		return "outbound_click"
	case "File Download":
		return "file_download"
	case "Form: Submission":
		return "form_submit"
	default:
		return strings.TrimSpace(name)
	}
}

func nonEmpty(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func dateOverlapKey(date time.Time) string {
	return date.Format(time.DateOnly)
}

func eventOverlapKey(date time.Time, eventName string) string {
	return dateOverlapKey(date) + "\x00" + strings.TrimSpace(eventName)
}

type plausibleManifestBuilder struct {
	manifest      *api.ImportManifest
	datasets      map[string]*api.ImportDatasetSummary
	seenDatasets  map[string]bool
	acceptedFiles int
	eventNames    *limitedStringSet
	propKeys      *limitedStringSet
	unattrKeys    *limitedStringSet
	warnings      map[string]bool
}

var plausibleEventDimensionsAvailable = []string{"date", "event_name", "url", "path"}

var plausibleEventDimensionsUnavailable = []string{
	"source",
	"referrer",
	"utm_source",
	"utm_medium",
	"utm_campaign",
	"utm_content",
	"utm_term",
	"browser",
	"browser_version",
	"os",
	"os_version",
	"device",
	"country",
	"region",
	"city",
	"language",
}

func newPlausibleManifestBuilder(sourceHash string) *plausibleManifestBuilder {
	return &plausibleManifestBuilder{
		manifest: &api.ImportManifest{
			Provider:     ProviderPlausible,
			SourceHash:   sourceHash,
			Files:        []string{},
			IgnoredFiles: []string{},
			MissingFiles: []string{},
			Datasets:     []api.ImportDatasetSummary{},
			EventDimensionCoverage: api.ImportEventDimensionCoverage{
				Available:   append([]string(nil), plausibleEventDimensionsAvailable...),
				Unavailable: append([]string(nil), plausibleEventDimensionsUnavailable...),
				Reason:      "Plausible CSV exports are daily aggregate rows and do not prove event-level audience relationships.",
			},
			Overlap: api.ImportOverlapSummary{
				Policy: "skip_native_day",
			},
			Warnings: []api.ImportWarning{},
			OverlapCandidates: &api.ImportOverlapCandidates{
				TrafficByDate:            map[string]api.ImportOverlapMetrics{},
				DimensionByDate:          map[string]api.ImportOverlapMetrics{},
				EventByDateName:          map[string]api.ImportOverlapMetrics{},
				EventDimensionByDateName: map[string]api.ImportOverlapMetrics{},
				EventPropertyByDateName:  map[string]api.ImportOverlapMetrics{},
			},
		},
		datasets:     make(map[string]*api.ImportDatasetSummary),
		seenDatasets: make(map[string]bool),
		eventNames:   newLimitedStringSet(manifestValueLimit),
		propKeys:     newLimitedStringSet(manifestValueLimit),
		unattrKeys:   newLimitedStringSet(manifestValueLimit),
		warnings:     make(map[string]bool),
	}
}

func (b *plausibleManifestBuilder) addAcceptedFile(filename string, dataset plausibleDataset) {
	b.acceptedFiles++
	b.manifest.Files = append(b.manifest.Files, filename)
	b.seenDatasets[dataset.key] = true
	summary := b.summary(dataset)
	summary.Files = append(summary.Files, filename)
}

func (b *plausibleManifestBuilder) addIgnored(filename string) {
	b.manifest.IgnoredFiles = append(b.manifest.IgnoredFiles, filename)
}

func (b *plausibleManifestBuilder) addMissingFiles() {
	for _, dataset := range plausibleDatasets {
		if !b.seenDatasets[dataset.key] {
			b.manifest.MissingFiles = append(b.manifest.MissingFiles, "imported_"+dataset.key)
		}
	}
}

func (b *plausibleManifestBuilder) scanned(dataset plausibleDataset) {
	b.manifest.RowsScanned++
	b.summary(dataset).RowsScanned++
	if dataset.key == "custom_events" {
		b.manifest.EventCoverage.RowsScanned++
	}
}

func (b *plausibleManifestBuilder) accept(dataset plausibleDataset, row plausibleParsedRow) {
	b.manifest.RowsAccepted++
	summary := b.summary(dataset)
	summary.RowsAccepted++
	summary.Visitors += row.visitors
	summary.Visits += row.visits
	summary.Pageviews += row.pageviews
	summary.Events += row.events
	b.updateDateRange(row.date)

	switch dataset.key {
	case "visitors":
		b.addOverlapCandidate(b.manifest.OverlapCandidates.TrafficByDate, dateOverlapKey(row.date), api.ImportOverlapMetrics{Rows: 1, Pageviews: row.pageviews})
	case "custom_events":
		b.manifest.EventCoverage.RowsAccepted++
		b.manifest.EventCoverage.Events += row.events
		b.manifest.EventCoverage.Visitors += row.visitors
		b.eventNames.add(row.event.EventName)
		eventKey := eventOverlapKey(row.date, row.event.EventName)
		b.addOverlapCandidate(b.manifest.OverlapCandidates.EventByDateName, eventKey, api.ImportOverlapMetrics{Rows: 1, Events: row.events})
		if row.event.LinkURL != "" {
			b.propKeys.add("url")
			b.manifest.EventPropertyCoverage.AttributedRows++
			b.manifest.EventPropertyCoverage.AttributedEvents += row.events
			b.manifest.EventPropertyCoverage.AttributedVisitors += row.visitors
			b.addOverlapCandidate(b.manifest.OverlapCandidates.EventDimensionByDateName, eventKey, api.ImportOverlapMetrics{Rows: 1, Events: row.events})
			b.addOverlapCandidate(b.manifest.OverlapCandidates.EventPropertyByDateName, eventKey, api.ImportOverlapMetrics{Rows: 1, Events: row.events})
		}
		if row.event.Path != "" {
			b.propKeys.add("path")
			b.manifest.EventPropertyCoverage.AttributedRows++
			b.manifest.EventPropertyCoverage.AttributedEvents += row.events
			b.manifest.EventPropertyCoverage.AttributedVisitors += row.visitors
			b.addOverlapCandidate(b.manifest.OverlapCandidates.EventDimensionByDateName, eventKey, api.ImportOverlapMetrics{Rows: 1, Events: row.events})
			b.addOverlapCandidate(b.manifest.OverlapCandidates.EventPropertyByDateName, eventKey, api.ImportOverlapMetrics{Rows: 1, Events: row.events})
		}
	case "custom_props":
		b.manifest.EventPropertyCoverage.UnattributedRows++
		b.manifest.EventPropertyCoverage.UnattributedEvents += row.events
		b.manifest.EventPropertyCoverage.UnattributedVisitors += row.visitors
		b.unattrKeys.add(row.eventProperty.PropertyKey)
		b.addOverlapCandidate(b.manifest.OverlapCandidates.EventPropertyByDateName, eventOverlapKey(row.date, ""), api.ImportOverlapMetrics{Rows: 1, Events: row.events})
		b.warn("unattributed_custom_props", "Plausible custom property rows do not contain an event name, so they are imported as unattributed aggregate properties.", row.eventProperty.SourceFile)
	default:
		b.addOverlapCandidate(b.manifest.OverlapCandidates.DimensionByDate, dateOverlapKey(row.date), api.ImportOverlapMetrics{Rows: 1})
	}
}

func (b *plausibleManifestBuilder) addOverlapCandidate(target map[string]api.ImportOverlapMetrics, key string, add api.ImportOverlapMetrics) {
	if target == nil || key == "" {
		return
	}
	current := target[key]
	current.Rows += add.Rows
	current.Pageviews += add.Pageviews
	current.Events += add.Events
	target[key] = current
}

func (b *plausibleManifestBuilder) skip(dataset plausibleDataset, reason string, filename string) {
	b.manifest.RowsSkipped++
	b.summary(dataset).RowsSkipped++
	b.warn("row_skipped", "A row was skipped: "+reason, filename)
}

func (b *plausibleManifestBuilder) warn(code, message, filename string) {
	key := code + "|" + message + "|" + filename
	if b.warnings[key] {
		return
	}
	b.warnings[key] = true
	b.manifest.Warnings = append(b.manifest.Warnings, api.ImportWarning{Code: code, Message: message, File: filename})
}

func (b *plausibleManifestBuilder) summary(dataset plausibleDataset) *api.ImportDatasetSummary {
	if summary, ok := b.datasets[dataset.key]; ok {
		return summary
	}
	summary := &api.ImportDatasetSummary{Key: dataset.key, Name: dataset.name, Files: []string{}}
	b.datasets[dataset.key] = summary
	return summary
}

func (b *plausibleManifestBuilder) updateDateRange(date time.Time) {
	if b.manifest.DateStart == nil || date.Before(*b.manifest.DateStart) {
		d := date
		b.manifest.DateStart = &d
	}
	if b.manifest.DateEnd == nil || date.After(*b.manifest.DateEnd) {
		d := date
		b.manifest.DateEnd = &d
	}
}

func (b *plausibleManifestBuilder) build() *api.ImportManifest {
	keys := make([]string, 0, len(b.datasets))
	for key := range b.datasets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	b.manifest.Datasets = b.manifest.Datasets[:0]
	for _, key := range keys {
		b.manifest.Datasets = append(b.manifest.Datasets, *b.datasets[key])
	}

	sort.Strings(b.manifest.Files)
	sort.Strings(b.manifest.IgnoredFiles)
	sort.Strings(b.manifest.MissingFiles)
	b.manifest.EventCoverage.EventNames = b.eventNames.values()
	b.manifest.EventCoverage.PropertyKeys = b.propKeys.values()
	b.manifest.EventPropertyCoverage.AttributedPropertyKeys = b.propKeys.values()
	b.manifest.EventPropertyCoverage.UnattributedPropertyKeys = b.unattrKeys.values()
	if b.manifest.EventPropertyCoverage.UnattributedRows > 0 {
		b.manifest.EventPropertyCoverage.UnattributedRelationship = "date_property_value"
		b.manifest.EventPropertyCoverage.UnavailableRelationshipMsg = "Plausible custom_props rows do not contain event names, so they are preserved but are not queryable under a selected event."
	}
	if b.eventNames.truncated {
		b.warn("event_names_truncated", "The validation manifest lists the first event names only; all rows are still validated and importable.", "")
	}
	if b.propKeys.truncated || b.unattrKeys.truncated {
		b.warn("property_keys_truncated", "The validation manifest lists the first property keys only; all rows are still validated and importable.", "")
	}
	return b.manifest
}

type limitedStringSet struct {
	limit     int
	valuesMap map[string]struct{}
	truncated bool
}

func newLimitedStringSet(limit int) *limitedStringSet {
	return &limitedStringSet{limit: limit, valuesMap: map[string]struct{}{}}
}

func (s *limitedStringSet) add(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if _, ok := s.valuesMap[value]; ok {
		return
	}
	if len(s.valuesMap) >= s.limit {
		s.truncated = true
		return
	}
	s.valuesMap[value] = struct{}{}
}

func (s *limitedStringSet) values() []string {
	values := make([]string, 0, len(s.valuesMap))
	for value := range s.valuesMap {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}
