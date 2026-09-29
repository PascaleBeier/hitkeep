package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
)

var ErrAnnotationNotFound = errors.New("annotation not found")

const annotationSelect = `
	SELECT id, site_id, starts_at, ends_at, body, created_at
	FROM site_annotations`

// ListAnnotations returns the site's notes that overlap [start, end], oldest
// first. A range note counts when any part of it falls inside the window.
func (s *Store) ListAnnotations(ctx context.Context, siteID uuid.UUID, start, end time.Time) ([]api.Annotation, error) {
	rows, err := s.db.QueryContext(ctx, annotationSelect+`
		WHERE site_id = ? AND starts_at <= ? AND COALESCE(ends_at, starts_at) >= ?
		ORDER BY starts_at, id`, siteID, end, start)
	if err != nil {
		return nil, fmt.Errorf("failed to list annotations: %w", err)
	}
	defer rows.Close()

	annotations := make([]api.Annotation, 0)
	for rows.Next() {
		var a api.Annotation
		if err := rows.Scan(&a.ID, &a.SiteID, &a.StartsAt, &a.EndsAt, &a.Body, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan annotation: %w", err)
		}
		annotations = append(annotations, normalizeAnnotationTimes(a))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate annotations: %w", err)
	}
	return annotations, nil
}

func (s *Store) CreateAnnotation(ctx context.Context, siteID uuid.UUID, input api.AnnotationInput, createdBy uuid.UUID) (*api.Annotation, error) {
	a := api.Annotation{
		ID:        uuid.New(),
		SiteID:    siteID,
		StartsAt:  input.StartsAt,
		EndsAt:    input.EndsAt,
		Body:      strings.TrimSpace(input.Body),
		CreatedAt: time.Now().UTC(),
	}
	a = normalizeAnnotationTimes(a)
	if err := s.Exec(ctx, `
		INSERT INTO site_annotations (id, site_id, starts_at, ends_at, body, created_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.SiteID, a.StartsAt, a.EndsAt, a.Body, a.CreatedAt, nullableUUID(createdBy),
	); err != nil {
		return nil, fmt.Errorf("failed to create annotation: %w", err)
	}
	return &a, nil
}

func (s *Store) UpdateAnnotation(ctx context.Context, siteID, id uuid.UUID, input api.AnnotationInput) (*api.Annotation, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE site_annotations SET starts_at = ?, ends_at = ?, body = ?
		WHERE id = ? AND site_id = ?`,
		input.StartsAt.UTC(), utcPtr(input.EndsAt), strings.TrimSpace(input.Body), id, siteID)
	if err != nil {
		return nil, fmt.Errorf("failed to update annotation: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return nil, fmt.Errorf("failed to determine updated annotation rows: %w", err)
	} else if n == 0 {
		return nil, ErrAnnotationNotFound
	}

	var a api.Annotation
	err = s.db.QueryRowContext(ctx, annotationSelect+` WHERE id = ? AND site_id = ?`, id, siteID).
		Scan(&a.ID, &a.SiteID, &a.StartsAt, &a.EndsAt, &a.Body, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to read updated annotation: %w", err)
	}
	a = normalizeAnnotationTimes(a)
	return &a, nil
}

func (s *Store) DeleteAnnotation(ctx context.Context, siteID, id uuid.UUID) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM site_annotations WHERE id = ? AND site_id = ?`, id, siteID)
	if err != nil {
		return fmt.Errorf("failed to delete annotation: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to determine deleted annotation rows: %w", err)
	}
	if n == 0 {
		return ErrAnnotationNotFound
	}
	return nil
}

func normalizeAnnotationTimes(a api.Annotation) api.Annotation {
	a.StartsAt = a.StartsAt.UTC()
	a.EndsAt = utcPtr(a.EndsAt)
	a.CreatedAt = a.CreatedAt.UTC()
	return a
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}
