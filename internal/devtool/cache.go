package devtool

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
)

const defaultCacheMaxAge = 30 * 24 * time.Hour

func (a *App) CacheStatus() (CacheReport, error) {
	root := filepath.Dir(filepath.Dir(a.workspace.StateDir))
	report := CacheReport{Root: root}
	linkedSnapshots := activeFrontendSnapshots(root)

	binaries, err := cacheChildren(filepath.Join(root, "bin"), "hk-binary", func(entry os.DirEntry) bool {
		return !entry.IsDir() && strings.HasPrefix(entry.Name(), "hk-") && !strings.Contains(entry.Name(), ".tmp")
	})
	if err != nil {
		return report, err
	}
	slices.SortFunc(binaries, func(left, right CacheEntry) int { return right.LastUsedAt.Compare(left.LastUsedAt) })
	for index := range binaries {
		binaries[index].InUse = sameFilePath(binaries[index].Path, a.executable) || index < 3
		binaries[index].Prunable = !binaries[index].InUse
	}
	report.Entries = append(report.Entries, binaries...)

	snapshots, err := cacheChildren(filepath.Join(root, "shared", "frontend"), "frontend-snapshot", func(entry os.DirEntry) bool {
		return entry.IsDir() && !strings.HasSuffix(entry.Name(), ".lock")
	})
	if err != nil {
		return report, err
	}
	for index := range snapshots {
		snapshots[index].InUse = linkedSnapshots[filepath.Clean(snapshots[index].Path)]
		snapshots[index].Prunable = !snapshots[index].InUse
	}
	report.Entries = append(report.Entries, snapshots...)

	toolchains, err := cacheChildren(filepath.Join(root, "shared", "toolchains"), "managed-toolchain", func(entry os.DirEntry) bool {
		return entry.IsDir() && !strings.HasPrefix(entry.Name(), ".")
	})
	if err != nil {
		return report, err
	}
	managedPaths, managedPathsErr := a.managedToolchainPaths()
	for index := range toolchains {
		toolchains[index].InUse = managedPathsErr == nil && (sameFilePath(toolchains[index].Path, managedPaths.GoRoot) || sameFilePath(toolchains[index].Path, managedPaths.NodeRoot))
		toolchains[index].Prunable = !toolchains[index].InUse
	}
	report.Entries = append(report.Entries, toolchains...)

	for _, name := range []string{"go-build", "go-mod", "npm", "playwright"} {
		path := filepath.Join(root, "shared", name)
		entry, statErr := cachePath(path, "managed-cache", name)
		if statErr == nil {
			entry.InUse = true
			report.Entries = append(report.Entries, entry)
		} else if !os.IsNotExist(statErr) {
			return report, statErr
		}
	}

	workspaces, err := cacheChildren(filepath.Join(root, "workspaces"), "workspace-state", func(entry os.DirEntry) bool { return entry.IsDir() })
	if err != nil {
		return report, err
	}
	for index := range workspaces {
		lease, loadErr := loadWorkspace(filepath.Join(workspaces[index].Path, "workspace.json"))
		active := len(runSummariesFromState(workspaces[index].Path, true, 1)) > 0
		alive := loadErr == nil && workspaceRootAlive(lease.Root)
		workspaces[index].InUse = workspaces[index].Key == a.workspace.ID || alive || active
		workspaces[index].Prunable = !workspaces[index].InUse
	}
	report.Entries = append(report.Entries, workspaces...)

	slices.SortFunc(report.Entries, func(left, right CacheEntry) int {
		if left.Kind != right.Kind {
			return strings.Compare(left.Kind, right.Kind)
		}
		return strings.Compare(left.Key, right.Key)
	})
	for _, entry := range report.Entries {
		report.TotalBytes += entry.Bytes
	}
	return report, nil
}

func (a *App) PruneCache(olderThan time.Duration, apply bool) (CachePruneResult, error) {
	if olderThan <= 0 {
		olderThan = defaultCacheMaxAge
	}
	report, err := a.CacheStatus()
	if err != nil {
		return CachePruneResult{}, err
	}
	return a.pruneCacheReport(report, olderThan, apply)
}

