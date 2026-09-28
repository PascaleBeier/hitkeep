ALTER TABLE instance_audit_log ADD COLUMN IF NOT EXISTS team_id UUID;
ALTER TABLE instance_audit_log ADD COLUMN IF NOT EXISTS target_user_id UUID;
ALTER TABLE instance_audit_log ADD COLUMN IF NOT EXISTS ip_country_code VARCHAR DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_instance_audit_log_team_created ON instance_audit_log (team_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_instance_audit_log_target_type ON instance_audit_log (target_type);

INSERT INTO instance_audit_log (
    id,
    created_at,
    actor_id,
    actor_email_snapshot,
    actor_role_snapshot,
    action,
    target_type,
    target_id,
    target_label,
    outcome,
    details,
    metadata_json,
    team_id,
    target_user_id
)
SELECT
    ta.id,
    ta.created_at,
    ta.actor_id,
    COALESCE(actor.email, ''),
    COALESCE(tm.role, ''),
    ta.action,
    'team',
    CAST(ta.tenant_id AS VARCHAR),
    COALESCE(t.name, ''),
    'success',
    ta.details,
    '{}',
    ta.tenant_id,
    ta.target_user_id
FROM team_audit_log ta
LEFT JOIN users actor ON actor.id = ta.actor_id
LEFT JOIN tenant_members tm ON tm.tenant_id = ta.tenant_id AND tm.user_id = ta.actor_id
LEFT JOIN tenants t ON t.id = ta.tenant_id
WHERE NOT EXISTS (
    SELECT 1
    FROM instance_audit_log ial
    WHERE ial.id = ta.id
);

DROP INDEX IF EXISTS team_audit_log_tenant_created_idx;

DROP TABLE team_audit_log;

CREATE VIEW team_audit_log AS
SELECT
    id,
    team_id AS tenant_id,
    actor_id,
    actor_email_snapshot,
    actor_role_snapshot,
    target_user_id,
    action,
    target_type,
    target_id,
    target_label,
    outcome,
    ip_address,
    ip_country_code,
    user_agent,
    request_id,
    details,
    created_at
FROM instance_audit_log
WHERE team_id IS NOT NULL;
