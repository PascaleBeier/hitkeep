package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

type sqlQueryExecContext interface {
	sqlExecContext
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type OpportunityInput struct {
	ID               uuid.UUID
	TeamID           uuid.UUID
	SiteID           uuid.UUID
	Kind             string
	TypeKey          string
	TitleKey         string
	SummaryKey       string
	ActionKey        string
	DigestKey        string
	CopyParams       map[string]any
	ImpactValue      string
	ImpactLabelKey   string
	Confidence       string
	Score            int
	ScoreBreakdown   api.OpportunityScoreBreakdown
	Status           string
	RouteLabelKey    string
	RouteParams      map[string]any
	RouteIcon        string
	DetectorVersion  string
	Evidence         []api.OpportunityEvidence
	CitedEvidenceIDs []string
	AIRunID          uuid.UUID
	GeneratedAt      time.Time
}

func (s *Store) UpsertOpportunities(ctx context.Context, inputs []OpportunityInput) ([]api.Opportunity, error) {
	return s.upsertOpportunities(ctx, inputs, nil)
}

func (s *Store) UpsertOpportunitiesWithAudit(ctx context.Context, inputs []OpportunityInput, audit AuditEntryParams) ([]api.Opportunity, error) {
	return s.upsertOpportunities(ctx, inputs, &audit)
}

func (s *Store) upsertOpportunities(ctx context.Context, inputs []OpportunityInput, audit *AuditEntryParams) ([]api.Opportunity, error) {
	if len(inputs) == 0 {
		return s.upsertNoOpportunities(ctx, audit)
	}
	if err := validateOpportunityInputs(inputs); err != nil {
		return nil, err
	}
	ids, err := s.upsertOpportunitiesTx(ctx, inputs, audit)
	if err != nil {
		return nil, err
	}
	return s.getUpsertedOpportunities(ctx, inputs, ids)
}

func (s *Store) upsertNoOpportunities(ctx context.Context, audit *AuditEntryParams) ([]api.Opportunity, error) {
	if audit != nil {
		if err := s.AppendAuditEntry(ctx, *audit); err != nil {
			return nil, err
		}
	}
	return []api.Opportunity{}, nil
}

func validateOpportunityInputs(inputs []OpportunityInput) error {
	for _, input := range inputs {
		if err := validateOpportunityInput(input); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) upsertOpportunitiesTx(ctx context.Context, inputs []OpportunityInput, audit *AuditEntryParams) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(inputs))
	err := s.Transact(ctx, func(tx *sql.Tx) error {
		for _, input := range inputs {
			id, err := s.upsertOpportunityExec(ctx, tx, input)
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}
		if audit != nil {
			return appendAuditEntryTx(ctx, tx, *audit)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Store) getUpsertedOpportunities(ctx context.Context, inputs []OpportunityInput, ids []uuid.UUID) ([]api.Opportunity, error) {
	out := make([]api.Opportunity, 0, len(ids))
	for i, id := range ids {
		opportunity, err := s.GetOpportunity(ctx, inputs[i].SiteID, id)
		if err != nil {
			return nil, err
		}
		if opportunity != nil {
			out = append(out, *opportunity)
		}
	}
	return out, nil
}

func (s *Store) upsertOpportunityExec(ctx context.Context, exec sqlQueryExecContext, input OpportunityInput) (uuid.UUID, error) {
	if err := validateOpportunityInput(input); err != nil {
		return uuid.Nil, err
	}
	id := input.ID
	if id == uuid.Nil {
		var err error
		id, err = uuid.NewV7()
		if err != nil {
			return uuid.Nil, fmt.Errorf("generate opportunity id: %w", err)
		}
	} else if err := ensureOpportunityIDScope(ctx, exec, id, input.TeamID, input.SiteID); err != nil {
		return uuid.Nil, err
	}
	generatedAt := input.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	now := time.Now().UTC()
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "new"
	}
	encoded, err := encodeOpportunityJSON(input)
	if err != nil {
		return uuid.Nil, err
	}
	status, err = opportunityStatusForUpsert(ctx, exec, id, input.SiteID, status, encoded)
	if err != nil {
		return uuid.Nil, err
	}

	_, err = exec.ExecContext(ctx, `
			INSERT INTO opportunities (
			id, team_id, site_id, kind, type_key, title_key, summary_key, action_key, digest_key,
			copy_params_json, impact_value, impact_label_key, confidence, score,
			score_breakdown_json, status, route_label_key, route_params_json, route_icon, detector_version,
			evidence_json, cited_evidence_ids_json, ai_run_id, generated_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			kind = excluded.kind,
			type_key = excluded.type_key,
			title_key = excluded.title_key,
			summary_key = excluded.summary_key,
			action_key = excluded.action_key,
			digest_key = excluded.digest_key,
			copy_params_json = excluded.copy_params_json,
			impact_value = excluded.impact_value,
			impact_label_key = excluded.impact_label_key,
			confidence = excluded.confidence,
			score = excluded.score,
			score_breakdown_json = excluded.score_breakdown_json,
			status = excluded.status,
			route_label_key = excluded.route_label_key,
			route_params_json = excluded.route_params_json,
			route_icon = excluded.route_icon,
			detector_version = excluded.detector_version,
			evidence_json = excluded.evidence_json,
			cited_evidence_ids_json = excluded.cited_evidence_ids_json,
			ai_run_id = excluded.ai_run_id,
			generated_at = excluded.generated_at,
			updated_at = ?
	`, id, input.TeamID, input.SiteID, input.Kind, input.TypeKey, input.TitleKey, input.SummaryKey, input.ActionKey, input.DigestKey,
		encoded.CopyParams, input.ImpactValue, input.ImpactLabelKey, input.Confidence, input.Score,
		encoded.ScoreBreakdown, status, input.RouteLabelKey, encoded.RouteParams, input.RouteIcon, input.DetectorVersion,
		encoded.Evidence, encoded.CitedEvidenceIDs, nullableUUID(input.AIRunID), generatedAt, now, now, now)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upsert opportunity: %w", err)
	}
	return id, nil
}

func opportunityStatusForUpsert(ctx context.Context, db sqlQueryExecContext, id, siteID uuid.UUID, requested string, encoded encodedOpportunityJSON) (string, error) {
	if id == uuid.Nil || requested != "new" {
		return requested, nil
	}
	existing, err := loadExistingOpportunityMaterial(ctx, db, id, siteID)
	if errors.Is(err, sql.ErrNoRows) {
		return requested, nil
	}
	if err != nil {
		return "", err
	}
	if !isSuppressedOpportunityStatus(existing.Status) {
		return requested, nil
	}
	if sameOpportunityMaterial(existing, encoded) {
		return existing.Status, nil
	}
	return requested, nil
}

type existingOpportunityMaterial struct {
	Status           string
	Evidence         string
	CopyParams       string
	CitedEvidenceIDs string
}

func loadExistingOpportunityMaterial(ctx context.Context, db sqlQueryExecContext, id, siteID uuid.UUID) (existingOpportunityMaterial, error) {
	var out existingOpportunityMaterial
	var evidence any
	var copyParams any
	var citedEvidenceIDs any
	err := db.QueryRowContext(ctx, `
		SELECT status, evidence_json, copy_params_json, cited_evidence_ids_json
		FROM opportunities
		WHERE id = ? AND site_id = ?
	`, id, siteID).Scan(&out.Status, &evidence, &copyParams, &citedEvidenceIDs)
	if err != nil {
		return existingOpportunityMaterial{}, err
	}
	out.Evidence = jsonScanString(evidence)
	out.CopyParams = jsonScanString(copyParams)
	out.CitedEvidenceIDs = jsonScanString(citedEvidenceIDs)
	return out, nil
}

func isSuppressedOpportunityStatus(status string) bool {
	return status == "dismissed" || status == "done"
}

func sameOpportunityMaterial(existing existingOpportunityMaterial, incoming encodedOpportunityJSON) bool {
	return sameOpportunityJSON(existing.Evidence, incoming.Evidence) &&
		sameOpportunityJSON(existing.CopyParams, incoming.CopyParams) &&
		sameOpportunityJSON(existing.CitedEvidenceIDs, incoming.CitedEvidenceIDs)
}

func sameOpportunityJSON(left, right string) bool {
	leftCanonical, leftErr := canonicalOpportunityJSON(left)
	rightCanonical, rightErr := canonicalOpportunityJSON(right)
	return leftErr == nil && rightErr == nil && leftCanonical == rightCanonical
}

func canonicalOpportunityJSON(value string) (string, error) {
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return "", err
	}
	raw, err := json.Marshal(decoded)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

type encodedOpportunityJSON struct {
	Evidence         string
	CopyParams       string
	RouteParams      string
	ScoreBreakdown   string
	CitedEvidenceIDs string
}

func encodeOpportunityJSON(input OpportunityInput) (encodedOpportunityJSON, error) {
	var out encodedOpportunityJSON
	fields := []struct {
		label string
		value any
		dest  *string
	}{
		{label: "evidence", value: nonNilSlice(input.Evidence), dest: &out.Evidence},
		{label: "copy params", value: nonNilMap(input.CopyParams), dest: &out.CopyParams},
		{label: "route params", value: nonNilMap(input.RouteParams), dest: &out.RouteParams},
		{label: "score breakdown", value: input.ScoreBreakdown, dest: &out.ScoreBreakdown},
		{label: "cited evidence ids", value: nonNilSlice(input.CitedEvidenceIDs), dest: &out.CitedEvidenceIDs},
	}
	for _, field := range fields {
		raw, err := json.Marshal(field.value)
		if err != nil {
			return encodedOpportunityJSON{}, fmt.Errorf("encode opportunity %s: %w", field.label, err)
		}
		*field.dest = string(raw)
	}
	return out, nil
}

func ensureOpportunityIDScope(ctx context.Context, db sqlQueryExecContext, id, teamID, siteID uuid.UUID) error {
	var teamIDRaw sql.NullString
	var siteIDRaw sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT CAST(team_id AS VARCHAR), CAST(site_id AS VARCHAR)
		FROM opportunities
		WHERE id = ?
	`, id).Scan(&teamIDRaw, &siteIDRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check opportunity id scope: %w", err)
	}
	existingTeamID := parseNullUUIDValue(teamIDRaw)
	existingSiteID := parseNullUUIDValue(siteIDRaw)
	if existingTeamID != teamID || existingSiteID != siteID {
		return fmt.Errorf("opportunity id %s belongs to different site", id)
	}
	return nil
}

func validateOpportunityInput(input OpportunityInput) error {
	if strings.TrimSpace(input.Kind) == "revenue" {
		return fmt.Errorf("invalid opportunity kind: revenue opportunities are no longer supported")
	}
	if err := validateOpportunityCustomerParams(input); err != nil {
		return err
	}
	if err := validateOpportunityPublicValues(input); err != nil {
		return err
	}
	if err := validateOpportunityTranslationKeys(input); err != nil {
		return err
	}
	if err := validateOpportunityEvidence(input.Evidence); err != nil {
		return err
	}
	return validateOpportunityCitations(input.Evidence, input.CitedEvidenceIDs)
}

func validateOpportunityTranslationKeys(input OpportunityInput) error {
	for _, item := range []struct {
		field string
		key   string
	}{
		{field: "type_key", key: input.TypeKey},
		{field: "title_key", key: input.TitleKey},
		{field: "summary_key", key: input.SummaryKey},
		{field: "action_key", key: input.ActionKey},
		{field: "digest_key", key: input.DigestKey},
		{field: "impact_label_key", key: input.ImpactLabelKey},
		{field: "route_label_key", key: input.RouteLabelKey},
	} {
		if !isOpportunityTranslationKey(item.key) {
			return fmt.Errorf("invalid opportunity %s: must be a translation key", item.field)
		}
	}
	return nil
}

func validateOpportunityEvidence(evidence []api.OpportunityEvidence) error {
	for _, item := range evidence {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("invalid opportunity evidence.id: must not be empty")
		}
		if !isOpportunityTranslationKey(item.LabelKey) {
			return fmt.Errorf("invalid opportunity evidence.label_key: must be a translation key")
		}
		if strings.TrimSpace(item.DetailKey) != "" && !isOpportunityTranslationKey(item.DetailKey) {
			return fmt.Errorf("invalid opportunity evidence.detail_key: must be a translation key")
		}
		if err := rejectRawOpportunityParamFields("evidence.detail_params", item.DetailParams); err != nil {
			return err
		}
	}
	return nil
}

func validateOpportunityCitations(evidence []api.OpportunityEvidence, citedEvidenceIDs []string) error {
	evidenceIDs := make(map[string]bool, len(evidence))
	for _, item := range evidence {
		evidenceIDs[item.ID] = true
	}
	if len(citedEvidenceIDs) == 0 {
		return fmt.Errorf("invalid opportunity cited evidence: must include at least one evidence id")
	}
	for _, id := range citedEvidenceIDs {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("invalid opportunity cited evidence: must not be empty")
		}
		if !evidenceIDs[id] {
			return fmt.Errorf("invalid opportunity cited evidence %q: missing from evidence", id)
		}
	}
	return nil
}

func validateOpportunityCustomerParams(input OpportunityInput) error {
	if err := rejectRawOpportunityParamFields("copy_params", input.CopyParams); err != nil {
		return err
	}
	return rejectRawOpportunityParamFields("route_params", input.RouteParams)
}

func validateOpportunityPublicValues(input OpportunityInput) error {
	if err := rejectRawPayloadStringValues(input.ImpactValue); err != nil {
		return fmt.Errorf("invalid opportunity impact_value: %w", err)
	}
	for _, item := range input.Evidence {
		if err := rejectRawPayloadStringValues(item.Value); err != nil {
			return fmt.Errorf("invalid opportunity evidence.value: %w", err)
		}
	}
	return nil
}

func rejectRawOpportunityParamFields(field string, value any) error {
	if err := rejectRawPayloadFields(value); err != nil {
		return fmt.Errorf("invalid opportunity %s: %w", field, err)
	}
	if params, ok := value.(map[string]any); ok {
		for key := range params {
			switch strings.TrimSpace(key) {
			case "monthly_upside", "currency":
				return fmt.Errorf("invalid opportunity %s: money/upside param %q is not supported", field, key)
			}
		}
	}
	return nil
}

func isOpportunityTranslationKey(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && trimmed == value && strings.Contains(value, ".") && !strings.ContainsAny(value, " \t\r\n")
}

func (s *Store) ListOpportunities(ctx context.Context, siteID uuid.UUID) ([]api.Opportunity, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, team_id, site_id, kind, type_key, title_key, summary_key, action_key, digest_key,
			copy_params_json, impact_value, impact_label_key, confidence, score,
			score_breakdown_json, status, route_label_key, route_params_json, route_icon, detector_version, evidence_json,
			cited_evidence_ids_json, CAST(ai_run_id AS VARCHAR), generated_at, created_at, updated_at
		FROM opportunities
		WHERE site_id = ?
		ORDER BY score DESC, updated_at DESC
	`, siteID)
	if err != nil {
		return nil, fmt.Errorf("list opportunities: %w", err)
	}
	defer rows.Close()

	opportunities := []api.Opportunity{}
	for rows.Next() {
		opportunity, err := scanOpportunity(rows)
		if err != nil {
			return nil, err
		}
		opportunities = append(opportunities, opportunity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read opportunities: %w", err)
	}
	return opportunities, nil
}

func (s *Store) GetOpportunity(ctx context.Context, siteID, opportunityID uuid.UUID) (*api.Opportunity, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, team_id, site_id, kind, type_key, title_key, summary_key, action_key, digest_key,
			copy_params_json, impact_value, impact_label_key, confidence, score,
			score_breakdown_json, status, route_label_key, route_params_json, route_icon, detector_version, evidence_json,
			cited_evidence_ids_json, CAST(ai_run_id AS VARCHAR), generated_at, created_at, updated_at
		FROM opportunities
		WHERE site_id = ? AND id = ?
	`, siteID, opportunityID)
	opportunity, err := scanOpportunity(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &opportunity, nil
}

func (s *Store) UpdateOpportunityStatus(ctx context.Context, siteID, opportunityID uuid.UUID, status string) (*api.Opportunity, error) {
	return s.updateOpportunityStatus(ctx, siteID, opportunityID, status, nil)
}

func (s *Store) UpdateOpportunityStatusWithAudit(ctx context.Context, siteID, opportunityID uuid.UUID, status string, audit AuditEntryParams) (*api.Opportunity, error) {
	return s.updateOpportunityStatus(ctx, siteID, opportunityID, status, &audit)
}

func (s *Store) updateOpportunityStatus(ctx context.Context, siteID, opportunityID uuid.UUID, status string, audit *AuditEntryParams) (*api.Opportunity, error) {
	status = strings.TrimSpace(status)
	switch status {
	case "new", "saved", "done", "dismissed":
	default:
		return nil, fmt.Errorf("unsupported opportunity status")
	}
	var updated bool
	err := s.Transact(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE opportunities
			SET status = ?, updated_at = CURRENT_TIMESTAMP
			WHERE site_id = ? AND id = ?
		`, status, siteID, opportunityID)
		if err != nil {
			return fmt.Errorf("update opportunity status: %w", err)
		}
		if rows, _ := res.RowsAffected(); rows == 0 {
			return nil
		}
		updated = true
		if audit != nil {
			return appendAuditEntryTx(ctx, tx, *audit)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, nil
	}
	return s.GetOpportunity(ctx, siteID, opportunityID)
}

