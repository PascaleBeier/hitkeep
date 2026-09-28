package devtool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const artifactIndexVersion = "hk.dev/artifacts/v1"

type artifactIndex struct {
	SchemaVersion string          `json:"schema_version"`
	UpdatedAt     time.Time       `json:"updated_at"`
	Entries       []artifactEntry `json:"entries"`
}

type artifactEntry struct {
	Kind              string    `json:"kind"`
	WorkspaceID       string    `json:"workspace_id"`
	OwnerID           string    `json:"owner_id"`
	SourceFingerprint string    `json:"source_fingerprint,omitempty"`
	Path              string    `json:"path"`
	Size              int64     `json:"size"`
	Digest            string    `json:"digest,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	LastUsedAt        time.Time `json:"last_used_at"`
	ProtectedBy       []string  `json:"protected_by,omitempty"`
	State             string    `json:"state"`
}

type artifactMaintenanceResult struct {
	CheckedAt time.Time `json:"checked_at"`
	Removed   int       `json:"removed"`
	Bytes     int64     `json:"bytes"`
	Error     string    `json:"error,omitempty"`
}

func (a *App) artifactIndexPath() string {
	return filepath.Join(a.workspace.StateDir, "artifacts", "index.json")
}

func (a *App) registerArtifactPaths(kind, ownerID, sourceFingerprint string, paths []string) error {
	lock, err := lockStateRoot(a.workspace.StateDir, "artifact-index")
	if err != nil {
		return err
	}
	defer unlockStateRoot(lock)
	index, err := a.loadArtifactIndex()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, path := range paths {
		if strings.HasPrefix(path, "image://") {
			continue
		}
		clean, info, err := a.safeArtifactPath(path)
		if err != nil {
			return err
		}
		digest, err := artifactDigest(clean, info)
		if err != nil {
			return err
		}
		found := false
		for position := range index.Entries {
			if index.Entries[position].Path != clean {
				continue
			}
			index.Entries[position].Kind = kind
			index.Entries[position].OwnerID = ownerID
			index.Entries[position].SourceFingerprint = sourceFingerprint
			index.Entries[position].Size = info.Size()
			index.Entries[position].Digest = digest
			index.Entries[position].LastUsedAt = now
			index.Entries[position].State = "current"
			found = true
			break
		}
		if !found {
			index.Entries = append(index.Entries, artifactEntry{Kind: kind, WorkspaceID: a.workspace.ID, OwnerID: ownerID, SourceFingerprint: sourceFingerprint, Path: clean, Size: info.Size(), Digest: digest, CreatedAt: now, LastUsedAt: now, State: "current"})
		}
	}
	index.UpdatedAt = now
	return writeJSONAtomic(a.artifactIndexPath(), index)
}

func (a *App) loadArtifactIndex() (artifactIndex, error) {
	var index artifactIndex
	raw, err := os.ReadFile(a.artifactIndexPath())
	if err == nil {
		if decodeErr := json.Unmarshal(raw, &index); decodeErr == nil && index.SchemaVersion == artifactIndexVersion {
			return index, nil
		}
	}
	if err != nil && !os.IsNotExist(err) {
		return artifactIndex{}, err
	}
	index = artifactIndex{SchemaVersion: artifactIndexVersion, UpdatedAt: time.Now().UTC()}
	root := filepath.Join(a.workspace.StateDir, "artifacts")
	seen := 0
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || path == a.artifactIndexPath() {
			return nil
		}
		seen++
		if seen > 10_000 {
			return errors.New("artifact reconstruction limit exceeded")
		}
		clean, info, safeErr := a.safeArtifactPath(path)
		if safeErr != nil {
			return nil
		}
		index.Entries = append(index.Entries, artifactEntry{Kind: "reconstructed", WorkspaceID: a.workspace.ID, Path: clean, Size: info.Size(), CreatedAt: info.ModTime(), LastUsedAt: info.ModTime(), State: "current"})
		return nil
	})
	if walkErr != nil && !os.IsNotExist(walkErr) {
		return artifactIndex{}, walkErr
	}
	return index, nil
}

func (a *App) safeArtifactPath(path string) (string, os.FileInfo, error) {
	clean, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", nil, err
	}
	stateRoot, err := filepath.Abs(a.workspace.StateDir)
	if err != nil {
		return "", nil, err
	}
	relative, err := filepath.Rel(stateRoot, clean)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("artifact path escapes workspace state: %s", path)
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return "", nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("artifact path is a symlink: %s", path)
	}
	return clean, info, nil
}

func artifactDigest(path string, info os.FileInfo) (string, error) {
	if info.IsDir() {
		return "", nil
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (a *App) maintainArtifacts() artifactMaintenanceResult {
	result := artifactMaintenanceResult{CheckedAt: time.Now().UTC()}
	marker := filepath.Join(a.workspace.StateDir, "artifacts", ".last-maintenance")
	if info, err := os.Stat(marker); err == nil && time.Since(info.ModTime()) < time.Hour {
		return result
	}
	index, err := a.loadArtifactIndex()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	sort.SliceStable(index.Entries, func(i, j int) bool { return index.Entries[i].CreatedAt.After(index.Entries[j].CreatedAt) })
	keptScreenshots := 0
	for position := range index.Entries {
		entry := &index.Entries[position]
		if entry.Kind == "screenshot" {
			keptScreenshots++
		}
		if result.Removed >= 20 || len(entry.ProtectedBy) > 0 || entry.State != "current" || entry.Kind != "screenshot" || keptScreenshots <= 10 || time.Since(entry.CreatedAt) < 7*24*time.Hour {
			continue
		}
		clean, info, safeErr := a.safeArtifactPath(entry.Path)
		if safeErr != nil {
			continue
		}
		quarantine := clean + ".quarantine"
		if renameErr := os.Rename(clean, quarantine); renameErr != nil {
			continue
		}
		if removeErr := os.RemoveAll(quarantine); removeErr != nil {
			_ = os.Rename(quarantine, clean)
			continue
		}
		entry.State = "stale"
		result.Removed++
		result.Bytes += info.Size()
	}
	index.UpdatedAt = result.CheckedAt
	if err := writeJSONAtomic(a.artifactIndexPath(), index); err != nil {
		result.Error = err.Error()
	}
	_ = os.WriteFile(marker, []byte(result.CheckedAt.Format(time.RFC3339Nano)), 0o600)
	return result
}
