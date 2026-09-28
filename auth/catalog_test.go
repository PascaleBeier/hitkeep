package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTypeScriptCapabilityCatalogIncludesEveryBackendCapability(t *testing.T) {
	rendered := RenderTypeScriptCapabilities()

	for _, entry := range append(append(InstanceCapabilityCatalog(), SiteCapabilityCatalog()...), TeamCapabilityCatalog()...) {
		if !strings.Contains(rendered, entry.Value) {
			t.Fatalf("generated TypeScript is missing capability %q (%s)", entry.Key, entry.Value)
		}
	}

	for _, role := range []InstanceRole{InstanceOwner, InstanceAdmin, InstanceUser} {
		for _, capability := range InstanceCapabilities(role) {
			if !strings.Contains(rendered, capability) {
				t.Fatalf("generated TypeScript is missing instance role %q capability %q", role, capability)
			}
		}
	}

	for _, role := range []SiteRole{SiteOwner, SiteAdmin, SiteEditor, SiteViewer} {
		for _, capability := range SiteCapabilities(role) {
			if !strings.Contains(rendered, capability) {
				t.Fatalf("generated TypeScript is missing site role %q capability %q", role, capability)
			}
		}
	}

	for _, role := range []string{"owner", "admin", "member"} {
		for _, capability := range TeamCapabilities(role) {
			if !strings.Contains(rendered, capability) {
				t.Fatalf("generated TypeScript is missing team role %q capability %q", role, capability)
			}
		}
	}
}

func TestGeneratedTypeScriptCapabilitiesAreCurrent(t *testing.T) {
	want := RenderTypeScriptCapabilities()
	path := filepath.Join("..", GeneratedTypeScriptCapabilitiesPath)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated TypeScript capability file: %v", err)
	}
	if string(got) != want {
		t.Fatalf("%s is out of date; run go run ./cmd/auth-capabilities --write", GeneratedTypeScriptCapabilitiesPath)
	}
}
