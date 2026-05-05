CREATE TABLE IF NOT EXISTS google_search_console_properties (
    team_id UUID NOT NULL,
    property_uri VARCHAR NOT NULL,
    permission_level VARCHAR DEFAULT '',
    last_seen_at TIMESTAMP DEFAULT now(),
    created_at TIMESTAMP DEFAULT now(),
    updated_at TIMESTAMP DEFAULT now(),
    PRIMARY KEY (team_id, property_uri)
);

CREATE TABLE IF NOT EXISTS google_search_console_site_mappings (
    site_id UUID PRIMARY KEY,
    team_id UUID NOT NULL,
    property_uri VARCHAR NOT NULL,
    mapped_by_user_id UUID,
    mapped_at TIMESTAMP DEFAULT now(),
    created_at TIMESTAMP DEFAULT now(),
    updated_at TIMESTAMP DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gsc_properties_team ON google_search_console_properties (team_id, property_uri);
CREATE INDEX IF NOT EXISTS idx_gsc_site_mappings_team ON google_search_console_site_mappings (team_id, property_uri);
