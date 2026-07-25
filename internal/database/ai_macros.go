package database

import (
	"context"
	"fmt"

	"hitkeep/internal/aianalytics"
)

// ensureAIClassificationMacros recreates the AI classification macros from the
// embedded AI agent master list. It runs on every store open — macros are
// persisted catalog objects, so recreating them applies a fresher list to all
// historical rows without a schema migration. The bootstrap macro migrations
// (2026_05_16 / tenant 0004) remain untouched history; this is the only
// writer going forward.
func (s *Store) ensureAIClassificationMacros(ctx context.Context) error {
	for _, statement := range aianalytics.EmbeddedAIClassificationMacroStatements() {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("ensure AI classification macros: %w", err)
		}
	}
	return nil
}
