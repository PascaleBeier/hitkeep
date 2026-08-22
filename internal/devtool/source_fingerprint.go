package devtool

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// EmbeddedDeveloperSourceFingerprint is set by the root launcher when it builds hk.
var EmbeddedDeveloperSourceFingerprint string

type DeveloperServerStaleError struct {
	Expected string
	Current  string
}

func (err *DeveloperServerStaleError) Error() string {
	return fmt.Sprintf("developer_server_stale: central hk source %s does not match workspace source %s; reload the registered MCP host", err.Current, err.Expected)
}

func DeveloperSourceFingerprint(root string) (string, error) {
	var lines []string
	for _, directory := range []string{"cmd/hk", "internal/devtool"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			name := entry.Name()
			if !(strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")) {
				return nil
			}
			return appendFingerprintLine(root, path, &lines)
		})
		if err != nil {
			return "", err
		}
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		if err := appendFingerprintLine(root, filepath.Join(root, name), &lines); err != nil {
			return "", err
		}
	}
	slices.Sort(lines)
	outer := sha256.Sum256([]byte(strings.Join(lines, "\n") + "\n"))
	return hex.EncodeToString(outer[:]), nil
}

func appendFingerprintLine(root, path string, lines *[]string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(content)
	*lines = append(*lines, hex.EncodeToString(digest[:])+"  "+filepath.ToSlash(relative))
	return nil
}

func (a *App) validateDevWorkerProtocol() error {
	if os.Getenv("HK_CHILD_DEV") != "1" {
		return fmt.Errorf("development worker is internal")
	}
	if os.Getenv("HK_EXPECTED_SCHEMA") != SchemaVersion || os.Getenv("HK_WORKER_PROTOCOL") != "1" {
		return fmt.Errorf("development worker protocol mismatch")
	}
	if os.Getenv("HK_WORKSPACE_ID") != a.workspace.ID {
		return fmt.Errorf("development worker workspace mismatch")
	}
	if os.Getenv("HK_SOURCE_FINGERPRINT") == "legacy" {
		return nil
	}
	fingerprint, err := DeveloperSourceFingerprint(a.workspace.Root)
	if err != nil {
		return err
	}
	if os.Getenv("HK_SOURCE_FINGERPRINT") != fingerprint {
		return fmt.Errorf("development worker source fingerprint mismatch")
	}
	return nil
}

func (a *App) validateWorkerProtocol() error {
	if os.Getenv("HK_CHILD_RUN") != "1" {
		return fmt.Errorf("worker protocol requires HK_CHILD_RUN=1")
	}
	if os.Getenv("HK_EXPECTED_SCHEMA") != SchemaVersion {
		return fmt.Errorf("worker schema mismatch")
	}
	if os.Getenv("HK_WORKER_PROTOCOL") != "1" {
		return fmt.Errorf("worker protocol mismatch")
	}
	if os.Getenv("HK_WORKSPACE_ID") != a.workspace.ID {
		return fmt.Errorf("worker workspace mismatch")
	}
	if os.Getenv("HK_SOURCE_FINGERPRINT") == "legacy" {
		return nil
	}
	fingerprint, err := DeveloperSourceFingerprint(a.workspace.Root)
	if err != nil {
		return err
	}
	if os.Getenv("HK_SOURCE_FINGERPRINT") != fingerprint {
		return fmt.Errorf("worker source fingerprint mismatch")
	}
	return nil
}

func VerifyDeveloperSource(root string) error {
	if EmbeddedDeveloperSourceFingerprint == "" {
		return nil
	}
	expected, err := DeveloperSourceFingerprint(root)
	if err != nil {
		return fmt.Errorf("fingerprint developer source: %w", err)
	}
	if expected != EmbeddedDeveloperSourceFingerprint {
		return &DeveloperServerStaleError{Expected: expected, Current: EmbeddedDeveloperSourceFingerprint}
	}
	return nil
}
