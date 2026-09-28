CREATE TABLE IF NOT EXISTS cloud_billing_accounts (
    tenant_id UUID PRIMARY KEY REFERENCES tenants (id),
    plan_code VARCHAR NOT NULL,
    plan_name VARCHAR NOT NULL,
    subscription_status VARCHAR NOT NULL,
    stripe_customer_id VARCHAR,
    stripe_subscription_id VARCHAR,
    stripe_price_id VARCHAR,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (stripe_customer_id),
    UNIQUE (stripe_subscription_id)
);

CREATE INDEX IF NOT EXISTS cloud_billing_accounts_plan_code_idx ON cloud_billing_accounts (plan_code);
