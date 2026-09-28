CREATE TABLE IF NOT EXISTS instance_audit_log (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    actor_id UUID REFERENCES users (id),
    actor_email_snapshot VARCHAR NOT NULL DEFAULT '',
    actor_role_snapshot VARCHAR NOT NULL DEFAULT '',
    action VARCHAR NOT NULL,
    target_type VARCHAR NOT NULL DEFAULT '',
    target_id VARCHAR NOT NULL DEFAULT '',
    target_label VARCHAR NOT NULL DEFAULT '',
    outcome VARCHAR NOT NULL DEFAULT 'success',
    ip_address VARCHAR NOT NULL DEFAULT '',
    user_agent VARCHAR NOT NULL DEFAULT '',
    request_id VARCHAR NOT NULL DEFAULT '',
    details VARCHAR NOT NULL DEFAULT '',
    metadata_json VARCHAR NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_instance_audit_log_created_at ON instance_audit_log (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_instance_audit_log_action ON instance_audit_log (action);
CREATE INDEX IF NOT EXISTS idx_instance_audit_log_actor ON instance_audit_log (actor_id);
CREATE INDEX IF NOT EXISTS idx_instance_audit_log_outcome ON instance_audit_log (outcome);
