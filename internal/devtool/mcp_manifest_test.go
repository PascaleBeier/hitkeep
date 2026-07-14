package devtool

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMCPManifestUsesStableIsolatedWorkspaceLauncher(t *testing.T) {
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
	root = app.workspace.Root
	launcher = filepath.Join(root, "hk")
	manifest, err := app.MCPManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != DeveloperMCPManifestSchemaVersion || manifest.Transport != "stdio" {
		t.Fatalf("unexpected manifest contract: %+v", manifest)
	}
	if manifest.ServerName != "hitkeep-dev-"+app.workspace.ID[:8] || manifest.WorkspaceID != app.workspace.ID {
		t.Fatalf("manifest is not workspace-specific: %+v", manifest)
	}
	if !manifest.Codex.Automatic || manifest.Codex.ServerName != "hitkeep-dev" || manifest.Codex.ConfigPath != filepath.Join(root, ".codex", "config.toml") || !manifest.Codex.RequiresTrustedProject {
		t.Fatalf("Codex project integration is not automatic and worktree-scoped: %+v", manifest.Codex)
	}
	if len(manifest.ProjectClients) != len(mcpProjectClientSpecs) {
		t.Fatalf("project client catalog mismatch: %+v", manifest.ProjectClients)
	}
	for index, client := range manifest.ProjectClients {
		spec := mcpProjectClientSpecs[index]
		if client.ClientID != spec.ID || client.ClientName != spec.Name || !client.Automatic || client.ConfigPath != filepath.Join(root, filepath.FromSlash(spec.ConfigPath)) || client.ServerName != spec.ServerName || client.RequiresTrustedProject != spec.RequiresTrustedProject {
			t.Fatalf("project integration does not match canonical spec: client=%+v spec=%+v", client, spec)
		}
	}
	wantArgs := []string{"--workspace", root, "mcp", "serve"}
	if manifest.Command != launcher || !reflect.DeepEqual(manifest.Args, wantArgs) {
		t.Fatalf("unexpected launcher: command=%q args=%v", manifest.Command, manifest.Args)
	}
	definition, ok := manifest.ClientConfig.MCPServers[manifest.ServerName]
	if !ok || definition.Command != launcher || !reflect.DeepEqual(definition.Args, wantArgs) {
		t.Fatalf("copyable client configuration does not match manifest: %+v", manifest.ClientConfig)
	}
}

func TestMCPManifestRejectsMissingOrUnsafeLauncher(t *testing.T) {
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
