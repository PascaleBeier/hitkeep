package devtool

import (
	"github.com/spf13/fileflow"

	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	publicAssetsArchive = "public-assets.tar.gz"
	maxAssetFileBytes   = 64 << 20
	maxAssetTotalBytes  = 256 << 20
)

type CIArtifactResult struct {
	Artifacts []string `json:"artifacts"`
	Count     int      `json:"count"`
	Bytes     int64    `json:"bytes,omitempty"`
}

func (a *App) BuildDashboardArchive(ctx context.Context, writer io.Writer) (CIArtifactResult, error) {
	if err := a.runCommand(ctx, writer, commandSpec{Args: npmCommand("ci", "--no-audit", "--no-fund"), Dir: "frontend/dashboard"}); err != nil {
		return CIArtifactResult{}, err
	}
	if err := a.runCommand(ctx, writer, commandSpec{Args: npmCommand("run", "build:prod"), Dir: "frontend/dashboard"}); err != nil {
		return CIArtifactResult{}, err
	}
	source := filepath.Join(a.workspace.Root, "frontend", "dashboard", "dist", "dashboard", "browser")
	archivePath := filepath.Join(a.workspace.Root, publicAssetsArchive)
	count, size, err := writeDeterministicTarGzip(source, archivePath)
	if err != nil {
		return CIArtifactResult{}, err
	}
	return CIArtifactResult{Artifacts: []string{archivePath}, Count: count, Bytes: size}, nil
}

func npmCommand(arguments ...string) []string {
	command := make([]string, 0, 1+len(arguments))
	command = append(command, "npm")
	return append(command, arguments...)
}

func (a *App) RestoreDashboardArchive(archivePath string) (CIArtifactResult, error) {
	if !filepath.IsAbs(archivePath) {
		archivePath = filepath.Join(a.workspace.Root, archivePath)
	}
	resolved, err := ValidateWorkspacePath(a.workspace, archivePath)
	if err != nil {
		return CIArtifactResult{}, err
	}
	destination := filepath.Join(a.workspace.Root, "public")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return CIArtifactResult{}, err
	}
	count, size, err := extractPublicAssets(resolved, destination)
	if err != nil {
		return CIArtifactResult{}, err
	}
	return CIArtifactResult{Artifacts: []string{destination}, Count: count, Bytes: size}, nil
}

func (a *App) PreparePublicImageContext() (CIArtifactResult, error) {
	var selfHosted []string
	for _, arch := range []string{"amd64", "arm64"} {
		path := filepath.Join(a.workspace.Root, "hitkeep-linux-"+arch)
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			return CIArtifactResult{}, fmt.Errorf("required public image binary is missing: %s", filepath.Base(path))
		}
		selfHosted = append(selfHosted, path)
	}
	quarantine := filepath.Join(a.workspace.StateDir, "artifacts", "cloud-release-binaries")
	if err := os.MkdirAll(quarantine, 0o700); err != nil {
		return CIArtifactResult{}, err
	}
	flow := fileflow.Flow{NoCreateDirs: true}
	for _, arch := range []string{"amd64", "arm64"} {
		name := "hitkeep-cloud-linux-" + arch
		source := filepath.Join(a.workspace.Root, name)
		if _, err := os.Stat(source); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return CIArtifactResult{}, err
		}
		destination := filepath.Join(quarantine, name)
		_ = os.Remove(destination)
		final, err := flow.Move(source, destination)
		if err != nil {
			return CIArtifactResult{}, fmt.Errorf("isolate cloud binary %s: %w", name, err)
		}
		if filepath.Dir(final) != quarantine {
			return CIArtifactResult{}, fmt.Errorf("isolated cloud binary escaped quarantine: %s", name)
		}
	}
	cloud, _ := filepath.Glob(filepath.Join(a.workspace.Root, "hitkeep-cloud-linux-*"))
	if len(cloud) != 0 {
		return CIArtifactResult{}, errors.New("cloud binaries remain in the public image context")
	}
	return CIArtifactResult{Artifacts: selfHosted, Count: len(selfHosted)}, nil
}

