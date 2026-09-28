CREATE TABLE traffic_exclusions (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    scope VARCHAR NOT NULL,
    tenant_id UUID REFERENCES tenants(id),
    site_id UUID REFERENCES sites(id),
    rule_type VARCHAR NOT NULL,
    cidr VARCHAR,
    country_code VARCHAR,
    user_agent VARCHAR,
    path VARCHAR,
    description VARCHAR,
    created_at TIMESTAMPTZ NOT NULL,
    created_by UUID REFERENCES users(id),
    CHECK (
        (scope = 'instance' AND tenant_id IS NULL AND site_id IS NULL)
        OR (scope = 'team' AND tenant_id IS NOT NULL AND site_id IS NULL)
        OR (scope = 'site' AND tenant_id IS NULL AND site_id IS NOT NULL)
    ),
    CHECK (
        (rule_type = 'cidr' AND cidr IS NOT NULL AND country_code IS NULL AND user_agent IS NULL AND path IS NULL)
        OR (rule_type = 'country' AND cidr IS NULL AND country_code IS NOT NULL AND user_agent IS NULL AND path IS NULL)
        OR (rule_type = 'user_agent' AND cidr IS NULL AND country_code IS NULL AND user_agent IS NOT NULL AND path IS NULL)
        OR (rule_type = 'path' AND cidr IS NULL AND country_code IS NULL AND user_agent IS NULL AND path IS NOT NULL)
    )
);

INSERT INTO traffic_exclusions (id, scope, rule_type, cidr, description, created_at, created_by)
SELECT id, 'instance', 'cidr', cidr, description, created_at, created_by
FROM instance_exclusions;

INSERT INTO traffic_exclusions (id, scope, site_id, rule_type, cidr, description, created_at, created_by)
SELECT id, 'site', site_id, 'cidr', cidr, description, created_at, created_by
FROM site_exclusions;

INSERT INTO traffic_exclusions (id, scope, rule_type, country_code, description, created_at, created_by)
SELECT id, 'instance', 'country', country_code, description, created_at, created_by
FROM instance_country_exclusions;

INSERT INTO traffic_exclusions (id, scope, site_id, rule_type, country_code, description, created_at, created_by)
SELECT id, 'site', site_id, 'country', country_code, description, created_at, created_by
FROM site_country_exclusions;

DROP TABLE instance_exclusions;
DROP TABLE site_exclusions;
DROP TABLE instance_country_exclusions;
DROP TABLE site_country_exclusions;
