package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"hitkeep/internal/devtool"
)

func TestProductionBuildsExcludeDeveloperPlatform(t *testing.T) {
	root := repositoryRoot(t)
	app, err := devtool.NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.ValidateProductionBoundary(context.Background()); err != nil {
		t.Fatal(err)
	}

	command := exec.Command("go", "list", "-deps", "./cmd/hk")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		t.Fatalf("inspect hk dependencies: %v", err)
	}
	dependencies := "\n" + string(output)
	for _, expected := range []string{
		"hitkeep/internal/devtool",
		"hitkeep/internal/devtool/cli",
		"hitkeep/internal/devtool/devmcp",
	} {
		if !strings.Contains(dependencies, "\n"+expected+"\n") {
			t.Errorf("hk dependency graph does not contain %s", expected)
		}
	}
}

func TestDockerBootstrapUsesWritableHostCaches(t *testing.T) {
	root := repositoryRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "hk"))
	if err != nil {
		t.Fatal(err)
	}
	launcher := string(raw)
	for _, required := range []string{
		"GOMODCACHE=/cache/go-mod",
		"GOCACHE=/cache/go-build",
		"$bootstrap_mod_cache:/cache/go-mod",
		"$bootstrap_build_cache:/cache/go-build",
		"! -name '*_test.go'",
	} {
		if !strings.Contains(launcher, required) {
			t.Fatalf("Docker bootstrap is missing writable cache configuration %q", required)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			break
		}
		root = parent
	}
	t.Fatal("resolve repository root")
	return ""
}
