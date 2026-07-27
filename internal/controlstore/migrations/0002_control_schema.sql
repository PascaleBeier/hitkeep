CREATE TABLE data_migrations (
    name TEXT PRIMARY KEY,
    applied_at TIMESTAMP NOT NULL
);

CREATE TABLE users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    given_name TEXT,
    last_name TEXT,
    password_login_enabled INTEGER NOT NULL DEFAULT 1 CHECK (password_login_enabled IN (0, 1))
);

CREATE UNIQUE INDEX users_email_nocase_idx ON users(lower(email));

CREATE TABLE tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    is_default INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1)),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    logo_url TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX tenants_one_default_idx ON tenants(is_default) WHERE is_default = 1;

CREATE TABLE sites (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    domain TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL,
    data_retention_days INTEGER DEFAULT 365,
    retention_synced_from_plan INTEGER NOT NULL DEFAULT 1 CHECK (retention_synced_from_plan IN (0, 1))
);

CREATE INDEX sites_user_id_idx ON sites(user_id);

CREATE TABLE site_tenants (
    site_id TEXT PRIMARY KEY REFERENCES sites(id),
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(site_id, tenant_id)
);

CREATE INDEX site_tenants_tenant_idx ON site_tenants(tenant_id);

CREATE TABLE tenant_members (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    role TEXT NOT NULL,
    added_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    added_by TEXT REFERENCES users(id),
    UNIQUE(tenant_id, user_id)
);

CREATE INDEX tenant_members_tenant_idx ON tenant_members(tenant_id);
CREATE INDEX tenant_members_user_idx ON tenant_members(user_id);

