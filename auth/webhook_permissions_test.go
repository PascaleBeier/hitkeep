package auth

import "testing"

func TestWebhookManagementPermissionsExcludeEditorsViewersAndAPIClientDelegation(t *testing.T) {
	t.Parallel()

	for _, role := range []InstanceRole{InstanceOwner, InstanceAdmin} {
		if !role.HasPermission(PermInstanceManageWebhooks) {
			t.Errorf("instance role %q must manage webhooks", role)
		}
	}
	if !InstanceAdmin.HasPermission(PermSiteManageWebhooks) {
		t.Fatal("instance admins must be able to manage site webhooks")
	}
	if InstanceUser.HasPermission(PermInstanceManageWebhooks) {
		t.Fatal("instance users must not manage instance webhooks")
	}

	for _, role := range []SiteRole{SiteOwner, SiteAdmin} {
		if !role.HasPermission(PermSiteManageWebhooks) {
			t.Errorf("site role %q must manage webhooks", role)
		}
	}
	for _, role := range []SiteRole{SiteEditor, SiteViewer} {
		if role.HasPermission(PermSiteManageWebhooks) {
			t.Errorf("site role %q must not manage webhooks", role)
		}
	}
}
