package devtool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDoctorRequiresDockerComposeForDevelopment(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test\n\ngo 1.26.6\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dashboard := filepath.Join(root, "frontend", "dashboard")
	if err := os.MkdirAll(dashboard, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dashboard, ".node-version"), []byte("24.19.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dashboard, "package.json"), []byte(`{"packageManager":"npm@12.0.2"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	fakeBin := t.TempDir()
	commands := map[string]string{
		"git":  "git version 2.50.0",
		"go":   "go version go1.26.6 test/arch",
		"node": "v24.19.0",
		"npm":  "12.0.2",
		"cc":   "cc 1.0",
	}
	for name, output := range commands {
		script := "#!/bin/sh\nprintf '%s\\n' '" + output + "'\n"
		if err := os.WriteFile(filepath.Join(fakeBin, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", fakeBin)
	report := app.Doctor(context.Background())
	if report.Ready {
		t.Fatalf("doctor reported development ready without Docker Compose: %+v", report)
	}
	if report.Capabilities.ContainerDevelopment {
		t.Fatalf("container development was reported without Docker Compose: %+v", report.Capabilities)
	}
}

func TestDoctorBoundsSlowChecksAndRunsThemInParallel(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	fakeBin := t.TempDir()
	if err := os.WriteFile(filepath.Join(fakeBin, "docker"), []byte("#!/bin/sh\n/bin/sleep 10\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin)
	started := time.Now()
	report := app.Doctor(context.Background())
	if elapsed := time.Since(started); elapsed > 7*time.Second {
		t.Fatalf("parallel bounded doctor took %s", elapsed)
	}
	for _, name := range []string{"docker", "compose", "buildx"} {
		found := false
		for _, check := range report.Checks {
			if check.Name == name {
				found = true
				if check.Status != "unavailable" || !strings.Contains(check.Detected, "timed out") {
					t.Fatalf("%s check was not bounded: %+v", name, check)
				}
			}
		}
		if !found {
			t.Fatalf("missing %s check", name)
		}
	}
}

func TestDoctorUsesManagedToolchainsWithoutHostGoOrNode(t *testing.T) {
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	root := initTestRepository(t)
	writeTestToolchainConfig(t, root)
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	config, err := app.ToolchainConfig()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := app.managedToolchainPaths()
	if err != nil {
		t.Fatal(err)
	}
	managedCommands := map[string]string{
		paths.GoExecutable:   "go version go" + config.Go + " test/arch",
		paths.NodeExecutable: "v" + config.Node,
		paths.NPMExecutable:  config.NPM,
	}
	for path, output := range managedCommands {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' '"+output+"'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	fakeBin := t.TempDir()
	hostCommands := map[string]string{
		"git":           "git version 2.50.0",
		"cc":            "cc 1.0",
		"docker":        "27.0.0",
		"golangci-lint": ToolVersion("golangci-lint"),
		"zizmor":        ToolVersion("zizmor"),
	}
	for name, output := range hostCommands {
		if err := os.WriteFile(filepath.Join(fakeBin, name), []byte("#!/bin/sh\nprintf '%s\\n' '"+output+"'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", fakeBin)
	report := app.Doctor(context.Background())
	if !report.Ready {
		t.Fatalf("managed toolchains did not preserve developer readiness without host Go or Node: %+v", report)
	}
	for _, name := range []string{"go", "node", "npm"} {
		found := false
		for _, check := range report.Checks {
			if check.Name == name {
				found = true
				if check.Status != "ok" {
					t.Fatalf("managed %s check failed without a host installation: %+v", name, check)
				}
			}
		}
		if !found {
			t.Fatalf("managed %s check is missing: %+v", name, report)
		}
	}
}

func TestCommandEnvironmentPrefersManagedToolchains(t *testing.T) {
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	root := initTestRepository(t)
	writeTestToolchainConfig(t, root)
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/host/bin")
	paths, err := app.managedToolchainPaths()
	if err != nil {
		t.Fatal(err)
	}
	for _, executable := range []string{paths.GoExecutable, paths.NodeExecutable} {
		if err := os.MkdirAll(filepath.Dir(executable), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	environment := app.commandEnvironment(nil)
	wantPath := strings.Join([]string{filepath.Dir(paths.GoExecutable), filepath.Dir(paths.NodeExecutable), "/host/bin"}, string(os.PathListSeparator))
	if got := environmentValue(environment, "PATH"); got != wantPath {
		t.Fatalf("managed toolchains were not preferred in PATH: got %q want %q", got, wantPath)
	}
	for name, want := range map[string]string{
		"GOCACHE":                  paths.GoBuildCache,
		"GOMODCACHE":               paths.GoModuleCache,
		"NPM_CONFIG_CACHE":         paths.NPMCache,
		"PLAYWRIGHT_BROWSERS_PATH": paths.PlaywrightCache,
	} {
		if got := environmentValue(environment, name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

func writeTestToolchainConfig(t *testing.T, root string) {
	t.Helper()
	for path, content := range map[string]string{
		"go.mod":                           "module example.test\n\ngo 1.26.6\n",
		"frontend/dashboard/.node-version": "24.19.0\n",
		"frontend/dashboard/package.json":  `{"packageManager":"npm@12.0.2"}`,
	} {
		absolute := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCatalogUsesWorkspaceScopedLocalImages(t *testing.T) {
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	first, err := NewApp(initTestRepository(t))
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewApp(initTestRepository(t))
	if err != nil {
		t.Fatal(err)
	}
	firstCatalog := first.Catalog()
	secondCatalog := second.Catalog()
	for index := range firstCatalog.Variants {
		firstImage := firstCatalog.Variants[index].LocalImage
		secondImage := secondCatalog.Variants[index].LocalImage
		if firstImage == secondImage {
			t.Fatalf("local image collided across workspaces: %s", firstImage)
		}
		if !strings.HasSuffix(firstImage, first.workspace.ID[:8]) || !strings.HasSuffix(secondImage, second.workspace.ID[:8]) {
			t.Fatalf("local images do not identify their workspaces: %s %s", firstImage, secondImage)
		}
	}
	variant, err := VariantByID("self-hosted")
	if err != nil {
		t.Fatal(err)
	}
	firstCache := environmentValue(first.ComposeEnvironment(variant), "HITKEEP_FRONTEND_CACHE_DIR")
	secondCache := environmentValue(second.ComposeEnvironment(variant), "HITKEEP_FRONTEND_CACHE_DIR")
	if firstCache == secondCache || !pathWithin(first.workspace.StateDir, firstCache) || !pathWithin(second.workspace.StateDir, secondCache) {
		t.Fatalf("mutable frontend caches are not workspace-confined: %q %q", firstCache, secondCache)
	}
}

func TestComposeEnvironmentExposesSocialProvidersInLocalDevelopment(t *testing.T) {
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(initTestRepository(t))
	if err != nil {
		t.Fatal(err)
	}
	variant, err := VariantByID("self-hosted")
	if err != nil {
		t.Fatal(err)
	}

	environment := app.ComposeEnvironment(variant)
	for _, name := range []string{
		"HITKEEP_SOCIAL_GOOGLE_CLIENT_ID",
		"HITKEEP_SOCIAL_GOOGLE_CLIENT_SECRET",
		"HITKEEP_SOCIAL_GITHUB_CLIENT_ID",
		"HITKEEP_SOCIAL_GITHUB_CLIENT_SECRET",
		"HITKEEP_SOCIAL_MICROSOFT_CLIENT_ID",
		"HITKEEP_SOCIAL_MICROSOFT_CLIENT_SECRET",
	} {
		if value := environmentValue(environment, name); value == "" {
			t.Fatalf("local development environment does not configure %s", name)
		}
	}
	if value := environmentValue(environment, "HITKEEP_DB_AUTO_RECOVER_WAL"); value != "true" {
		t.Fatalf("local development environment does not enable controlled WAL recovery: %q", value)
	}
}

func environmentValue(environment []string, name string) string {
	prefix := name + "="
	for _, entry := range environment {
		if value, ok := strings.CutPrefix(entry, prefix); ok {
			return value
		}
	}
	return ""
}
