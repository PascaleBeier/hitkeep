package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"hitkeep/internal/importables"
)

const importedSinkBatchSize = 5000

type ImportedDataSink struct {
	store    *Store
	siteID   uuid.UUID
	importID uuid.UUID
	overlap  *ImportOverlapPlan

	tx        *sql.Tx
	traffic   *sql.Stmt
	dimension *sql.Stmt
	event     *sql.Stmt
	eventDim  *sql.Stmt
	property  *sql.Stmt
	rows      int64
	batchRows int
}

type ImportedDataSinkOptions struct {
	Overlap *ImportOverlapPlan
}

func NewImportedDataSink(ctx context.Context, store *Store, siteID, importID uuid.UUID) (*ImportedDataSink, error) {
	return NewImportedDataSinkWithOptions(ctx, store, siteID, importID, ImportedDataSinkOptions{})
}

func NewImportedDataSinkWithOptions(ctx context.Context, store *Store, siteID, importID uuid.UUID, options ImportedDataSinkOptions) (*ImportedDataSink, error) {
	sink := &ImportedDataSink{store: store, siteID: siteID, importID: importID, overlap: options.Overlap}
	if err := sink.begin(ctx); err != nil {
		return nil, err
	}
	return sink, nil
}

func (s *ImportedDataSink) Rows() int64 {
	return s.rows
}

func (s *ImportedDataSink) PutTraffic(ctx context.Context, row importables.TrafficRow) error {
	if s.overlap.SkipTrafficDate(row.Date) {
		return nil
	}
	result, err := s.traffic.ExecContext(ctx, s.siteID, s.importID, row.Date, row.Visitors, row.Visits, row.Pageviews, row.Bounces, row.VisitDuration, row.SourceFile, s.siteID, row.Date)
	if err != nil {
		return fmt.Errorf("insert imported traffic: %w", err)
	}
	return s.tick(ctx, result)
}

func (s *ImportedDataSink) PutDimension(ctx context.Context, row importables.DimensionRow) error {
	if s.overlap.SkipTrafficDate(row.Date) {
		return nil
	}
	detail := nullableString(row.Detail)
	result, err := s.dimension.ExecContext(
		ctx,
		s.siteID,
		s.importID,
		row.Date,
		row.Dimension,
		row.Name,
		detail,
		row.Visitors,
		row.Visits,
		row.Pageviews,
		row.Bounces,
		row.VisitDuration,
		row.Events,
		row.Entrances,
		row.Exits,
		row.SourceFile,
		s.siteID,
		row.Date,
		row.Dimension,
		row.Name,
		detail,
	)
	if err != nil {
		return fmt.Errorf("insert imported dimension: %w", err)
	}
	return s.tick(ctx, result)
}

func (s *ImportedDataSink) PutEvent(ctx context.Context, row importables.EventRow) error {
	if s.overlap.SkipEvent(row.Date, row.EventName) {
		return nil
	}
	path := nullableString(row.Path)
	linkURL := nullableString(row.LinkURL)
	result, err := s.event.ExecContext(ctx, s.siteID, s.importID, row.Date, row.EventName, path, linkURL, row.Visitors, row.Events, row.SourceFile, s.siteID, row.Date, row.EventName, path, linkURL)
	if err != nil {
		return fmt.Errorf("insert imported event: %w", err)
	}
	return s.tick(ctx, result)
}

func (s *ImportedDataSink) PutEventProperty(ctx context.Context, row importables.EventPropertyRow) error {
	if s.overlap.SkipEvent(row.Date, row.EventName) {
		return nil
	}
	eventName := row.EventName
	result, err := s.property.ExecContext(ctx, s.siteID, s.importID, row.Date, eventName, row.PropertyKey, row.PropertyValue, row.Visitors, row.Events, row.SourceFile, s.siteID, row.Date, eventName, row.PropertyKey, row.PropertyValue)
	if err != nil {
		return fmt.Errorf("insert imported event property: %w", err)
	}
	return s.tick(ctx, result)
}

func (s *ImportedDataSink) PutEventDimension(ctx context.Context, row importables.EventDimensionRow) error {
	if s.overlap.SkipEvent(row.Date, row.EventName) {
		return nil
	}
	detail := nullableString(row.Detail)
	result, err := s.eventDim.ExecContext(
		ctx,
		s.siteID,
		s.importID,
		row.Date,
		row.EventName,
		row.Dimension,
		row.Name,
		detail,
		row.Visitors,
		row.Events,
		row.SourceFile,
		s.siteID,
		row.Date,
		row.EventName,
		row.Dimension,
		row.Name,
		detail,
	)
	if err != nil {
		return fmt.Errorf("insert imported event dimension: %w", err)
	}
	return s.tick(ctx, result)
}