CREATE TABLE site_members (
    id TEXT PRIMARY KEY,
    site_id TEXT NOT NULL REFERENCES sites(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    role TEXT NOT NULL,
    added_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    added_by TEXT REFERENCES users(id),
    UNIQUE(site_id, user_id)
);

CREATE INDEX site_members_site_idx ON site_members(site_id);
CREATE INDEX site_members_user_idx ON site_members(user_id);

CREATE TABLE instance_roles (
    user_id TEXT PRIMARY KEY REFERENCES users(id),
    role TEXT NOT NULL,
    granted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    granted_by TEXT REFERENCES users(id)
);

CREATE TABLE user_preferences (
    user_id TEXT PRIMARY KEY REFERENCES users(id),
    default_locale TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    active_tenant_id TEXT REFERENCES tenants(id),
    dismissed_onboarding_at TIMESTAMP
);

CREATE TABLE remember_me_tokens (
    token TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL
);

CREATE INDEX remember_me_tokens_user_id_idx ON remember_me_tokens(user_id);

CREATE TABLE user_totp_factors (
    user_id TEXT PRIMARY KEY REFERENCES users(id),
    secret TEXT NOT NULL,
    enabled_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE user_recovery_codes (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    code_hash TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    used_at TIMESTAMP,
    UNIQUE(user_id, code_hash)
);

CREATE INDEX user_recovery_codes_user_id_idx ON user_recovery_codes(user_id);

CREATE TABLE user_passkeys (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    credential_id TEXT NOT NULL UNIQUE,
    public_key TEXT,
    transports_json TEXT CHECK (transports_json IS NULL OR json_valid(transports_json)),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    sign_count INTEGER,
    credential_json TEXT CHECK (credential_json IS NULL OR json_valid(credential_json))
);

CREATE INDEX user_passkeys_user_id_idx ON user_passkeys(user_id);

CREATE TABLE pending_signups (
    token TEXT PRIMARY KEY,
    email TEXT NOT NULL,
    hashed_password TEXT NOT NULL,
    given_name TEXT NOT NULL DEFAULT '',
    last_name TEXT NOT NULL DEFAULT '',
    team_name TEXT NOT NULL DEFAULT '',
    jurisdiction TEXT NOT NULL DEFAULT '',
    locale TEXT NOT NULL DEFAULT '',
    accepted_tos_at TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    plan_code TEXT DEFAULT 'free',
    billing_interval TEXT DEFAULT 'monthly'
);

CREATE INDEX pending_signups_email_idx ON pending_signups(email);
CREATE INDEX pending_signups_expires_at_idx ON pending_signups(expires_at);

CREATE TABLE social_identities (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    provider TEXT NOT NULL CHECK (provider IN ('google', 'github', 'microsoft')),
    subject TEXT NOT NULL,
    observed_email TEXT NOT NULL DEFAULT '',
    linked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP,
    UNIQUE(provider, subject),
    UNIQUE(user_id, provider)
);

CREATE INDEX social_identities_user_idx ON social_identities(user_id);

CREATE TABLE pending_social_confirmations (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    provider TEXT NOT NULL CHECK (provider IN ('google', 'github', 'microsoft')),
    subject TEXT NOT NULL,
    observed_email TEXT NOT NULL DEFAULT '',
    target_email TEXT NOT NULL,
    target_user_id TEXT REFERENCES users(id),
    team_name TEXT NOT NULL DEFAULT '',
    jurisdiction TEXT NOT NULL DEFAULT '',
    locale TEXT NOT NULL DEFAULT '',
    plan_code TEXT NOT NULL DEFAULT 'free',
    billing_interval TEXT NOT NULL DEFAULT 'monthly',
    accepted_tos_at TIMESTAMP,
    return_path TEXT NOT NULL DEFAULT '/dashboard',
    remember_me INTEGER NOT NULL DEFAULT 0 CHECK (remember_me IN (0, 1)),
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(provider, subject)
);

CREATE INDEX pending_social_confirmations_expires_idx ON pending_social_confirmations(expires_at);
CREATE INDEX pending_social_confirmations_user_idx ON pending_social_confirmations(target_user_id);

CREATE TABLE api_clients (
    id TEXT PRIMARY KEY,
    user_id TEXT REFERENCES users(id),
    tenant_id TEXT REFERENCES tenants(id),
    name TEXT NOT NULL,
    description TEXT,
    secret_hash TEXT NOT NULL UNIQUE,
    instance_role TEXT NOT NULL DEFAULT 'user',
    expires_at TIMESTAMP,
    last_used_at TIMESTAMP,
    revoked_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX api_clients_secret_hash_idx ON api_clients(secret_hash);
CREATE INDEX api_clients_tenant_id_idx ON api_clients(tenant_id);
CREATE INDEX api_clients_user_id_idx ON api_clients(user_id);

CREATE TABLE api_client_site_roles (
    id TEXT PRIMARY KEY,
    api_client_id TEXT NOT NULL,
    site_id TEXT NOT NULL REFERENCES sites(id),
    role TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    UNIQUE(api_client_id, site_id)
);

CREATE INDEX api_client_site_roles_client_id_idx ON api_client_site_roles(api_client_id);
CREATE INDEX api_client_site_roles_site_id_idx ON api_client_site_roles(site_id);

CREATE TABLE tenant_archives (
    tenant_id TEXT PRIMARY KEY REFERENCES tenants(id),
    archived_at TIMESTAMP NOT NULL,
    archived_by TEXT REFERENCES users(id)
);

CREATE INDEX tenant_archives_archived_at_idx ON tenant_archives(archived_at);

CREATE TABLE team_invites (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    email TEXT NOT NULL,
    role TEXT NOT NULL,
    invited_user_id TEXT REFERENCES users(id),
    status TEXT NOT NULL DEFAULT 'pending',
    created_by TEXT REFERENCES users(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    accepted_at TIMESTAMP,
    revoked_at TIMESTAMP,
    requires_password_setup INTEGER CHECK (requires_password_setup IS NULL OR requires_password_setup IN (0, 1))
);

CREATE TABLE team_sso_configs (
    tenant_id TEXT PRIMARY KEY REFERENCES tenants(id),
    provider_type TEXT NOT NULL DEFAULT 'oidc' CHECK (provider_type = 'oidc'),
    issuer_url TEXT NOT NULL,
    client_id TEXT NOT NULL,
    client_secret_encrypted TEXT NOT NULL,
    email_claim TEXT NOT NULL DEFAULT 'email',
    display_name_claim TEXT NOT NULL DEFAULT 'name',
    enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    auto_provision INTEGER DEFAULT 0 CHECK (auto_provision IS NULL OR auto_provision IN (0, 1))
);

CREATE TABLE team_sso_domains (
    tenant_id TEXT NOT NULL REFERENCES team_sso_configs(tenant_id),
    domain TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(tenant_id, domain)
);

CREATE INDEX team_sso_domains_tenant_idx ON team_sso_domains(tenant_id);

CREATE TABLE sso_identities (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    issuer_url TEXT NOT NULL,
    subject TEXT NOT NULL,
    email TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, issuer_url, subject),
    UNIQUE(tenant_id, user_id)
);

CREATE INDEX sso_identities_tenant_idx ON sso_identities(tenant_id);
CREATE INDEX sso_identities_user_idx ON sso_identities(user_id);

CREATE TABLE instance_audit_log (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    actor_id TEXT REFERENCES users(id),
    actor_email_snapshot TEXT NOT NULL DEFAULT '',
    actor_role_snapshot TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    target_type TEXT NOT NULL DEFAULT '',
    target_id TEXT NOT NULL DEFAULT '',
    target_label TEXT NOT NULL DEFAULT '',
    outcome TEXT NOT NULL DEFAULT 'success',
    ip_address TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT '',
    details TEXT NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(metadata_json)),
    team_id TEXT REFERENCES tenants(id),
    target_user_id TEXT REFERENCES users(id),
    ip_country_code TEXT DEFAULT ''
);

CREATE INDEX instance_audit_log_action_idx ON instance_audit_log(action);
CREATE INDEX instance_audit_log_actor_idx ON instance_audit_log(actor_id);
CREATE INDEX instance_audit_log_created_at_idx ON instance_audit_log(created_at);
CREATE INDEX instance_audit_log_team_created_idx ON instance_audit_log(team_id, created_at);

CREATE VIEW team_audit_log AS
SELECT id, team_id AS tenant_id, actor_id, actor_email_snapshot,
       actor_role_snapshot, target_user_id, action, target_type, target_id,
       target_label, outcome, ip_address, ip_country_code, user_agent,
       request_id, details, created_at
FROM instance_audit_log
WHERE team_id IS NOT NULL;

CREATE TABLE share_links (
    id TEXT PRIMARY KEY,
    site_id TEXT NOT NULL REFERENCES sites(id),
    token_hash TEXT NOT NULL UNIQUE,
    created_by TEXT REFERENCES users(id),
    created_at TIMESTAMP NOT NULL,
    revoked_at TIMESTAMP
);

CREATE INDEX share_links_site_id_idx ON share_links(site_id);
CREATE INDEX share_links_token_hash_idx ON share_links(token_hash);

CREATE TABLE qr_codes (
    id TEXT PRIMARY KEY,
    site_id TEXT NOT NULL REFERENCES sites(id),
    created_by TEXT REFERENCES users(id),
    name TEXT NOT NULL,
    destination_url TEXT NOT NULL,
    utm_source TEXT,
    utm_medium TEXT,
    utm_campaign TEXT,
    utm_term TEXT,
    utm_content TEXT,
    custom_params_json TEXT CHECK (custom_params_json IS NULL OR json_valid(custom_params_json)),
    style_json TEXT CHECK (style_json IS NULL OR json_valid(style_json)),
    token TEXT NOT NULL UNIQUE,
    token_hash TEXT NOT NULL UNIQUE,
    token_hint TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    archived_at TIMESTAMP
);

CREATE INDEX qr_codes_site_id_idx ON qr_codes(site_id);
CREATE INDEX qr_codes_token_hash_idx ON qr_codes(token_hash);

CREATE TABLE qr_code_assets (
    qr_code_id TEXT PRIMARY KEY,
    site_id TEXT NOT NULL REFERENCES sites(id),
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL,
    byte_size INTEGER NOT NULL,
    width INTEGER,
    height INTEGER,
    checksum TEXT NOT NULL,
    storage_key TEXT NOT NULL DEFAULT '',
    data BLOB,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX qr_code_assets_site_id_idx ON qr_code_assets(site_id);

CREATE TABLE qr_code_share_links (
    id TEXT PRIMARY KEY,
    site_id TEXT NOT NULL REFERENCES sites(id),
    qr_code_id TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    token_hint TEXT NOT NULL,
    created_by TEXT REFERENCES users(id),
    created_at TIMESTAMP NOT NULL,
    revoked_at TIMESTAMP
);

CREATE INDEX qr_code_share_links_qr_code_id_idx ON qr_code_share_links(qr_code_id);
CREATE INDEX qr_code_share_links_site_id_idx ON qr_code_share_links(site_id);
CREATE INDEX qr_code_share_links_token_hash_idx ON qr_code_share_links(token_hash);

CREATE TABLE site_imports (
    id TEXT PRIMARY KEY,
    site_id TEXT NOT NULL REFERENCES sites(id),
    provider TEXT NOT NULL,
    status TEXT NOT NULL,
    source_hash TEXT,
    manifest TEXT CHECK (manifest IS NULL OR json_valid(manifest)),
    error TEXT,
    bytes_total INTEGER NOT NULL DEFAULT 0,
    bytes_received INTEGER NOT NULL DEFAULT 0,
    rows_scanned INTEGER NOT NULL DEFAULT 0,
    rows_imported INTEGER NOT NULL DEFAULT 0,
    created_by TEXT REFERENCES users(id),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    validated_at TIMESTAMP,
    started_at TIMESTAMP,
    finished_at TIMESTAMP
);

CREATE INDEX site_imports_site_status_idx ON site_imports(site_id, status, created_at);

CREATE TABLE site_import_files (
    import_id TEXT NOT NULL,
    file_id TEXT NOT NULL,
    filename TEXT NOT NULL,
    relative_path TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    bytes_received INTEGER NOT NULL DEFAULT 0,
    sha256 TEXT,
    status TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    cleaned_at TIMESTAMP,
    PRIMARY KEY(import_id, file_id)
);

CREATE INDEX site_import_files_import_idx ON site_import_files(import_id);

CREATE TABLE google_search_console_connections (
    team_id TEXT PRIMARY KEY REFERENCES tenants(id),
    connected_by_user_id TEXT REFERENCES users(id),
    google_account_email TEXT DEFAULT '',
    google_account_id TEXT DEFAULT '',
    access_token TEXT DEFAULT '',
    refresh_token TEXT DEFAULT '',
    token_type TEXT DEFAULT '',
    scope TEXT DEFAULT '',
    token_expiry TIMESTAMP,
    connected INTEGER DEFAULT 0 CHECK (connected IS NULL OR connected IN (0, 1)),
    connected_at TIMESTAMP,
    disconnected_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE google_search_console_properties (
    team_id TEXT NOT NULL REFERENCES tenants(id),
    property_uri TEXT NOT NULL,
    permission_level TEXT DEFAULT '',
    last_seen_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(team_id, property_uri)
);

CREATE TABLE google_search_console_site_mappings (
    site_id TEXT PRIMARY KEY REFERENCES sites(id),
    team_id TEXT NOT NULL REFERENCES tenants(id),
    property_uri TEXT NOT NULL,
    mapped_by_user_id TEXT REFERENCES users(id),
    mapped_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE google_search_console_sync_state (
    site_id TEXT PRIMARY KEY REFERENCES sites(id),
    team_id TEXT NOT NULL REFERENCES tenants(id),
    state TEXT NOT NULL DEFAULT 'idle',
    imported_start_date DATE,
    imported_end_date DATE,
    last_success_at TIMESTAMP,
    last_attempt_at TIMESTAMP,
    last_error_category TEXT DEFAULT '',
    next_retry_at TIMESTAMP,
    manual INTEGER NOT NULL DEFAULT 0 CHECK (manual IN (0, 1)),
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE ai_runs (
    id TEXT PRIMARY KEY,
    team_id TEXT REFERENCES tenants(id),
    site_id TEXT REFERENCES sites(id),
    actor_id TEXT REFERENCES users(id),
    actor_type TEXT NOT NULL DEFAULT '',
    feature TEXT NOT NULL,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    template_version TEXT NOT NULL DEFAULT '',
    evidence_ids_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(evidence_ids_json)),
    input_hash TEXT NOT NULL DEFAULT '',
    output_hash TEXT NOT NULL DEFAULT '',
    output_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(output_json)),
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    tool_call_count INTEGER NOT NULL DEFAULT 0,
    lifecycle_events_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(lifecycle_events_json)),
    status TEXT NOT NULL,
    error_category TEXT NOT NULL DEFAULT '',
    latency_ms INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX ai_runs_created_idx ON ai_runs(created_at);
CREATE INDEX ai_runs_feature_status_idx ON ai_runs(feature, status);
CREATE INDEX ai_runs_team_site_created_idx ON ai_runs(team_id, site_id, created_at);

CREATE TABLE opportunities (
    id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL REFERENCES tenants(id),
    site_id TEXT NOT NULL REFERENCES sites(id),
    kind TEXT NOT NULL,
    type_key TEXT NOT NULL,
    title_key TEXT NOT NULL,
    summary_key TEXT NOT NULL,
    action_key TEXT NOT NULL,
    digest_key TEXT NOT NULL DEFAULT '',
    copy_params_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(copy_params_json)),
    impact_value TEXT NOT NULL,
    impact_label_key TEXT NOT NULL,
    confidence TEXT NOT NULL,
    score INTEGER NOT NULL DEFAULT 0,
    score_breakdown_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(score_breakdown_json)),
    status TEXT NOT NULL,
    route_label_key TEXT NOT NULL DEFAULT '',
    route_params_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(route_params_json)),
    route_icon TEXT NOT NULL DEFAULT '',
    detector_version TEXT NOT NULL DEFAULT '',
    evidence_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(evidence_json)),
    cited_evidence_ids_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(cited_evidence_ids_json)),
    ai_run_id TEXT REFERENCES ai_runs(id),
    generated_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX opportunities_site_status_score_idx ON opportunities(site_id, status, score);
