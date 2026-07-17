package devtool

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMCPManifestUsesOneCentralLocallyBuiltLauncher(t *testing.T) {
	root := initTestRepository(t)
	launcher := filepath.Join(root, "hk")
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	launcher = filepath.Join(app.Root(), "hk")
	manifest, err := app.MCPManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != DeveloperMCPManifestSchemaVersion || manifest.Transport != "stdio" {
		t.Fatalf("unexpected manifest contract: %+v", manifest)
	}
	if manifest.ServerName != "hitkeep-dev" || manifest.Scope != "central" || manifest.WorkspaceRouting != "client-roots" || manifest.Delegation != "workspace-mcp" {
		t.Fatalf("manifest is not central, root-routed, and delegated: %+v", manifest)
	}
	if !reflect.DeepEqual(manifest.Notifications, []string{"progress", "logging"}) {
		t.Fatalf("manifest does not advertise streaming notifications: %+v", manifest)
	}
	wantArgs := []string{"mcp", "serve"}
	if manifest.Command != launcher || !reflect.DeepEqual(manifest.Args, wantArgs) {
		t.Fatalf("unexpected central launcher: command=%q args=%v", manifest.Command, manifest.Args)
	}
	definition, ok := manifest.ClientConfig.MCPServers[manifest.ServerName]
	if !ok || definition.Command != launcher || !reflect.DeepEqual(definition.Args, wantArgs) {
		t.Fatalf("copyable client configuration does not match manifest: %+v", manifest.ClientConfig)
	}
}

func TestMCPManifestRejectsMissingOrUnsafeLocalLauncher(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.MCPManifest(); err == nil {
		t.Fatal("missing hk launcher was accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "hk"), []byte("not executable\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.MCPManifest(); err == nil {
		t.Fatal("non-executable hk launcher was accepted")
	}
}
