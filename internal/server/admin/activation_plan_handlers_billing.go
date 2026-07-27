//go:build billing

package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"hitkeep/internal/controlstore"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	"hitkeep/internal/server/shared"
)

// handleSetActivationTeamPlan lets an instance owner or admin manually grant
// a cloud team a plan without a real Stripe subscription (e.g. comping the
// instance owner's own account). It immediately re-syncs the team's data
// retention to the new plan's cap rather than waiting for the daily
// CloudRetentionSyncWorker reconciliation pass.
func (h *handler) handleSetActivationTeamPlan() http.HandlerFunc {
	type request struct {
		PlanCode string `json:"plan_code"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		teamID, err := uuid.Parse(r.PathValue("team_id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"status": "error", "message": "Invalid team_id",
			})
			return
		}

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"status": "error", "message": "Invalid request body",
			})
			return
		}

		switch req.PlanCode {
		case database.CloudPlanFree, database.CloudPlanPro, database.CloudPlanBusiness:
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"status": "error", "message": "Invalid plan_code",
			})
			return
		}

		account, err := h.ctx.Store.GetCloudBillingAccount(r.Context(), teamID)
		if err != nil && !errors.Is(err, controlstore.ErrCloudBillingAccountNotFound) {
			slog.Error("Failed to load cloud billing account", "team_id", teamID, "error", err)
			h.appendAudit(r, "cloud_billing.plan_override", "team", teamID.String(), "", "failure", err.Error())
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"status": "error", "message": "Failed to load billing account",
			})
			return
		}
		if account == nil {
			account = &database.CloudBillingAccount{TenantID: teamID}
		}

		previousPlanCode := account.PlanCode
		if previousPlanCode == "" {
			previousPlanCode = "none"
		}

		account.PlanCode = req.PlanCode
		account.PlanName = entitlements.CloudPlanName(req.PlanCode)
		if req.PlanCode == database.CloudPlanFree {
			account.SubscriptionStatus = database.CloudSubscriptionStatusFree
		} else {
			account.SubscriptionStatus = database.CloudSubscriptionStatusActive
		}

		teamName := ""
		if team, err := h.ctx.Store.GetTenant(r.Context(), teamID); err == nil && team != nil {
			teamName = team.Name
		}

		if err := h.ctx.Store.UpsertCloudBillingAccount(r.Context(), *account); err != nil {
			slog.Error("Failed to set cloud billing plan", "team_id", teamID, "error", err)
			h.appendAudit(r, "cloud_billing.plan_override", "team", teamID.String(), teamName, "failure", err.Error())
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"status": "error", "message": "Failed to set plan",
			})
			return
		}

		if h.ctx.TenantStores != nil {
			ent := h.ctx.Limits().TeamEntitlements(r.Context(), teamID)
			if ent != nil {
				if _, err := h.ctx.TenantStores.SyncTeamRetention(r.Context(), teamID, ent.MaxRetentionDays); err != nil {
					slog.Error("Failed to sync team retention after plan override", "team_id", teamID, "error", err)
				}
			}
		}

		actorID := shared.GetUserIDFromContext(r)
		h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
			ActorID:     actorID,
			TeamID:      teamID,
			Action:      "cloud_billing.plan_override",
			TargetType:  "team",
			TargetID:    teamID.String(),
			TargetLabel: teamName,
			Outcome:     "success",
			Details:     fmt.Sprintf("Plan changed from %s to %s", previousPlanCode, req.PlanCode),
		})

		writeJSON(w, http.StatusOK, map[string]string{
			"status":    "ok",
			"plan_code": account.PlanCode,
			"plan_name": account.PlanName,
		})
	}
}