CREATE INDEX opportunities_team_site_updated_idx ON opportunities(team_id, site_id, updated_at);

CREATE TABLE custom_tracking_domains (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    hostname TEXT NOT NULL UNIQUE,
    verification_token TEXT NOT NULL,
    verification_status TEXT NOT NULL DEFAULT 'pending',
    target_status TEXT NOT NULL DEFAULT 'pending',
    tls_mode TEXT NOT NULL DEFAULT 'external',
    tls_status TEXT NOT NULL DEFAULT 'pending',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    last_error TEXT,
    verified_at TIMESTAMP,
    last_checked_at TIMESTAMP,
    last_tls_ask_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX custom_tracking_domains_hostname_idx ON custom_tracking_domains(hostname);
CREATE INDEX custom_tracking_domains_tenant_idx ON custom_tracking_domains(tenant_id);

CREATE TABLE traffic_exclusions (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL CHECK (scope IN ('instance', 'team', 'site')),
    tenant_id TEXT REFERENCES tenants(id),
    site_id TEXT REFERENCES sites(id),
    rule_type TEXT NOT NULL CHECK (rule_type IN ('cidr', 'country', 'user_agent', 'path')),
    cidr TEXT,
    country_code TEXT,
    user_agent TEXT,
    path TEXT,
    description TEXT,
    created_at TIMESTAMP NOT NULL,
    created_by TEXT REFERENCES users(id)
);

CREATE TABLE cloud_billing_accounts (
    tenant_id TEXT PRIMARY KEY REFERENCES tenants(id),
    plan_code TEXT NOT NULL,
    plan_name TEXT NOT NULL,
    subscription_status TEXT NOT NULL,
    stripe_customer_id TEXT UNIQUE,
    stripe_subscription_id TEXT UNIQUE,
    stripe_price_id TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    billing_interval TEXT DEFAULT 'monthly'
);

CREATE INDEX cloud_billing_accounts_plan_code_idx ON cloud_billing_accounts(plan_code);

CREATE TABLE cloud_billing_events (
    stripe_event_id TEXT PRIMARY KEY,
    tenant_id TEXT REFERENCES tenants(id),
    event_type TEXT NOT NULL,
    livemode INTEGER NOT NULL DEFAULT 0 CHECK (livemode IN (0, 1)),
    payload TEXT CHECK (payload IS NULL OR json_valid(payload)),
    processing_status TEXT NOT NULL,
    processing_error TEXT,
    processed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX cloud_billing_events_tenant_created_idx ON cloud_billing_events(tenant_id, created_at);

CREATE TABLE cloud_conversion_events (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    event_name TEXT NOT NULL,
    plan_code TEXT NOT NULL,
    billing_interval TEXT NOT NULL,
    dedupe_key TEXT NOT NULL UNIQUE,
    occurred_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL
);

CREATE INDEX cloud_conversion_events_name_time_idx ON cloud_conversion_events(event_name, occurred_at);
CREATE INDEX cloud_conversion_events_tenant_time_idx ON cloud_conversion_events(tenant_id, occurred_at);

CREATE TABLE cloud_lifecycle_messages (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    processing_error TEXT,
    sent_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, user_id, kind)
);

CREATE INDEX cloud_lifecycle_messages_status_idx ON cloud_lifecycle_messages(status);
CREATE INDEX cloud_lifecycle_messages_tenant_kind_idx ON cloud_lifecycle_messages(tenant_id, kind);

CREATE TABLE report_definitions (
    id TEXT PRIMARY KEY,
    tenant_id TEXT REFERENCES tenants(id),
    owner_user_id TEXT REFERENCES users(id),
    created_by TEXT REFERENCES users(id),
    name TEXT NOT NULL,
    scope TEXT NOT NULL CHECK (scope IN ('personal', 'team')),
    preset TEXT NOT NULL CHECK (preset IN ('site_summary', 'portfolio_digest', 'opportunity_brief')),
    site_mode TEXT NOT NULL DEFAULT 'selected' CHECK (site_mode IN ('selected', 'all_accessible')),
    frequency TEXT NOT NULL CHECK (frequency IN ('daily', 'weekly', 'monthly')),
    timezone TEXT NOT NULL,
    local_time TEXT NOT NULL,
    weekly_day INTEGER CHECK (weekly_day BETWEEN 0 AND 6),
    monthly_day INTEGER CHECK (monthly_day BETWEEN 1 AND 28),
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'paused')),
    next_run_at TIMESTAMP,
    consent_version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX report_definitions_owner_user_id_idx ON report_definitions(owner_user_id);
