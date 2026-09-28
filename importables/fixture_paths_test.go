package importables

import (
	"os"
	"path/filepath"
	"testing"
)

func repoFixturePath(t *testing.T, parts ...string) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve fixture working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return filepath.Join(append([]string{root}, parts...)...)
		}
		parent := filepath.Dir(root)
		if parent == root {
			break
		}
		root = parent
	}
	t.Fatal("resolve repository root for fixtures")
	return ""
}