type opportunityScanner interface {
	Scan(dest ...any) error
}

type opportunityScanRaw struct {
	Evidence         any
	CopyParams       any
	RouteParams      any
	ScoreBreakdown   any
	CitedEvidenceIDs any
	AIRunID          sql.NullString
}

func scanOpportunity(row opportunityScanner) (api.Opportunity, error) {
	var opportunity api.Opportunity
	var raw opportunityScanRaw
	if err := scanOpportunityRow(row, &opportunity, &raw); err != nil {
		return api.Opportunity{}, err
	}
	if err := decodeOpportunityRaw(&opportunity, raw); err != nil {
		return api.Opportunity{}, err
	}
	normalizeOpportunity(&opportunity)
	assignOpportunityAIRunID(&opportunity, raw.AIRunID)
	return opportunity, nil
}

func scanOpportunityRow(row opportunityScanner, opportunity *api.Opportunity, raw *opportunityScanRaw) error {
	return row.Scan(
		&opportunity.ID,
		&opportunity.TeamID,
		&opportunity.SiteID,
		&opportunity.Kind,
		&opportunity.TypeKey,
		&opportunity.TitleKey,
		&opportunity.SummaryKey,
		&opportunity.ActionKey,
		&opportunity.DigestKey,
		&raw.CopyParams,
		&opportunity.ImpactValue,
		&opportunity.ImpactLabelKey,
		&opportunity.Confidence,
		&opportunity.Score,
		&raw.ScoreBreakdown,
		&opportunity.Status,
		&opportunity.RouteLabelKey,
		&raw.RouteParams,
		&opportunity.RouteIcon,
		&opportunity.DetectorVersion,
		&raw.Evidence,
		&raw.CitedEvidenceIDs,
		&raw.AIRunID,
		&opportunity.GeneratedAt,
		&opportunity.CreatedAt,
		&opportunity.UpdatedAt,
	)
}

