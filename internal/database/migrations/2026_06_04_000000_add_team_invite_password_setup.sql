ALTER TABLE team_invites ADD COLUMN requires_password_setup BOOLEAN;

UPDATE team_invites
SET requires_password_setup = FALSE
WHERE requires_password_setup IS NULL;

UPDATE team_invites AS ti
SET requires_password_setup = TRUE
WHERE ti.status = 'pending'
  AND ti.invited_user_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1
      FROM tenant_members AS tm
      WHERE tm.user_id = ti.invited_user_id
  );