CREATE INDEX report_definitions_tenant_id_idx ON report_definitions(tenant_id);

CREATE TABLE report_definition_sites (
    id TEXT PRIMARY KEY,
    report_id TEXT NOT NULL REFERENCES report_definitions(id),
    site_id TEXT NOT NULL REFERENCES sites(id),
    tenant_id TEXT REFERENCES tenants(id),
    created_at TIMESTAMP NOT NULL,
    UNIQUE(report_id, site_id)
);

CREATE INDEX report_definition_sites_report_id_idx ON report_definition_sites(report_id);
CREATE INDEX report_definition_sites_site_id_idx ON report_definition_sites(site_id);

CREATE TABLE report_recipients (
    id TEXT PRIMARY KEY,
    report_id TEXT NOT NULL REFERENCES report_definitions(id),
    tenant_id TEXT REFERENCES tenants(id),
    user_id TEXT REFERENCES users(id),
    external_email TEXT,
    external_locale TEXT,
    consent_version INTEGER NOT NULL DEFAULT 1,
    confirmation_token_hash TEXT UNIQUE,
    confirmation_expires_at TIMESTAMP,
    confirmation_sent_at TIMESTAMP,
    confirmation_error_code TEXT,
    confirmed_at TIMESTAMP,
    unsubscribe_token_hash TEXT UNIQUE,
    opted_out_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE(report_id, user_id),
    UNIQUE(report_id, external_email)
);