func decodeOpportunityRaw(opportunity *api.Opportunity, raw opportunityScanRaw) error {
	fields := []struct {
		label string
		raw   any
		dest  any
	}{
		{label: "evidence", raw: raw.Evidence, dest: &opportunity.Evidence},
		{label: "copy params", raw: raw.CopyParams, dest: &opportunity.CopyParams},
		{label: "route params", raw: raw.RouteParams, dest: &opportunity.RouteParams},
		{label: "score breakdown", raw: raw.ScoreBreakdown, dest: &opportunity.ScoreBreakdown},
		{label: "cited evidence ids", raw: raw.CitedEvidenceIDs, dest: &opportunity.CitedEvidenceIDs},
	}
	for _, field := range fields {
		if err := decodeOpportunityJSONField(field.raw, field.dest, field.label); err != nil {
			return err
		}
	}
	return nil
}

func decodeOpportunityJSONField(raw any, dest any, label string) error {
	value := jsonScanString(raw)
	if value == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(value), dest); err != nil {
		return fmt.Errorf("decode opportunity %s: %w", label, err)
	}
	return nil
}

func normalizeOpportunity(opportunity *api.Opportunity) {
	if opportunity.Evidence == nil {
		opportunity.Evidence = []api.OpportunityEvidence{}
	}
	if opportunity.CopyParams == nil {
		opportunity.CopyParams = map[string]any{}
	}
	if opportunity.RouteParams == nil {
		opportunity.RouteParams = map[string]any{}
	}
	if opportunity.CitedEvidenceIDs == nil {
		opportunity.CitedEvidenceIDs = []string{}
	}
}

func assignOpportunityAIRunID(opportunity *api.Opportunity, raw sql.NullString) {
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return
	}
	if id, err := uuid.Parse(raw.String); err == nil {
		opportunity.AIRunID = &id
	}
}

func nonNilMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func nonNilSlice[T any](value []T) []T {
	if value == nil {
		return []T{}
	}
	return value
}

func jsonScanString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []byte:
		return strings.TrimSpace(string(typed))
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(raw))
	}
}
