UPDATE opportunities
SET
    kind = CASE WHEN kind = 'revenue' THEN 'traffic' ELSE kind END,
    impact_label_key = CASE
        WHEN impact_label_key = 'opportunities.impact.estimated_monthly_upside' THEN 'opportunities.impact.checkout_starts'
        ELSE impact_label_key
    END,
    copy_params_json = json_merge_patch(copy_params_json, '{"monthly_upside":null,"currency":null}');

DROP INDEX IF EXISTS idx_opportunities_site_status_score;
DROP INDEX IF EXISTS idx_opportunities_team_site_updated;

CREATE TABLE opportunities_without_money_contract (
    id UUID PRIMARY KEY,
    team_id UUID NOT NULL,
    site_id UUID NOT NULL,
    kind VARCHAR NOT NULL,
    type_key VARCHAR NOT NULL,
    title_key VARCHAR NOT NULL,
    summary_key VARCHAR NOT NULL,
    action_key VARCHAR NOT NULL,
    digest_key VARCHAR NOT NULL DEFAULT '',
    copy_params_json JSON NOT NULL DEFAULT '{}',
    impact_value VARCHAR NOT NULL,
    impact_label_key VARCHAR NOT NULL,
    confidence VARCHAR NOT NULL,
    score BIGINT NOT NULL DEFAULT 0,
    score_breakdown_json JSON NOT NULL DEFAULT '{}',
    status VARCHAR NOT NULL,
    route_label_key VARCHAR NOT NULL DEFAULT '',
    route_params_json JSON NOT NULL DEFAULT '{}',
    route_icon VARCHAR NOT NULL DEFAULT '',
    detector_version VARCHAR NOT NULL DEFAULT '',
    evidence_json JSON NOT NULL DEFAULT '[]',
    cited_evidence_ids_json JSON NOT NULL DEFAULT '[]',
    ai_run_id UUID,
    generated_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO opportunities_without_money_contract (
    id, team_id, site_id, kind, type_key, title_key, summary_key, action_key, digest_key,
    copy_params_json, impact_value, impact_label_key, confidence, score, score_breakdown_json,
    status, route_label_key, route_params_json, route_icon, detector_version, evidence_json,
    cited_evidence_ids_json, ai_run_id, generated_at, created_at, updated_at
)
SELECT
    id, team_id, site_id, kind, type_key, title_key, summary_key, action_key, digest_key,
    copy_params_json, impact_value, impact_label_key, confidence, score, score_breakdown_json,
    status, route_label_key, route_params_json, route_icon, detector_version, evidence_json,
    cited_evidence_ids_json, ai_run_id, generated_at, created_at, updated_at
FROM opportunities;

DROP TABLE opportunities;
ALTER TABLE opportunities_without_money_contract RENAME TO opportunities;

CREATE INDEX IF NOT EXISTS idx_opportunities_site_status_score ON opportunities (site_id, status, score DESC);
CREATE INDEX IF NOT EXISTS idx_opportunities_team_site_updated ON opportunities (team_id, site_id, updated_at DESC);
