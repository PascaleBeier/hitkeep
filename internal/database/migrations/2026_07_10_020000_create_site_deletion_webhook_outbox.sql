CREATE TABLE IF NOT EXISTS site_deletion_webhook_outbox (
    delivery_id        UUID PRIMARY KEY,
    event_id           UUID        NOT NULL,
    source_site_id     UUID        NOT NULL,
    webhook_id         UUID        NOT NULL,
    event_type         VARCHAR     NOT NULL,
    api_version        VARCHAR     NOT NULL,
    webhook_name       VARCHAR     NOT NULL,
    destination_url    VARCHAR     NOT NULL,
    signing_secret     VARCHAR     NOT NULL,
    event_payload_json JSON        NOT NULL,
    payload_json       JSON        NOT NULL,
    occurred_at        TIMESTAMPTZ NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS site_deletion_webhook_outbox_event_idx
    ON site_deletion_webhook_outbox (event_id);

CREATE INDEX IF NOT EXISTS site_deletion_webhook_outbox_site_idx
    ON site_deletion_webhook_outbox (source_site_id, created_at);
