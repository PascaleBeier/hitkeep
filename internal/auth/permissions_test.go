package auth

import "testing"

func TestMinInstanceRoleReturnsLeastPrivilegedRole(t *testing.T) {
	tests := []struct {
		name string
		a    InstanceRole
		b    InstanceRole
		want InstanceRole
	}{
		{name: "owner constrained to user", a: InstanceOwner, b: InstanceUser, want: InstanceUser},
		{name: "admin constrained to user", a: InstanceAdmin, b: InstanceUser, want: InstanceUser},
		{name: "owner constrained to admin", a: InstanceOwner, b: InstanceAdmin, want: InstanceAdmin},
		{name: "same role remains same", a: InstanceAdmin, b: InstanceAdmin, want: InstanceAdmin},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MinInstanceRole(tc.a, tc.b); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
			if got := MinInstanceRole(tc.b, tc.a); got != tc.want {
				t.Fatalf("expected symmetric result %q, got %q", tc.want, got)
			}
		})
	}
}

func TestMinSiteRoleReturnsLeastPrivilegedRole(t *testing.T) {
	tests := []struct {
		name string
		a    SiteRole
		b    SiteRole
		want SiteRole
	}{
		{name: "owner constrained to viewer", a: SiteOwner, b: SiteViewer, want: SiteViewer},
		{name: "admin constrained to editor", a: SiteAdmin, b: SiteEditor, want: SiteEditor},
		{name: "editor constrained to viewer", a: SiteEditor, b: SiteViewer, want: SiteViewer},
		{name: "same role remains same", a: SiteAdmin, b: SiteAdmin, want: SiteAdmin},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MinSiteRole(tc.a, tc.b); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
			if got := MinSiteRole(tc.b, tc.a); got != tc.want {
				t.Fatalf("expected symmetric result %q, got %q", tc.want, got)
			}
		})
	}
}
