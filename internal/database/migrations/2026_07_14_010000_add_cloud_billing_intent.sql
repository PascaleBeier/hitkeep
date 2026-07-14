ALTER TABLE pending_signups
    ADD COLUMN plan_code VARCHAR DEFAULT 'free';

ALTER TABLE pending_signups
    ADD COLUMN billing_interval VARCHAR DEFAULT 'monthly';

ALTER TABLE cloud_billing_accounts
    ADD COLUMN billing_interval VARCHAR DEFAULT 'monthly';
