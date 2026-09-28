-- Legacy subscriptions were converted into report definitions by the initial
-- reports migration. Rebuild the parent table without migration-only markers,
-- then remove the obsolete subscription storage.

CREATE TABLE report_definitions_current (
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
    next_run_at      TIMESTAMPTZ,
    consent_version  INTEGER NOT NULL DEFAULT 1,
    created_at       TIMESTAMPTZ NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL,
    CHECK (
        (scope = 'personal' AND owner_user_id IS NOT NULL AND tenant_id IS NULL)
        OR (scope = 'team' AND owner_user_id IS NULL AND tenant_id IS NOT NULL)
    ),
    CHECK ((frequency = 'weekly' AND weekly_day IS NOT NULL) OR (frequency <> 'weekly' AND weekly_day IS NULL)),
    CHECK ((frequency = 'monthly' AND monthly_day IS NOT NULL) OR (frequency <> 'monthly' AND monthly_day IS NULL))
);

INSERT INTO report_definitions_current (
    id, tenant_id, owner_user_id, created_by, name, scope, preset, site_mode,
    frequency, timezone, local_time, weekly_day, monthly_day, status,
    next_run_at, consent_version, created_at, updated_at
)
SELECT
    id, tenant_id, owner_user_id, created_by, name, scope, preset, site_mode,
    frequency, timezone, local_time, weekly_day, monthly_day, status,
    next_run_at, consent_version, created_at, updated_at
FROM report_definitions;

DROP TABLE report_definitions;
ALTER TABLE report_definitions_current RENAME TO report_definitions;

CREATE INDEX report_definitions_owner_user_id_idx ON report_definitions (owner_user_id);
CREATE INDEX report_definitions_tenant_id_idx ON report_definitions (tenant_id);

DROP TABLE site_report_subscriptions;
DROP TABLE digest_subscriptions;
