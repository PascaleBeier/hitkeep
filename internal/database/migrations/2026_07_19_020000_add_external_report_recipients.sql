-- Rebuild the named-report tables as one dependency-safe unit. Development
-- databases that applied the first named-report migration may still have
-- DuckDB foreign keys from child tables to report_definitions. DuckDB cannot
-- ALTER that referenced parent in place, so preserve all rows in unconstrained
-- replacement tables before dropping the old dependency graph.

CREATE TABLE report_definitions_v2 (
    id               UUID PRIMARY KEY DEFAULT uuidv7(),
    tenant_id        UUID,
    owner_user_id    UUID,
    created_by       UUID,
    name             VARCHAR NOT NULL,
    scope            VARCHAR NOT NULL CHECK (scope IN ('personal', 'team')),
    preset           VARCHAR NOT NULL CHECK (preset IN ('site_summary', 'portfolio_digest', 'opportunity_brief')),
    site_mode        VARCHAR NOT NULL DEFAULT 'selected' CHECK (site_mode IN ('selected', 'all_accessible')),
    frequency        VARCHAR NOT NULL CHECK (frequency IN ('daily', 'weekly', 'monthly')),
    timezone         VARCHAR NOT NULL,
    local_time       VARCHAR NOT NULL CHECK (regexp_matches(local_time, '^([01][0-9]|2[0-3]):(00|15|30|45)$')),
    weekly_day       SMALLINT CHECK (weekly_day BETWEEN 0 AND 6),
    monthly_day      SMALLINT CHECK (monthly_day BETWEEN 1 AND 28),
    status           VARCHAR NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'paused')),
    source           VARCHAR NOT NULL DEFAULT 'v2' CHECK (source IN ('v2', 'legacy')),
    legacy_key       VARCHAR,
    next_run_at      TIMESTAMPTZ,
    consent_version  INTEGER NOT NULL DEFAULT 1,
    created_at       TIMESTAMPTZ NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL,
    CHECK (
        (scope = 'personal' AND owner_user_id IS NOT NULL AND tenant_id IS NULL)
        OR (scope = 'team' AND owner_user_id IS NULL AND tenant_id IS NOT NULL)
    ),
    CHECK ((frequency = 'weekly' AND weekly_day IS NOT NULL) OR (frequency <> 'weekly' AND weekly_day IS NULL)),
    CHECK ((frequency = 'monthly' AND monthly_day IS NOT NULL) OR (frequency <> 'monthly' AND monthly_day IS NULL)),
    UNIQUE (legacy_key)
);

INSERT INTO report_definitions_v2 (
    id, tenant_id, owner_user_id, created_by, name, scope, preset, site_mode,
    frequency, timezone, local_time, weekly_day, monthly_day, status, source,
    legacy_key, next_run_at, consent_version, created_at, updated_at
)
SELECT
    id, tenant_id, owner_user_id, created_by, name, scope, preset, site_mode,
    frequency, timezone, local_time, weekly_day, monthly_day, status, source,
    legacy_key, next_run_at, 1, created_at, updated_at
FROM report_definitions;

CREATE TABLE report_definition_sites_v2 (
    id         UUID PRIMARY KEY DEFAULT uuidv7(),
    report_id  UUID NOT NULL,
    site_id    UUID NOT NULL,
    tenant_id  UUID,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (report_id, site_id)
);

INSERT INTO report_definition_sites_v2 (id, report_id, site_id, tenant_id, created_at)
SELECT id, report_id, site_id, tenant_id, created_at
FROM report_definition_sites;

CREATE TABLE report_recipients_v2 (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    report_id                UUID NOT NULL,
    tenant_id                UUID,
    user_id                  UUID,
    external_email           VARCHAR,
    external_locale          VARCHAR,
    consent_version          INTEGER NOT NULL DEFAULT 1,
    confirmation_token_hash  VARCHAR,
    confirmation_expires_at  TIMESTAMPTZ,
    confirmation_sent_at     TIMESTAMPTZ,
    confirmation_error_code  VARCHAR,
    confirmed_at             TIMESTAMPTZ,
    unsubscribe_token_hash   VARCHAR,
    opted_out_at             TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL,
    updated_at               TIMESTAMPTZ NOT NULL,
    CHECK (
        (user_id IS NOT NULL AND external_email IS NULL)
        OR (user_id IS NULL AND external_email IS NOT NULL)
    ),
    UNIQUE (report_id, user_id),
    UNIQUE (report_id, external_email),
    UNIQUE (confirmation_token_hash),
    UNIQUE (unsubscribe_token_hash)
);

