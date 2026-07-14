package devtool

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestRepositoryDevelopmentDocs(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	if err := ValidateDevelopmentDocs(root); err != nil {
		t.Fatal(err)
	}
}
