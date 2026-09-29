package devtool

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractTarWritesFilesAndRejectsEscapes(t *testing.T) {
	archive := func(name, content string) *bytes.Buffer {
		var buffer bytes.Buffer
		writer := tar.NewWriter(&buffer)
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return &buffer
	}

	dir := t.TempDir()
	if err := extractTar(archive("cmd/email-preview/main.go", "package main\n"), dir); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "cmd", "email-preview", "main.go")); err != nil || string(data) != "package main\n" {
		t.Fatalf("extracted file = %q, %v", data, err)
	}

	if err := extractTar(archive("../escape.txt", "x"), dir); err == nil {
		t.Fatal("extractTar must reject paths outside the target directory")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("escape.txt was written outside the target: %v", err)
	}
}
