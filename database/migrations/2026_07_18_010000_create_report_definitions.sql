CREATE TABLE report_definitions (
    id               UUID PRIMARY KEY DEFAULT uuidv7(),
    tenant_id        UUID REFERENCES tenants(id),
    -- User ownership is application-managed so user cleanup can preserve team
    -- reports while removing personal reports without DuckDB parent-row rewrites.
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
CREATE INDEX report_definitions_owner_user_id_idx ON report_definitions (owner_user_id);
CREATE INDEX report_definitions_tenant_id_idx ON report_definitions (tenant_id);
-- Report-internal relationships are application-managed below. DuckDB cannot
-- update referenced report or run parent rows after children exist, while the
-- scheduler must update next_run_at and lifecycle status throughout delivery.

CREATE TABLE report_definition_sites (
    id         UUID PRIMARY KEY DEFAULT uuidv7(),
    report_id  UUID NOT NULL,
    site_id    UUID NOT NULL REFERENCES sites(id),
    tenant_id  UUID REFERENCES tenants(id),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (report_id, site_id)
);
CREATE INDEX report_definition_sites_report_id_idx ON report_definition_sites (report_id);
CREATE INDEX report_definition_sites_site_id_idx ON report_definition_sites (site_id);

CREATE TABLE report_recipients (
    id                     UUID PRIMARY KEY DEFAULT uuidv7(),
    report_id              UUID NOT NULL,
    tenant_id              UUID REFERENCES tenants(id),
    user_id                UUID NOT NULL REFERENCES users(id),
    unsubscribe_token_hash VARCHAR,
    opted_out_at           TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL,
    updated_at             TIMESTAMPTZ NOT NULL,
    UNIQUE (report_id, user_id),
    UNIQUE (unsubscribe_token_hash)
);
CREATE INDEX report_recipients_report_id_idx ON report_recipients (report_id);
CREATE INDEX report_recipients_user_id_idx ON report_recipients (user_id);

CREATE TABLE report_runs (
    id                  UUID PRIMARY KEY DEFAULT uuidv7(),
    report_id           UUID NOT NULL,
    tenant_id           UUID REFERENCES tenants(id),
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
CREATE INDEX report_runs_report_id_idx ON report_runs (report_id, scheduled_for);

CREATE TABLE report_deliveries (
    id                  UUID PRIMARY KEY DEFAULT uuidv7(),
    report_id           UUID NOT NULL,
    run_id              UUID NOT NULL,
    tenant_id           UUID REFERENCES tenants(id),
    recipient_user_id   UUID NOT NULL REFERENCES users(id),
    status              VARCHAR NOT NULL CHECK (status IN ('queued', 'sending', 'accepted', 'failed', 'skipped')),
    message_id          VARCHAR NOT NULL,
    attempt_count       INTEGER NOT NULL DEFAULT 0,
    next_attempt_at     TIMESTAMPTZ,
    safe_error_code     VARCHAR,
    smtp_accepted_at    TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL,
    UNIQUE (run_id, recipient_user_id)
);
CREATE INDEX report_deliveries_run_id_idx ON report_deliveries (run_id);
CREATE INDEX report_deliveries_retry_idx ON report_deliveries (status, next_attempt_at);

-- Enabled legacy subscriptions become legacy-managed named reports. Their
-- UTC timezone and 08:00 local time preserve the previous dispatch behavior.
INSERT INTO report_definitions (
    id, owner_user_id, created_by, name, scope, preset, site_mode, frequency,
    timezone, local_time, weekly_day, monthly_day, status, source, legacy_key,
    next_run_at, created_at, updated_at
)
SELECT
    uuidv7(), ds.user_id, ds.user_id,
    'Portfolio Digest · ' || upper(substr(ds.frequency, 1, 1)) || substr(ds.frequency, 2),
    'personal', 'portfolio_digest', 'all_accessible', ds.frequency,
    'UTC', '08:00',
    CASE WHEN ds.frequency = 'weekly' THEN 1 ELSE NULL END,
    CASE WHEN ds.frequency = 'monthly' THEN 1 ELSE NULL END,
    'active', 'legacy',
    'digest:' || CAST(ds.user_id AS VARCHAR) || ':' || ds.frequency,
    NULL, ds.created_at, ds.updated_at
FROM digest_subscriptions ds
WHERE ds.enabled = true;

INSERT INTO report_definitions (
    id, owner_user_id, created_by, name, scope, preset, site_mode, frequency,
    timezone, local_time, weekly_day, monthly_day, status, source, legacy_key,
    next_run_at, created_at, updated_at
)
SELECT
    uuidv7(), srs.user_id, srs.user_id,
    'Site Summary · ' || s.domain,
    'personal', 'site_summary', 'selected', srs.frequency,
    'UTC', '08:00',
    CASE WHEN srs.frequency = 'weekly' THEN 1 ELSE NULL END,
    CASE WHEN srs.frequency = 'monthly' THEN 1 ELSE NULL END,
    'active', 'legacy',
    'site:' || CAST(srs.user_id AS VARCHAR) || ':' || CAST(srs.site_id AS VARCHAR) || ':' || srs.frequency,
    NULL, srs.created_at, srs.updated_at
FROM site_report_subscriptions srs
JOIN sites s ON s.id = srs.site_id
WHERE srs.enabled = true;

INSERT INTO report_definition_sites (id, report_id, site_id, tenant_id, created_at)
SELECT uuidv7(), rd.id, srs.site_id, st.tenant_id, rd.created_at
FROM site_report_subscriptions srs
JOIN report_definitions rd
  ON rd.legacy_key = 'site:' || CAST(srs.user_id AS VARCHAR) || ':' || CAST(srs.site_id AS VARCHAR) || ':' || srs.frequency
LEFT JOIN site_tenants st ON st.site_id = srs.site_id
WHERE srs.enabled = true;

INSERT INTO report_recipients (id, report_id, tenant_id, user_id, created_at, updated_at)
SELECT uuidv7(), rd.id, rd.tenant_id, rd.owner_user_id, rd.created_at, rd.updated_at
FROM report_definitions rd
WHERE rd.source = 'legacy';