func (s *ImportedDataSink) Flush(ctx context.Context) error {
	if s.tx == nil {
		return nil
	}
	if err := s.closeStatements(); err != nil {
		return err
	}
	if err := s.tx.Commit(); err != nil {
		return fmt.Errorf("commit imported analytics batch: %w", err)
	}
	s.tx = nil
	s.batchRows = 0
	return nil
}

func (s *ImportedDataSink) Abort() {
	_ = s.closeStatements()
	if s.tx != nil {
		_ = s.tx.Rollback()
		s.tx = nil
	}
}

func (s *ImportedDataSink) tick(ctx context.Context, result sql.Result) error {
	inserted := true
	if affected, err := result.RowsAffected(); err == nil {
		inserted = affected > 0
	}
	if !inserted {
		return nil
	}
	s.rows++
	s.batchRows++
	if s.batchRows < importedSinkBatchSize {
		return nil
	}
	if err := s.Flush(ctx); err != nil {
		return err
	}
	return s.begin(ctx)
}

func (s *ImportedDataSink) begin(ctx context.Context) error {
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin imported analytics batch: %w", err)
	}
	s.tx = tx

	if s.traffic, err = tx.PrepareContext(ctx, `
		INSERT INTO imported_traffic_daily (
			site_id, import_id, date, visitors, visits, pageviews, bounces, visit_duration, source_file
		)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1
			FROM imported_traffic_daily
			WHERE site_id = ? AND date = ?
		)
	`); err != nil {
		s.Abort()
		return fmt.Errorf("prepare imported traffic insert: %w", err)
	}
	if s.dimension, err = tx.PrepareContext(ctx, `
		INSERT INTO imported_dimension_daily (
			site_id, import_id, date, dimension, name, detail, visitors, visits, pageviews,
			bounces, visit_duration, events, entrances, exits, source_file
		)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1
			FROM imported_dimension_daily
			WHERE site_id = ? AND date = ? AND dimension = ? AND name = ? AND detail IS NOT DISTINCT FROM ?
		)
	`); err != nil {
		s.Abort()
		return fmt.Errorf("prepare imported dimension insert: %w", err)
	}
	if s.event, err = tx.PrepareContext(ctx, `
		INSERT INTO imported_event_daily (
			site_id, import_id, date, event_name, path, link_url, visitors, events, source_file
		)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1
			FROM imported_event_daily
			WHERE site_id = ? AND date = ? AND event_name = ? AND path IS NOT DISTINCT FROM ? AND link_url IS NOT DISTINCT FROM ?
		)
	`); err != nil {
		s.Abort()
		return fmt.Errorf("prepare imported event insert: %w", err)
	}
	if s.property, err = tx.PrepareContext(ctx, `
		INSERT INTO imported_event_properties_daily (
			site_id, import_id, date, event_name, property_key, property_value, visitors, events, source_file
		)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1
			FROM imported_event_properties_daily
			WHERE site_id = ? AND date = ? AND event_name = ? AND property_key = ? AND property_value = ?
		)
	`); err != nil {
		s.Abort()
		return fmt.Errorf("prepare imported event property insert: %w", err)
	}
	if s.eventDim, err = tx.PrepareContext(ctx, `
		INSERT INTO imported_event_dimensions_daily (
			site_id, import_id, date, event_name, dimension, name, detail, visitors, events, source_file
		)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1
			FROM imported_event_dimensions_daily
			WHERE site_id = ? AND date = ? AND event_name = ? AND dimension = ? AND name = ? AND detail IS NOT DISTINCT FROM ?
		)
	`); err != nil {
		s.Abort()
		return fmt.Errorf("prepare imported event dimension insert: %w", err)
	}
	return nil
}

func (s *ImportedDataSink) closeStatements() error {
	var firstErr error
	for _, stmt := range []*sql.Stmt{s.traffic, s.dimension, s.event, s.property, s.eventDim} {
		if stmt == nil {
			continue
		}
		if err := stmt.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.traffic = nil
	s.dimension = nil
	s.event = nil
	s.property = nil
	s.eventDim = nil
	return firstErr
}

func (s *Store) DeleteImportedDataForImport(ctx context.Context, siteID, importID uuid.UUID) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin imported data delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var deleted int64
	for _, query := range []string{
		"DELETE FROM imported_event_properties_daily WHERE site_id = ? AND import_id = ?",
		"DELETE FROM imported_event_dimensions_daily WHERE site_id = ? AND import_id = ?",
		"DELETE FROM imported_event_daily WHERE site_id = ? AND import_id = ?",
		"DELETE FROM imported_dimension_daily WHERE site_id = ? AND import_id = ?",
		"DELETE FROM imported_traffic_daily WHERE site_id = ? AND import_id = ?",
	} {
		result, err := tx.ExecContext(ctx, query, siteID, importID)
		if err != nil {
			return 0, fmt.Errorf("delete imported data: %w", err)
		}
		if affected, err := result.RowsAffected(); err == nil {
			deleted += affected
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit imported data delete: %w", err)
	}
	return deleted, nil
}
