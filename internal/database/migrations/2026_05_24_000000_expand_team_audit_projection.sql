CREATE OR REPLACE VIEW team_audit_log AS
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