func (a *App) pruneCacheReport(report CacheReport, olderThan time.Duration, apply bool) (CachePruneResult, error) {
	result := CachePruneResult{DryRun: !apply, OlderThan: olderThan.String()}
	cutoff := time.Now().Add(-olderThan)
	for _, entry := range report.Entries {
		if entry.Prunable && entry.LastUsedAt.Before(cutoff) {
			result.Candidates = append(result.Candidates, entry)
			result.CandidateBytes += entry.Bytes
		}
	}
	if !apply {
		return result, nil
	}
	lock, err := lockStateRoot(report.Root, "cache-prune")
	if err != nil {
		return result, err
	}
	defer unlockStateRoot(lock)
	for _, entry := range result.Candidates {
		if !pathWithin(report.Root, entry.Path) {
			return result, fmt.Errorf("refuse cache path outside managed root: %s", entry.Path)
		}
		release := func() {}
		removalPath := entry.Path
		if entry.Kind == "frontend-snapshot" {
			var acquired bool
			release, acquired = trySnapshotLock(entry.Path + ".lock")
			if !acquired {
				continue
			}
			if activeFrontendSnapshots(report.Root)[filepath.Clean(entry.Path)] {
				release()
				continue
			}
			removalPath = fmt.Sprintf("%s.prune-%d-%d", entry.Path, os.Getpid(), time.Now().UnixNano())
			if renameErr := os.Rename(entry.Path, removalPath); renameErr != nil {
				release()
				if os.IsNotExist(renameErr) {
					continue
				}
				return result, fmt.Errorf("retire managed cache %s: %w", entry.Path, renameErr)
			}
			release()
			release = func() {}
			if err := makeTreeOwnerWritable(removalPath); err != nil {
				return result, fmt.Errorf("prepare managed cache %s for removal: %w", entry.Path, err)
			}
		}
		removeErr := os.RemoveAll(removalPath)
		release()
		if removeErr != nil {
			return result, fmt.Errorf("remove managed cache %s: %w", entry.Path, removeErr)
		}
		result.Removed = append(result.Removed, entry)
		result.RemovedBytes += entry.Bytes
	}
	return result, nil
}

func makeTreeOwnerWritable(root string) error {
	rooted, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer rooted.Close()
	return fs.WalkDir(rooted.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return rooted.Chmod(path, info.Mode().Perm()|0o300)
	})
}

func cacheChildren(root, kind string, include func(os.DirEntry) bool) ([]CacheEntry, error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result []CacheEntry
	for _, child := range entries {
		if !include(child) {
			continue
		}
		entry, err := cachePath(filepath.Join(root, child.Name()), kind, child.Name())
		if err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, nil
}

func cachePath(path, kind, key string) (CacheEntry, error) {
	entry := CacheEntry{Kind: kind, Key: key, Path: path}
	err := filepath.WalkDir(path, func(current string, directoryEntry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := directoryEntry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(entry.LastUsedAt) {
			entry.LastUsedAt = info.ModTime().UTC()
		}
		if info.Mode().IsRegular() {
			entry.Bytes += info.Size()
		}
		return nil
	})
	return entry, err
}

func activeFrontendSnapshots(stateRoot string) map[string]bool {
	active := map[string]bool{}
	paths, _ := filepath.Glob(filepath.Join(stateRoot, "workspaces", "*", "workspace.json"))
	for _, leasePath := range paths {
		workspace, err := loadWorkspace(leasePath)
		if err != nil || !workspaceRootAlive(workspace.Root) {
			continue
		}
		link, err := os.Readlink(filepath.Join(workspace.Root, "frontend", "dashboard", "node_modules"))
		if err != nil {
			continue
		}
		if !filepath.IsAbs(link) {
			link = filepath.Join(workspace.Root, "frontend", "dashboard", link)
		}
		link = filepath.Clean(link)
		if filepath.Base(link) == "node_modules" {
			link = filepath.Dir(link)
		}
		active[link] = true
	}
	return active
}

func sameFilePath(left, right string) bool {
	left, leftErr := filepath.Abs(left)
	right, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && filepath.Clean(left) == filepath.Clean(right)
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func trySnapshotLock(path string) (func(), bool) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return func() {}, false
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return func() {}, false
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, true
}
