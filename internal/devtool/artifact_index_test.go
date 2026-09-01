package devtool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArtifactIndexRejectsEscapesAndReconstructs(t *testing.T) {
	stateDir := t.TempDir()
	app := &App{workspace: Workspace{ID: "workspace", StateDir: stateDir}}
	artifact := filepath.Join(stateDir, "artifacts", "screenshots", "capture", "image.png")
	if err := os.MkdirAll(filepath.Dir(artifact), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := app.registerArtifactPaths("screenshot", "capture", "source", []string{artifact}); err != nil {
		t.Fatal(err)
	}
	index, err := app.loadArtifactIndex()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Entries) != 1 || index.Entries[0].Digest == "" || index.Entries[0].Path != artifact {
		t.Fatalf("unexpected artifact index: %+v", index)
	}

	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(stateDir, "artifacts", "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if err := app.registerArtifactPaths("screenshot", "escape", "source", []string{link}); err == nil {
		t.Fatal("symlink escape was indexed")
	}

	if err := os.Remove(app.artifactIndexPath()); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := app.loadArtifactIndex()
	if err != nil {
		t.Fatal(err)
	}
	if len(rebuilt.Entries) == 0 {
		t.Fatal("missing index was not reconstructed")
	}
}

func TestRegisterArtifactPathsSkipsVirtualImageArtifacts(t *testing.T) {
	stateDir := t.TempDir()
	app := &App{workspace: Workspace{ID: "workspace", StateDir: stateDir}}
	log := filepath.Join(stateDir, "runs", "build.log")
	if err := os.MkdirAll(filepath.Dir(log), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(log, []byte("log"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := app.registerArtifactPaths("run", "build", "source", []string{log, "image://hitkeep:local"}); err != nil {
		t.Fatal(err)
	}
	index, err := app.loadArtifactIndex()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Entries) != 1 || index.Entries[0].Path != log {
		t.Fatalf("unexpected indexed artifacts: %+v", index.Entries)
	}
}