func (a *App) GenerateReleaseChecksums() (CIArtifactResult, error) {
	artifacts, err := a.requiredReleaseArtifacts()
	if err != nil {
		return CIArtifactResult{}, err
	}
	checksumPath := filepath.Join(a.workspace.Root, "SHA256SUMS")
	temporary, err := os.CreateTemp(a.workspace.Root, ".hk-checksums-*")
	if err != nil {
		return CIArtifactResult{}, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	for _, path := range artifacts {
		digest, digestErr := fileSHA256(path)
		if digestErr != nil {
			_ = temporary.Close()
			return CIArtifactResult{}, digestErr
		}
		if _, err := fmt.Fprintf(temporary, "%s  %s\n", digest, filepath.Base(path)); err != nil {
			_ = temporary.Close()
			return CIArtifactResult{}, err
		}
	}
	if err := temporary.Close(); err != nil {
		return CIArtifactResult{}, err
	}
	if err := os.Rename(temporaryPath, checksumPath); err != nil {
		return CIArtifactResult{}, err
	}
	return CIArtifactResult{Artifacts: append(artifacts, checksumPath), Count: len(artifacts) + 1}, nil
}

func (a *App) VerifyReleaseArtifacts() (CIArtifactResult, error) {
	artifacts, err := a.requiredReleaseArtifacts()
	if err != nil {
		return CIArtifactResult{}, err
	}
	checksumPath := filepath.Join(a.workspace.Root, "SHA256SUMS")
	file, err := os.Open(checksumPath)
	if err != nil {
		return CIArtifactResult{}, err
	}
	defer file.Close()
	want := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || strings.ContainsAny(fields[1], `/\\`) {
			return CIArtifactResult{}, errors.New("invalid SHA256SUMS entry")
		}
		want[fields[1]] = fields[0]
	}
	if err := scanner.Err(); err != nil {
		return CIArtifactResult{}, err
	}
	for _, path := range artifacts {
		name := filepath.Base(path)
		digest, err := fileSHA256(path)
		if err != nil {
			return CIArtifactResult{}, err
		}
		if want[name] != digest {
			return CIArtifactResult{}, fmt.Errorf("checksum mismatch for %s", name)
		}
		delete(want, name)
	}
	if len(want) != 0 {
		return CIArtifactResult{}, errors.New("SHA256SUMS contains unexpected artifacts")
	}
	return CIArtifactResult{Artifacts: append(artifacts, checksumPath), Count: len(artifacts) + 1}, nil
}

func (a *App) requiredReleaseArtifacts() ([]string, error) {
	var artifacts []string
	for _, name := range []string{
		"hitkeep-cloud-linux-amd64", "hitkeep-cloud-linux-arm64",
		"hitkeep-linux-amd64", "hitkeep-linux-arm64",
		"hitkeep-configuration.json",
	} {
		path := filepath.Join(a.workspace.Root, name)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("required release artifact is missing: %s", name)
		}
		artifacts = append(artifacts, path)
	}
	slices.Sort(artifacts)
	return artifacts, nil
}

func writeDeterministicTarGzip(source, destination string) (int, int64, error) {
	paths := []string{}
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refuse dashboard asset symlink: %s", path)
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	slices.Sort(paths)
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".hk-assets-*.tar.gz")
	if err != nil {
		return 0, 0, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	gzipWriter := gzip.NewWriter(temporary)
	gzipWriter.ModTime = time.Unix(0, 0).UTC()
	gzipWriter.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	count := 0
	var total int64
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return 0, 0, err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return 0, 0, err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return 0, 0, err
		}
		header.Name = filepath.ToSlash(relative)
		header.ModTime = time.Unix(0, 0).UTC()
		header.AccessTime = time.Time{}
		header.ChangeTime = time.Time{}
		header.Uid, header.Gid = 0, 0
		header.Uname, header.Gname = "", ""
		if err := tarWriter.WriteHeader(header); err != nil {
			return 0, 0, err
		}
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return 0, 0, err
			}
			_, copyErr := io.Copy(tarWriter, file)
			closeErr := file.Close()
			if copyErr != nil {
				return 0, 0, copyErr
			}
			if closeErr != nil {
				return 0, 0, closeErr
			}
			count++
			total += info.Size()
		}
	}
	if err := tarWriter.Close(); err != nil {
		return 0, 0, err
	}
	if err := gzipWriter.Close(); err != nil {
		return 0, 0, err
	}
	if err := temporary.Close(); err != nil {
		return 0, 0, err
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return 0, 0, err
	}
	return count, total, nil
}

func extractPublicAssets(archivePath, destination string) (int, int64, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return 0, 0, err
	}
	defer gzipReader.Close()
	root, err := os.OpenRoot(destination)
	if err != nil {
		return 0, 0, err
	}
	defer root.Close()
	reader := tar.NewReader(gzipReader)
	count := 0
	var total int64
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, 0, err
		}
		name := filepath.Clean(filepath.FromSlash(header.Name))
		if name == "." || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return 0, 0, fmt.Errorf("unsafe dashboard archive path %q", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(name, 0o755); err != nil {
				return 0, 0, err
			}
		case tar.TypeReg:
			if header.Size < 0 || header.Size > maxAssetFileBytes || total+header.Size > maxAssetTotalBytes {
				return 0, 0, errors.New("dashboard archive exceeds bounded extraction size")
			}
			if err := root.MkdirAll(filepath.Dir(name), 0o755); err != nil {
				return 0, 0, err
			}
			mode := fs.FileMode(0o644)
			if header.Mode&0o111 != 0 {
				mode = 0o755
			}
			output, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return 0, 0, err
			}
			if _, err := io.CopyN(output, reader, header.Size); err != nil {
				_ = output.Close()
				return 0, 0, err
			}
			if err := output.Close(); err != nil {
				return 0, 0, err
			}
			count++
			total += header.Size
		default:
			return 0, 0, fmt.Errorf("unsupported dashboard archive entry %q", header.Name)
		}
	}
	return count, total, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