INSERT INTO report_recipients_v2 (
    id, report_id, tenant_id, user_id, consent_version, confirmed_at,
    unsubscribe_token_hash, opted_out_at, created_at, updated_at
)
SELECT
    id, report_id, tenant_id, user_id, 1, created_at,
    unsubscribe_token_hash, opted_out_at, created_at, updated_at
FROM report_recipients;

CREATE TABLE report_runs_v2 (
    id                  UUID PRIMARY KEY DEFAULT uuidv7(),
    report_id           UUID NOT NULL,
    tenant_id           UUID,
    scheduled_for       TIMESTAMPTZ NOT NULL,
    period_start        TIMESTAMPTZ NOT NULL,
    period_end          TIMESTAMPTZ NOT NULL,
    status              VARCHAR NOT NULL CHECK (status IN ('queued', 'running', 'completed', 'partial', 'failed', 'skipped')),
    safe_error_code     VARCHAR,
    started_at          TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL,
    UNIQUE (report_id, scheduled_for)
);

INSERT INTO report_runs_v2 (
    id, report_id, tenant_id, scheduled_for, period_start, period_end, status,
    safe_error_code, started_at, completed_at, created_at, updated_at
)
SELECT
    id, report_id, tenant_id, scheduled_for, period_start, period_end, status,
    safe_error_code, started_at, completed_at, created_at, updated_at
FROM report_runs;

CREATE TABLE report_deliveries_v2 (
    id                  UUID PRIMARY KEY DEFAULT uuidv7(),
    report_id           UUID NOT NULL,
    run_id              UUID NOT NULL,
    tenant_id           UUID,
    recipient_id        UUID NOT NULL,
    recipient_kind      VARCHAR NOT NULL CHECK (recipient_kind IN ('member', 'external')),
    status              VARCHAR NOT NULL CHECK (status IN ('queued', 'sending', 'accepted', 'failed', 'skipped')),
    message_id          VARCHAR NOT NULL,
    attempt_count       INTEGER NOT NULL DEFAULT 0,
    next_attempt_at     TIMESTAMPTZ,
    safe_error_code     VARCHAR,
    smtp_accepted_at    TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL,
    UNIQUE (run_id, recipient_id)
);

INSERT INTO report_deliveries_v2 (
    id, report_id, run_id, tenant_id, recipient_id, recipient_kind, status,
    message_id, attempt_count, next_attempt_at, safe_error_code,
    smtp_accepted_at, created_at, updated_at
)
SELECT
    d.id, d.report_id, d.run_id, d.tenant_id, rr.id, 'member', d.status,
    d.message_id, d.attempt_count, d.next_attempt_at, d.safe_error_code,
    d.smtp_accepted_at, d.created_at, d.updated_at
FROM report_deliveries d
JOIN report_recipients_v2 rr
  ON rr.report_id = d.report_id AND rr.user_id = d.recipient_user_id;

DROP TABLE report_deliveries;
DROP TABLE report_definition_sites;
DROP TABLE report_recipients;
DROP TABLE report_runs;
DROP TABLE report_definitions;

ALTER TABLE report_definitions_v2 RENAME TO report_definitions;
ALTER TABLE report_definition_sites_v2 RENAME TO report_definition_sites;
ALTER TABLE report_recipients_v2 RENAME TO report_recipients;
ALTER TABLE report_runs_v2 RENAME TO report_runs;
ALTER TABLE report_deliveries_v2 RENAME TO report_deliveries;

CREATE INDEX report_definitions_owner_user_id_idx ON report_definitions (owner_user_id);
CREATE INDEX report_definitions_tenant_id_idx ON report_definitions (tenant_id);
CREATE INDEX report_definition_sites_report_id_idx ON report_definition_sites (report_id);
CREATE INDEX report_definition_sites_site_id_idx ON report_definition_sites (site_id);
CREATE INDEX report_recipients_report_id_idx ON report_recipients (report_id);
CREATE INDEX report_recipients_user_id_idx ON report_recipients (user_id);
CREATE INDEX report_recipients_confirmation_idx ON report_recipients (confirmation_token_hash);
CREATE INDEX report_runs_report_id_idx ON report_runs (report_id, scheduled_for);
CREATE INDEX report_deliveries_run_id_idx ON report_deliveries (run_id);
CREATE INDEX report_deliveries_retry_idx ON report_deliveries (status, next_attempt_at);
