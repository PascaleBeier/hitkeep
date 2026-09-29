package devtool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncDocsScreenshotsCopiesDocsSetAndReadmeSubset(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "capture")
	docs := filepath.Join(root, "hitkeep-docs")
	readme := filepath.Join(root, "repo", ".github", "assets")
	for _, dir := range []string{source, docs} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"dashboard-overview.png", "settings-sites.png"} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	written, err := syncDocsScreenshots(source, readme, docs, []string{"dashboard-overview.png"})
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 3 {
		t.Fatalf("written = %v, want both docs images and one README image", written)
	}
	for _, path := range []string{filepath.Join(docs, "settings-sites.png"), filepath.Join(readme, "dashboard-overview.png")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(readme, "settings-sites.png")); !os.IsNotExist(err) {
		t.Fatal("only the README subset belongs in .github/assets")
	}

	if _, err := syncDocsScreenshots(source, readme, filepath.Join(root, "missing-docs"), []string{"mcp.png"}); err == nil {
		t.Fatal("a missing README screenshot must fail the sync")
	}
}
