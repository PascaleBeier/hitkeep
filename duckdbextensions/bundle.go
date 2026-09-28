// Package duckdbextensions owns HitKeep's offline, signed DuckDB extensions.
package duckdbextensions

import (
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

//go:embed manifest.json
var manifestJSON []byte

type artifact struct {
	SHA256 string `json:"sha256"`
}

var manifest = sync.OnceValues(func() (struct {
	Version   string              `json:"version"`
	Artifacts map[string]artifact `json:"artifacts"`
}, error) {
	var result struct {
		Version   string              `json:"version"`
		Artifacts map[string]artifact `json:"artifacts"`
	}
	err := json.Unmarshal(manifestJSON, &result)
	return result, err
})

var extraction struct {
	sync.Mutex
	directory string
	started   bool
	ready     map[string]bool
}

// Configure selects the application-owned extraction directory before any
// extension is loaded. Native libraries must share one path across all stores.
func Configure(directory string) error {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	extraction.Lock()
	defer extraction.Unlock()
	if extraction.started && extraction.directory != directory {
		return fmt.Errorf("DuckDB extension directory cannot change after loading")
	}
	extraction.directory = directory
	return nil
}

// Path returns the same signed native library path for every database in the
// process. Standalone database utilities use a per-user temporary cache.
func Path(name string) (string, error) {
	extraction.Lock()
	if extraction.directory == "" {
		extraction.directory = filepath.Join(os.TempDir(), fmt.Sprintf("hitkeep-%d", os.Getuid()), ".duckdb", "extensions")
	}
	extraction.started = true
	directory := extraction.directory
	extraction.Unlock()
	return extract(directory, name)
}

// extract atomically materializes an exact bundled artifact. Versioned,
// content-addressed paths let upgrades and rollbacks coexist.
func extract(directory, name string) (string, error) {
	if name != "httpfs" && name != "aws" && name != "excel" {
		return "", fmt.Errorf("unknown bundled DuckDB extension %q", name)
	}
	lock, err := manifest()
	if err != nil {
		return "", err
	}
	platform := runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "darwin" {
		platform = "osx_" + runtime.GOARCH
	}
	key := platform + "/" + name + ".duckdb_extension.gz"
	entry, ok := lock.Artifacts[key]
	if !ok {
		return "", fmt.Errorf("no bundled DuckDB extension for %s", platform)
	}
	directory, err = filepath.Abs(filepath.Join(directory, lock.Version, platform, entry.SHA256))
	if err != nil {
		return "", err
	}
	path := filepath.Join(directory, name+".duckdb_extension")
	extraction.Lock()
	defer extraction.Unlock()
	if extraction.ready[path] {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", fmt.Errorf("create DuckDB extension directory: %w", err)
	}
	if file, err := os.Open(path); err == nil {
		hash := sha256.New()
		_, readErr := io.Copy(hash, file)
		closeErr := file.Close()
		if readErr == nil && closeErr == nil && fmt.Sprintf("%x", hash.Sum(nil)) == entry.SHA256 {
			markReady(path)
			return path, nil
		}
	}
	archive, err := archives.Open("assets/" + key)
	if err != nil {
		return "", err
	}
	defer archive.Close()
	reader, err := gzip.NewReader(archive)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	file, err := os.CreateTemp(directory, ".extract-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(reader, 128<<20))
	closeErr := file.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if n >= 128<<20 {
		return "", fmt.Errorf("bundled %s extension exceeds size limit", name)
	}
	if fmt.Sprintf("%x", hash.Sum(nil)) != entry.SHA256 {
		return "", fmt.Errorf("bundled %s extension checksum mismatch", name)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return "", err
	}
	markReady(path)
	return path, nil
}

func markReady(path string) {
	if extraction.ready == nil {
		extraction.ready = make(map[string]bool)
	}
	extraction.ready[path] = true
}