CREATE INDEX report_recipients_report_id_idx ON report_recipients(report_id);
CREATE INDEX report_recipients_user_id_idx ON report_recipients(user_id);

CREATE TABLE report_runs (
    id TEXT PRIMARY KEY,
    report_id TEXT NOT NULL REFERENCES report_definitions(id),
    tenant_id TEXT REFERENCES tenants(id),
    scheduled_for TIMESTAMP NOT NULL,
    period_start TIMESTAMP NOT NULL,
    period_end TIMESTAMP NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'completed', 'partial', 'failed', 'skipped')),
    safe_error_code TEXT,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE(report_id, scheduled_for)
);

CREATE INDEX report_runs_report_id_idx ON report_runs(report_id, scheduled_for);

CREATE TABLE report_deliveries (
    id TEXT PRIMARY KEY,
    report_id TEXT NOT NULL REFERENCES report_definitions(id),
    run_id TEXT NOT NULL REFERENCES report_runs(id),
    tenant_id TEXT REFERENCES tenants(id),
    recipient_id TEXT NOT NULL REFERENCES report_recipients(id),
    recipient_kind TEXT NOT NULL CHECK (recipient_kind IN ('member', 'external')),
    status TEXT NOT NULL CHECK (status IN ('queued', 'sending', 'accepted', 'failed', 'skipped')),
    message_id TEXT NOT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMP,
    safe_error_code TEXT,
    smtp_accepted_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE(run_id, recipient_id)
);

