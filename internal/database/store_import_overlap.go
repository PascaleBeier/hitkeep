package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

const ImportOverlapPolicySkipNativeDay = "skip_native_day"

type ImportOverlapPlan struct {
	NativeTrafficDays map[string]struct{}
	NativeEventDays   map[string]struct{}
	NativeEventKeys   map[string]struct{}
}

func (s *Store) BuildImportOverlapPlan(ctx context.Context, siteID uuid.UUID, start, end time.Time) (*ImportOverlapPlan, error) {
	plan := &ImportOverlapPlan{
		NativeTrafficDays: map[string]struct{}{},
		NativeEventDays:   map[string]struct{}{},
		NativeEventKeys:   map[string]struct{}{},
	}
	if start.IsZero() || end.IsZero() {
		return plan, nil
	}
	rangeStart := importDateStart(start)
	rangeEnd := importDateStart(end).AddDate(0, 0, 1)

	hitRows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT CAST(timestamp AS DATE) AS day
		FROM hits
		WHERE site_id = ? AND timestamp >= ? AND timestamp < ?
	`, siteID, rangeStart, rangeEnd)
	if err != nil {
		return nil, fmt.Errorf("query native hit overlap: %w", err)
	}
	defer hitRows.Close()
	for hitRows.Next() {
		var day time.Time
		if err := hitRows.Scan(&day); err != nil {
			return nil, fmt.Errorf("scan native hit overlap: %w", err)
		}
		plan.NativeTrafficDays[importDateKey(day)] = struct{}{}
	}
	if err := hitRows.Err(); err != nil {
		return nil, fmt.Errorf("read native hit overlap: %w", err)
	}

	eventRows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT CAST(timestamp AS DATE) AS day, name
		FROM events
		WHERE site_id = ? AND timestamp >= ? AND timestamp < ?
	`, siteID, rangeStart, rangeEnd)
	if err != nil {
		return nil, fmt.Errorf("query native event overlap: %w", err)
	}
	defer eventRows.Close()
	for eventRows.Next() {
		var (
			day  time.Time
			name string
		)
		if err := eventRows.Scan(&day, &name); err != nil {
			return nil, fmt.Errorf("scan native event overlap: %w", err)
		}
		dateKey := importDateKey(day)
		plan.NativeEventDays[dateKey] = struct{}{}
		plan.NativeEventKeys[importEventKeyString(dateKey, name)] = struct{}{}
	}
	if err := eventRows.Err(); err != nil {
		return nil, fmt.Errorf("read native event overlap: %w", err)
	}
	return plan, nil
}

func (s *Store) AnnotateImportManifestOverlap(ctx context.Context, siteID uuid.UUID, manifest *api.ImportManifest) (*ImportOverlapPlan, error) {
	if manifest == nil {
		return &ImportOverlapPlan{
			NativeTrafficDays: map[string]struct{}{},
			NativeEventDays:   map[string]struct{}{},
			NativeEventKeys:   map[string]struct{}{},
		}, nil
	}
	manifest.Overlap.Policy = ImportOverlapPolicySkipNativeDay
	if manifest.DateStart == nil || manifest.DateEnd == nil {
		return &ImportOverlapPlan{
			NativeTrafficDays: map[string]struct{}{},
			NativeEventDays:   map[string]struct{}{},
			NativeEventKeys:   map[string]struct{}{},
		}, nil
	}
	plan, err := s.BuildImportOverlapPlan(ctx, siteID, *manifest.DateStart, *manifest.DateEnd)
	if err != nil {
		return nil, err
	}
	plan.Annotate(manifest)
	return plan, nil
}

func (p *ImportOverlapPlan) SkipTrafficDate(date time.Time) bool {
	if p == nil {
		return false
	}
	_, ok := p.NativeTrafficDays[importDateKey(date)]
	return ok
}

func (p *ImportOverlapPlan) SkipEvent(date time.Time, eventName string) bool {
	if p == nil {
		return false
	}
	dateKey := importDateKey(date)
	eventName = strings.TrimSpace(eventName)
	if eventName == "" {
		_, ok := p.NativeEventDays[dateKey]
		return ok
	}
	_, ok := p.NativeEventKeys[importEventKeyString(dateKey, eventName)]
	return ok
}

func (p *ImportOverlapPlan) Annotate(manifest *api.ImportManifest) {
	if manifest == nil {
		return
	}
	manifest.Overlap.Policy = ImportOverlapPolicySkipNativeDay
	if p == nil {
		return
	}
	manifest.Overlap.NativeTrafficDays = len(p.NativeTrafficDays)
	manifest.Overlap.NativeEventDays = len(p.NativeEventDays)
	manifest.Overlap.NativeEventKeys = len(p.NativeEventKeys)
	manifest.Overlap.EstimatedSkippedRows = 0
	manifest.Overlap.EstimatedSkippedPageviews = 0
	manifest.Overlap.EstimatedSkippedEvents = 0

	candidates := manifest.OverlapCandidates
	if candidates == nil {
		return
	}
	for dateKey, metrics := range candidates.TrafficByDate {
		if _, ok := p.NativeTrafficDays[dateKey]; ok {
			manifest.Overlap.EstimatedSkippedRows += metrics.Rows
			manifest.Overlap.EstimatedSkippedPageviews += metrics.Pageviews
		}
	}
	for dateKey, metrics := range candidates.DimensionByDate {
		if _, ok := p.NativeTrafficDays[dateKey]; ok {
			manifest.Overlap.EstimatedSkippedRows += metrics.Rows
		}
	}
	for key, metrics := range candidates.EventByDateName {
		if _, ok := p.NativeEventKeys[key]; ok {
			manifest.Overlap.EstimatedSkippedRows += metrics.Rows
			manifest.Overlap.EstimatedSkippedEvents += metrics.Events
		}
	}
	for key, metrics := range candidates.EventDimensionByDateName {
		if _, ok := p.NativeEventKeys[key]; ok {
			manifest.Overlap.EstimatedSkippedRows += metrics.Rows
		}
	}
	for key, metrics := range candidates.EventPropertyByDateName {
		dateKey, eventName := splitImportEventKey(key)
		_, eventOverlap := p.NativeEventKeys[key]
		_, dayOverlap := p.NativeEventDays[dateKey]
		if eventOverlap || (eventName == "" && dayOverlap) {
			manifest.Overlap.EstimatedSkippedRows += metrics.Rows
		}
	}
}

func importDateStart(date time.Time) time.Time {
	year, month, day := date.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func importDateKey(date time.Time) string {
	return importDateStart(date).Format(time.DateOnly)
}

func importEventKeyString(dateKey string, eventName string) string {
	return dateKey + "\x00" + strings.TrimSpace(eventName)
}

func splitImportEventKey(key string) (string, string) {
	dateKey, eventName, ok := strings.Cut(key, "\x00")
	if !ok {
		return key, ""
	}
	return dateKey, eventName
}
