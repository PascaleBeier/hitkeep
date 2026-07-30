package devtool

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestNPMCommandUsesCanonicalInstalledBinary(t *testing.T) {
	want := []string{"npm", "run", "build:prod"}
	if got := npmCommand("run", "build:prod"); !slices.Equal(got, want) {
		t.Fatalf("npm command = %v, want %v", got, want)
	}
}

func TestReleaseArtifactChecksumsAndPublicImageBoundary(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"hitkeep-cloud-linux-amd64", "hitkeep-cloud-linux-arm64",
		"hitkeep-linux-amd64", "hitkeep-linux-arm64",
		"hitkeep-configuration.json",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	generated, err := app.GenerateReleaseChecksums()
	if err != nil {
		t.Fatal(err)
	}
	if generated.Count != 6 {
		t.Fatalf("generated artifact count = %d, want 6", generated.Count)
	}
	if _, err := app.VerifyReleaseArtifacts(); err != nil {
		t.Fatal(err)
	}
	prepared, err := app.PreparePublicImageContext()
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Count != 2 {
		t.Fatalf("public image artifact count = %d, want 2", prepared.Count)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		if _, err := os.Stat(filepath.Join(root, "hitkeep-cloud-linux-"+arch)); !os.IsNotExist(err) {
			t.Fatalf("cloud binary remains in public context: %v", err)
		}
		quarantined := filepath.Join(app.workspace.StateDir, "artifacts", "cloud-release-binaries", "hitkeep-cloud-linux-"+arch)
		if _, err := os.Stat(quarantined); err != nil {
			t.Fatalf("cloud binary was deleted instead of relocated: %v", err)
		}
	}
}

func TestDashboardArchiveIsDeterministicAndConfined(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(filepath.Join(source, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "index.html"), []byte("<h1>HitKeep</h1>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "assets", "app.js"), []byte("console.log('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(t.TempDir(), "first.tar.gz")
	second := filepath.Join(t.TempDir(), "second.tar.gz")
	if _, _, err := writeDeterministicTarGzip(source, first); err != nil {
		t.Fatal(err)
	}
	if _, _, err := writeDeterministicTarGzip(source, second); err != nil {
		t.Fatal(err)
	}
	firstDigest, _ := fileSHA256(first)
	secondDigest, _ := fileSHA256(second)
	if firstDigest != secondDigest {
		t.Fatalf("dashboard archives are not deterministic: %s != %s", firstDigest, secondDigest)
	}
	destination := filepath.Join(t.TempDir(), "public")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	count, _, err := extractPublicAssets(first, destination)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("restored asset count = %d, want 2", count)
	}
	if raw, err := os.ReadFile(filepath.Join(destination, "assets", "app.js")); err != nil || string(raw) != "console.log('ok')\n" {
		t.Fatalf("restored asset mismatch: %v: %q", err, raw)
	}
}
