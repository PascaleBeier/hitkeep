// Package refresh maintains the signed extension bundle for the DuckDB selected by go.mod.
package refresh

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const bundleDir = "internal/duckdbextensions"

type artifact struct {
	ArchiveSHA256 string `json:"archive_sha256"`
	SHA256        string `json:"sha256"`
}

type manifest struct {
	Version   string              `json:"version"`
	Artifacts map[string]artifact `json:"artifacts"`
}

func Run(ctx context.Context, check bool, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, "go", "list", "-m", "-json", "github.com/duckdb/duckdb-go/v2").Output()
	if err != nil {
		return fmt.Errorf("resolve DuckDB dependency: %w", err)
	}
	var module struct {
		Version string
		Replace *json.RawMessage
	}
	if err := json.Unmarshal(output, &module); err != nil {
		return err
	}
	if module.Replace != nil {
		return fmt.Errorf("DuckDB replacement requires an explicitly rebuilt extension bundle")
	}
	parts := strings.Split(module.Version, ".")
	if len(parts) != 3 || parts[0] != "v2" {
		return fmt.Errorf("unsupported DuckDB Go version %q", module.Version)
	}
	encoded, err := strconv.Atoi(parts[1])
	if err != nil || encoded < 10000 {
		return fmt.Errorf("unsupported DuckDB Go version %q", module.Version)
	}
	version := fmt.Sprintf("v%d.%d.%d", encoded/10000, encoded/100%100, encoded%100)
	lockPath := filepath.Join(bundleDir, "manifest.json")
	lock := manifest{Version: version, Artifacts: make(map[string]artifact)}
	if check {
		data, err := os.ReadFile(lockPath)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &lock); err != nil {
			return err
		}
		if lock.Version != version {
			return fmt.Errorf("DuckDB is %s but extensions are %s; run go run ./internal/duckdbextensions/update", version, lock.Version)
		}
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	for _, platform := range []string{"linux_amd64", "linux_arm64", "osx_amd64", "osx_arm64"} {
		for _, name := range []string{"httpfs", "aws", "excel"} {
			if err := ctx.Err(); err != nil {
				return err
			}
			key := platform + "/" + name + ".duckdb_extension.gz"
			path := filepath.Join(bundleDir, "assets", filepath.FromSlash(key))
			var data []byte
			if check {
				data, err = os.ReadFile(path)
			} else {
				url := "https://extensions.duckdb.org/" + version + "/" + key
				fmt.Fprintln(out, "Downloading", url)
				data, err = download(ctx, client, url)
			}
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			reader, err := gzip.NewReader(bytes.NewReader(data))
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			hash := sha256.New()
			n, readErr := io.Copy(hash, io.LimitReader(reader, 128<<20))
			closeErr := reader.Close()
			if readErr != nil {
				return readErr
			}
			if closeErr != nil {
				return closeErr
			}
			if n >= 128<<20 {
				return fmt.Errorf("%s exceeds extension size limit", key)
			}
			entry := artifact{ArchiveSHA256: fmt.Sprintf("%x", sha256.Sum256(data)), SHA256: fmt.Sprintf("%x", hash.Sum(nil))}
			if check {
				if lock.Artifacts[key] != entry {
					return fmt.Errorf("%s does not match the extension manifest", key)
				}
				continue
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(path, data, 0600); err != nil { //nolint:gosec // Artifact paths use only fixed platform and extension names.
				return err
			}
			lock.Artifacts[key] = entry
		}
	}
	if check {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(lockPath, append(data, '\n'), 0600)
}

func download(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if len(data) >= 64<<20 {
		return nil, fmt.Errorf("archive exceeds size limit")
	}
	return data, nil
}
