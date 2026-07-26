CREATE TABLE IF NOT EXISTS data_migrations (
    name       VARCHAR PRIMARY KEY,
    applied_at TIMESTAMP NOT NULL
);

-- Sites without an explicit mapping have always resolved to the default
-- tenant. Materialize that fallback before the data-plane split so the
-- migration and subsequent runtime routing share one closed-world scope.
INSERT INTO site_tenants (site_id, tenant_id)
SELECT
    s.id,
    (SELECT id FROM tenants WHERE is_default = TRUE LIMIT 1)
FROM sites s
LEFT JOIN site_tenants st ON st.site_id = s.id
WHERE st.site_id IS NULL
  AND EXISTS (SELECT 1 FROM tenants WHERE is_default = TRUE);