CREATE INDEX report_deliveries_retry_idx ON report_deliveries(status, next_attempt_at);
CREATE INDEX report_deliveries_run_id_idx ON report_deliveries(run_id);

CREATE TABLE webhooks (
    id TEXT PRIMARY KEY,
    site_id TEXT REFERENCES sites(id),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    destination_url TEXT NOT NULL,
    secret TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX webhooks_site_id_idx ON webhooks(site_id);

CREATE TABLE webhook_event_subscriptions (
    webhook_id TEXT NOT NULL REFERENCES webhooks(id),
    event_type TEXT NOT NULL,
    PRIMARY KEY(webhook_id, event_type)
);

CREATE TABLE webhook_events (
    id TEXT PRIMARY KEY,
    site_id TEXT,
    event_type TEXT NOT NULL,
    api_version TEXT NOT NULL,
    payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
    occurred_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL
);

CREATE TABLE webhook_deliveries (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL,
    webhook_id TEXT NOT NULL,
    site_id TEXT,
    event_type TEXT NOT NULL,
    webhook_name TEXT NOT NULL,
    destination_url TEXT NOT NULL,
    signing_secret TEXT NOT NULL,
    payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
    status TEXT NOT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMP,
    last_attempt_at TIMESTAMP,
    completed_at TIMESTAMP,
    response_status INTEGER,
    last_error_code TEXT NOT NULL DEFAULT '',
    last_error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    dispatch_queued_at TIMESTAMP
);

CREATE INDEX webhook_deliveries_dispatch_due_idx ON webhook_deliveries(status, next_attempt_at, dispatch_queued_at);
CREATE INDEX webhook_deliveries_due_idx ON webhook_deliveries(status, next_attempt_at);
CREATE INDEX webhook_deliveries_site_created_idx ON webhook_deliveries(site_id, created_at);
CREATE INDEX webhook_deliveries_webhook_created_idx ON webhook_deliveries(webhook_id, created_at);

CREATE TABLE webhook_delivery_attempts (
    id TEXT PRIMARY KEY,
    delivery_id TEXT NOT NULL,
    site_id TEXT,
    attempt_number INTEGER NOT NULL,
    status TEXT NOT NULL,
    response_status INTEGER,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP NOT NULL,
    next_attempt_at TIMESTAMP
);

CREATE INDEX webhook_delivery_attempts_delivery_idx ON webhook_delivery_attempts(delivery_id, attempt_number);
CREATE INDEX webhook_delivery_attempts_site_idx ON webhook_delivery_attempts(site_id, completed_at);

CREATE TABLE site_deletion_webhook_outbox (
    delivery_id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL,
    source_site_id TEXT NOT NULL,
    webhook_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    api_version TEXT NOT NULL,
    webhook_name TEXT NOT NULL,
    destination_url TEXT NOT NULL,
    signing_secret TEXT NOT NULL,
    event_payload_json TEXT NOT NULL CHECK (json_valid(event_payload_json)),
    payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
    occurred_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL
);

CREATE INDEX site_deletion_webhook_outbox_event_idx ON site_deletion_webhook_outbox(event_id);
CREATE INDEX site_deletion_webhook_outbox_site_idx ON site_deletion_webhook_outbox(source_site_id, created_at);
