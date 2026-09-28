ALTER TABLE user_preferences
ADD COLUMN IF NOT EXISTS active_tenant_id UUID;

UPDATE user_preferences up
SET active_tenant_id = (
    SELECT tm.tenant_id
    FROM tenant_members tm
    WHERE tm.user_id = up.user_id
    ORDER BY
        CASE tm.role
            WHEN 'owner' THEN 0
            WHEN 'admin' THEN 1
            ELSE 2
        END ASC,
        tm.added_at ASC
    LIMIT 1
)
WHERE up.active_tenant_id IS NULL;
