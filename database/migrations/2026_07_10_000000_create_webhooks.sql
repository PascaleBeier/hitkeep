CREATE TABLE IF NOT EXISTS webhooks (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    site_id         UUID REFERENCES sites(id),
    name            VARCHAR     NOT NULL,
    description     VARCHAR     NOT NULL DEFAULT '',
    destination_url VARCHAR     NOT NULL,
    secret          VARCHAR     NOT NULL,
    enabled         BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS webhooks_site_id_idx ON webhooks (site_id);

CREATE TABLE IF NOT EXISTS webhook_event_subscriptions (
    webhook_id UUID    NOT NULL,
    event_type VARCHAR NOT NULL,
    PRIMARY KEY (webhook_id, event_type)
);

CREATE TABLE IF NOT EXISTS webhook_events (
    id           UUID PRIMARY KEY DEFAULT uuidv7(),
    site_id      UUID,
    event_type   VARCHAR     NOT NULL,
    api_version  VARCHAR     NOT NULL,
    payload_json JSON        NOT NULL,
    occurred_at  TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS webhook_events_site_created_idx ON webhook_events (site_id, created_at DESC);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id                  UUID PRIMARY KEY DEFAULT uuidv7(),
    event_id            UUID        NOT NULL,
    webhook_id          UUID        NOT NULL,
    site_id             UUID,
    event_type          VARCHAR     NOT NULL,
    webhook_name        VARCHAR     NOT NULL,
    destination_url     VARCHAR     NOT NULL,
    signing_secret      VARCHAR     NOT NULL,
    payload_json        JSON        NOT NULL,
    status              VARCHAR     NOT NULL,
    attempt_count       INTEGER     NOT NULL DEFAULT 0,
    next_attempt_at     TIMESTAMPTZ,
    last_attempt_at     TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ,
    response_status     INTEGER,
    last_error_code     VARCHAR     NOT NULL DEFAULT '',
    last_error_message  VARCHAR     NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS webhook_deliveries_due_idx ON webhook_deliveries (status, next_attempt_at);
CREATE INDEX IF NOT EXISTS webhook_deliveries_webhook_created_idx ON webhook_deliveries (webhook_id, created_at DESC);
CREATE INDEX IF NOT EXISTS webhook_deliveries_site_created_idx ON webhook_deliveries (site_id, created_at DESC);

CREATE TABLE IF NOT EXISTS webhook_delivery_attempts (
    id               UUID PRIMARY KEY DEFAULT uuidv7(),
    delivery_id      UUID        NOT NULL,
    site_id          UUID,
    attempt_number   INTEGER     NOT NULL,
    status           VARCHAR     NOT NULL,
    response_status  INTEGER,
    error_code       VARCHAR     NOT NULL DEFAULT '',
    error_message    VARCHAR     NOT NULL DEFAULT '',
    started_at       TIMESTAMPTZ NOT NULL,
    completed_at     TIMESTAMPTZ NOT NULL,
    next_attempt_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS webhook_delivery_attempts_delivery_idx ON webhook_delivery_attempts (delivery_id, attempt_number);
CREATE INDEX IF NOT EXISTS webhook_delivery_attempts_site_idx ON webhook_delivery_attempts (site_id, completed_at DESC);
