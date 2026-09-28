CREATE TABLE site_report_subscriptions (
    id         UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id    UUID NOT NULL REFERENCES users(id),
    site_id    UUID NOT NULL REFERENCES sites(id),
    frequency  VARCHAR NOT NULL CHECK (frequency IN ('daily', 'weekly', 'monthly')),
    enabled    BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (user_id, site_id, frequency)
);
CREATE INDEX site_report_subscriptions_user_id_idx ON site_report_subscriptions (user_id);

CREATE TABLE digest_subscriptions (
    id         UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id    UUID NOT NULL REFERENCES users(id),
    frequency  VARCHAR NOT NULL CHECK (frequency IN ('daily', 'weekly', 'monthly')),
    enabled    BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (user_id, frequency)
);
CREATE INDEX digest_subscriptions_user_id_idx ON digest_subscriptions (user_id);
