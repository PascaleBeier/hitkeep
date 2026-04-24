package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

// Goals

func (s *Store) CreateGoal(ctx context.Context, goal *api.Goal) error {
	if goal.ID == uuid.Nil {
		goal.ID = uuid.New()
	}
	if goal.CreatedAt.IsZero() {
		goal.CreatedAt = time.Now()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO goals (id, site_id, name, type, value, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		goal.ID, goal.SiteID, goal.Name, goal.Type, goal.Value, goal.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create goal: %w", err)
	}
	return nil
}

func (s *Store) UpsertGoal(ctx context.Context, goal *api.Goal) error {
	if goal.ID == uuid.Nil {
		goal.ID = uuid.New()
	}
	if goal.CreatedAt.IsZero() {
		goal.CreatedAt = time.Now()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO goals (id, site_id, name, type, value, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			site_id = excluded.site_id,
			name = excluded.name,
			type = excluded.type,
			value = excluded.value,
			created_at = excluded.created_at`,
		goal.ID, goal.SiteID, goal.Name, goal.Type, goal.Value, goal.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert goal: %w", err)
	}
	return nil
}

func (s *Store) GetGoals(ctx context.Context, siteID uuid.UUID) ([]api.Goal, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, site_id, name, type, value, created_at
		FROM goals
		WHERE site_id = ?
		ORDER BY created_at DESC`,
		siteID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query goals: %w", err)
	}
	defer rows.Close()

	goals := make([]api.Goal, 0)
	for rows.Next() {
		var g api.Goal
		if err := rows.Scan(&g.ID, &g.SiteID, &g.Name, &g.Type, &g.Value, &g.CreatedAt); err != nil {
			return nil, err
		}
		goals = append(goals, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read goal rows: %w", err)
	}

	return goals, nil
}

func (s *Store) DeleteGoal(ctx context.Context, id uuid.UUID, siteID uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := deleteRollups(ctx, tx, siteID, id, goalRollupQueries); err != nil {
		return err
	}

	res, err := tx.ExecContext(ctx, "DELETE FROM goals WHERE id = ? AND site_id = ?", id, siteID)
	if err != nil {
		return fmt.Errorf("failed to delete goal: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("goal not found")
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// Funnels

func (s *Store) CreateFunnel(ctx context.Context, funnel *api.Funnel) error {
	if funnel.ID == uuid.Nil {
		funnel.ID = uuid.New()
	}
	if funnel.CreatedAt.IsZero() {
		funnel.CreatedAt = time.Now()
	}

	stepsJSON, err := json.Marshal(funnel.Steps)
	if err != nil {
		return fmt.Errorf("failed to marshal funnel steps: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO funnels (id, site_id, name, steps, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		funnel.ID, funnel.SiteID, funnel.Name, stepsJSON, funnel.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create funnel: %w", err)
	}
	return nil
}

func (s *Store) UpsertFunnel(ctx context.Context, funnel *api.Funnel) error {
	if funnel.ID == uuid.Nil {
		funnel.ID = uuid.New()
	}
	if funnel.CreatedAt.IsZero() {
		funnel.CreatedAt = time.Now()
	}

	stepsJSON, err := json.Marshal(funnel.Steps)
	if err != nil {
		return fmt.Errorf("failed to marshal funnel steps: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO funnels (id, site_id, name, steps, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			site_id = excluded.site_id,
			name = excluded.name,
			steps = excluded.steps,
			created_at = excluded.created_at`,
		funnel.ID, funnel.SiteID, funnel.Name, stepsJSON, funnel.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert funnel: %w", err)
	}
	return nil
}

func (s *Store) GetFunnels(ctx context.Context, siteID uuid.UUID) ([]api.Funnel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, site_id, name, CAST(steps AS VARCHAR), created_at
		FROM funnels
		WHERE site_id = ?
		ORDER BY created_at DESC`,
		siteID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query funnels: %w", err)
	}
	defer rows.Close()

	funnels := make([]api.Funnel, 0)
	for rows.Next() {
		var f api.Funnel
		var stepsJSON string
		if err := rows.Scan(&f.ID, &f.SiteID, &f.Name, &stepsJSON, &f.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(stepsJSON), &f.Steps); err != nil {
			return nil, fmt.Errorf("failed to unmarshal funnel steps: %w", err)
		}
		funnels = append(funnels, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read funnel rows: %w", err)
	}

	return funnels, nil
}

func (s *Store) DeleteFunnel(ctx context.Context, id uuid.UUID, siteID uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := deleteRollups(ctx, tx, siteID, id, funnelRollupQueries); err != nil {
		return err
	}

	res, err := tx.ExecContext(ctx, "DELETE FROM funnels WHERE id = ? AND site_id = ?", id, siteID)
	if err != nil {
		return fmt.Errorf("failed to delete funnel: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("funnel not found")
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

var goalRollupQueries = []string{
	"DELETE FROM goal_rollups_hourly WHERE site_id = ? AND goal_id = ?",
	"DELETE FROM goal_rollups_daily WHERE site_id = ? AND goal_id = ?",
	"DELETE FROM goal_rollups_monthly WHERE site_id = ? AND goal_id = ?",
}

var funnelRollupQueries = []string{
	"DELETE FROM funnel_rollups_hourly WHERE site_id = ? AND funnel_id = ?",
	"DELETE FROM funnel_rollups_daily WHERE site_id = ? AND funnel_id = ?",
	"DELETE FROM funnel_rollups_monthly WHERE site_id = ? AND funnel_id = ?",
}

func deleteRollups(ctx context.Context, tx *sql.Tx, siteID uuid.UUID, entityID uuid.UUID, queries []string) error {
	for _, query := range queries {
		if _, err := tx.ExecContext(ctx, query, siteID, entityID); err != nil {
			return fmt.Errorf("failed to delete rollups: %w", err)
		}
	}
	return nil
}
