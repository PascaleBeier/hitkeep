CREATE TABLE IF NOT EXISTS ai_runs (
    id UUID PRIMARY KEY,
    team_id UUID,
    site_id UUID,
    actor_id UUID,
    actor_type VARCHAR NOT NULL DEFAULT '',
    feature VARCHAR NOT NULL,
    provider VARCHAR NOT NULL,
    model VARCHAR NOT NULL,
    template_version VARCHAR NOT NULL DEFAULT '',
    evidence_ids_json JSON NOT NULL DEFAULT '[]',
    input_hash VARCHAR NOT NULL DEFAULT '',
    output_hash VARCHAR NOT NULL DEFAULT '',
    output_json JSON NOT NULL DEFAULT '{}',
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    total_tokens BIGINT NOT NULL DEFAULT 0,
    tool_call_count BIGINT NOT NULL DEFAULT 0,
    lifecycle_events_json JSON NOT NULL DEFAULT '[]',
    status VARCHAR NOT NULL,
    error_category VARCHAR NOT NULL DEFAULT '',
    latency_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_ai_runs_created ON ai_runs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_runs_team_site_created ON ai_runs (team_id, site_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_runs_feature_status ON ai_runs (feature, status);

CREATE TABLE IF NOT EXISTS opportunities (
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

CREATE INDEX IF NOT EXISTS idx_opportunities_site_status_score ON opportunities (site_id, status, score DESC);
CREATE INDEX IF NOT EXISTS idx_opportunities_team_site_updated ON opportunities (team_id, site_id, updated_at DESC);
