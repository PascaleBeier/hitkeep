package admin

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"hitkeep/internal/server/shared"
)

func (h *handler) appendAudit(r *http.Request, action, targetType, targetID, targetLabel, outcome, details string) {
	actorID := shared.GetUserIDFromContext(r)
	if actorID == uuid.Nil {
		slog.Warn("Instance audit without actor ID", "action", action)
		return
	}

	h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
		ActorID:     actorID,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		TargetLabel: targetLabel,
		Outcome:     outcome,
		Details:     details,
	})
}
