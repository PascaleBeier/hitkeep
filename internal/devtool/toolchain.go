package devtool

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	toolchainDownloadLimit = 512 << 20
	toolchainMetadataLimit = 8 << 20
)

type managedToolchainPaths struct {
	Root             string
	GoRoot           string
	NodeRoot         string
	GoExecutable     string
	NodeExecutable   string
	NPMExecutable    string
	NPMCLI           string
	GoBuildCache     string
	GoModuleCache    string
	NPMCache         string
	PlaywrightCache  string
	Platform         string
	GoArchive        string
	NodeArchive      string
	GoDownloadURL    string
	NodeDownloadURL  string
	NodeChecksumsURL string
}

func (a *App) managedToolchainPaths() (managedToolchainPaths, error) {
	config, err := a.ToolchainConfig()
	if err != nil {
		return managedToolchainPaths{}, err
	}
	if !strings.Contains(" darwin linux ", " "+runtime.GOOS+" ") || !strings.Contains(" amd64 arm64 ", " "+runtime.GOARCH+" ") {
		return managedToolchainPaths{}, fmt.Errorf("managed toolchains do not support %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	root := filepath.Join(filepath.Dir(filepath.Dir(a.workspace.StateDir)), "shared")
	platform := runtime.GOOS + "-" + runtime.GOARCH
	goArchive := fmt.Sprintf("go%s.%s-%s.tar.gz", config.Go, runtime.GOOS, runtime.GOARCH)
	nodeArch := runtime.GOARCH
	if nodeArch == "amd64" {
		nodeArch = "x64"
	}
	nodeArchive := fmt.Sprintf("node-v%s-%s-%s.tar.gz", config.Node, runtime.GOOS, nodeArch)
	goRoot := filepath.Join(root, "toolchains", "go-"+config.Go+"-"+platform)
	nodeRoot := filepath.Join(root, "toolchains", "node-"+config.Node+"-"+platform)
	return managedToolchainPaths{
		Root:             root,
		GoRoot:           goRoot,
		NodeRoot:         nodeRoot,
		GoExecutable:     filepath.Join(goRoot, "bin", "go"),
		NodeExecutable:   filepath.Join(nodeRoot, "bin", "node"),
		NPMExecutable:    filepath.Join(nodeRoot, "bin", "npm"),
		NPMCLI:           filepath.Join(nodeRoot, "lib", "node_modules", "npm", "bin", "npm-cli.js"),
		GoBuildCache:     filepath.Join(root, "go-build"),
		GoModuleCache:    filepath.Join(root, "go-mod"),
		NPMCache:         filepath.Join(root, "npm"),
		PlaywrightCache:  filepath.Join(root, "playwright"),
		Platform:         platform,
		GoArchive:        goArchive,
		NodeArchive:      nodeArchive,
		GoDownloadURL:    "https://go.dev/dl/" + goArchive,
		NodeDownloadURL:  fmt.Sprintf("https://nodejs.org/dist/v%s/%s", config.Node, nodeArchive),
		NodeChecksumsURL: fmt.Sprintf("https://nodejs.org/dist/v%s/SHASUMS256.txt", config.Node),
	}, nil
}

func (a *App) preferredDeveloperExecutable(name string) string {
	paths, err := a.managedToolchainPaths()
	if err != nil {
		return name
	}
	candidate := ""
	switch name {
	case "go":
		candidate = paths.GoExecutable
	case "node":
		candidate = paths.NodeExecutable
	case "npm":
		candidate = paths.NPMExecutable
	}
	if candidate != "" {
		if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			return candidate
		}
	}
	return name
}

func (a *App) preferredNPMProbe() (string, []string) {
	paths, err := a.managedToolchainPaths()
	if err == nil && isExecutableFile(paths.NodeExecutable) {
		if info, statErr := os.Stat(paths.NPMCLI); statErr == nil && info.Mode().IsRegular() {
			return paths.NodeExecutable, []string{paths.NPMCLI, "--version"}
		}
	}
	return a.preferredDeveloperExecutable("npm"), []string{"--version"}
}

func (a *App) ensureManagedToolchains(ctx context.Context, writer io.Writer) error {
	paths, err := a.managedToolchainPaths()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(paths.Root, "toolchains"), 0o700); err != nil {
		return fmt.Errorf("create managed toolchain root: %w", err)
	}
	lock, err := lockStateRoot(paths.Root, "managed-toolchains-"+paths.Platform)
	if err != nil {
		return err
	}
	defer unlockStateRoot(lock)

	if !isExecutableFile(paths.GoExecutable) {
		_, _ = fmt.Fprintln(writer, "provisioning pinned managed Go toolchain")
		checksum, checksumErr := goArchiveChecksum(ctx, paths.GoArchive)
		if checksumErr != nil {
			return checksumErr
		}
		if err := installVerifiedToolchainArchive(ctx, paths.GoDownloadURL, checksum, paths.GoRoot); err != nil {
			return fmt.Errorf("install managed Go toolchain: %w", err)
		}
	}
	if !isExecutableFile(paths.NodeExecutable) {
		_, _ = fmt.Fprintln(writer, "provisioning pinned managed Node.js toolchain")
		checksum, checksumErr := nodeArchiveChecksum(ctx, paths.NodeChecksumsURL, paths.NodeArchive)
		if checksumErr != nil {
			return checksumErr
		}
		if err := installVerifiedToolchainArchive(ctx, paths.NodeDownloadURL, checksum, paths.NodeRoot); err != nil {
			return fmt.Errorf("install managed Node.js toolchain: %w", err)
		}
	}
	config, _ := a.ToolchainConfig()
	if detected := commandOutput(ctx, paths.NodeExecutable, paths.NPMCLI, "--version"); detected != config.NPM {
		_, _ = fmt.Fprintln(writer, "provisioning pinned managed npm toolchain")
		if err := os.MkdirAll(paths.NPMCache, 0o700); err != nil {
			return err
		}
		command := exec.CommandContext(ctx, paths.NodeExecutable, paths.NPMCLI, "install", "--global", "--prefix", paths.NodeRoot, "--cache", paths.NPMCache, "--no-audit", "--no-fund", "npm@"+config.NPM) //nolint:gosec
		command.Dir = a.workspace.Root
		command.Env = a.commandEnvironment(nil)
		command.Stdout = writer
		command.Stderr = writer
		if err := command.Run(); err != nil {
			return fmt.Errorf("install managed npm %s: %w", config.NPM, err)
		}
	}
	for _, directory := range []string{paths.GoBuildCache, paths.GoModuleCache, paths.NPMCache, paths.PlaywrightCache} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0
}

func commandOutput(ctx context.Context, executable string, args ...string) string {
	output, err := exec.CommandContext(ctx, executable, args...).Output() //nolint:gosec
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

type goDownloadRelease struct {
	Version string `json:"version"`
	Files   []struct {
		Filename string `json:"filename"`
		SHA256   string `json:"sha256"`
	} `json:"files"`
}

func goArchiveChecksum(ctx context.Context, filename string) (string, error) {
	raw, err := downloadMetadata(ctx, "https://go.dev/dl/?mode=json&include=all")
	if err != nil {
		return "", fmt.Errorf("download Go release metadata: %w", err)
	}
	var releases []goDownloadRelease
	if err := json.Unmarshal(raw, &releases); err != nil {
		return "", fmt.Errorf("decode Go release metadata: %w", err)
	}
	for _, release := range releases {
		for _, file := range release.Files {
			if file.Filename == filename && len(file.SHA256) == sha256.Size*2 {
				return strings.ToLower(file.SHA256), nil
			}
		}
	}
	return "", fmt.Errorf("go release metadata does not contain %s", filename)
}

func nodeArchiveChecksum(ctx context.Context, checksumsURL, filename string) (string, error) {
	raw, err := downloadMetadata(ctx, checksumsURL)
	if err != nil {
		return "", fmt.Errorf("download Node.js checksums: %w", err)
	}
	for line := range strings.Lines(string(raw)) {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == filename && len(fields[0]) == sha256.Size*2 {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("node.js checksums do not contain %s", filename)
}

func downloadMetadata(ctx context.Context, url string) ([]byte, error) {
	request, err := toolchainRequest(ctx, url)
	if err != nil {
		return nil, err
	}
	response, err := toolchainHTTPClient().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", url, response.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, toolchainMetadataLimit+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > toolchainMetadataLimit {
		return nil, errors.New("toolchain metadata exceeds size limit")
	}
	return raw, nil
}

func installVerifiedToolchainArchive(ctx context.Context, url, expectedChecksum, target string) error {
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	archive, err := os.CreateTemp(parent, ".toolchain-*.tar.gz")
	if err != nil {
		return err
	}
	archivePath := archive.Name()
	defer os.Remove(archivePath)
	digest := sha256.New()
	request, err := toolchainRequest(ctx, url)
	if err != nil {
		_ = archive.Close()
		return err
	}
	response, err := toolchainHTTPClient().Do(request)
	if err != nil {
		_ = archive.Close()
		return err
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		_ = archive.Close()
		return fmt.Errorf("%s returned %s", url, response.Status)
	}
	written, copyErr := io.Copy(io.MultiWriter(archive, digest), io.LimitReader(response.Body, toolchainDownloadLimit+1))
	closeResponseErr := response.Body.Close()
	closeArchiveErr := archive.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeResponseErr != nil {
		return closeResponseErr
	}
	if closeArchiveErr != nil {
		return closeArchiveErr
	}
	if written > toolchainDownloadLimit {
		return errors.New("toolchain archive exceeds size limit")
	}
	detectedChecksum := hex.EncodeToString(digest.Sum(nil))
	if !strings.EqualFold(detectedChecksum, expectedChecksum) {
		return fmt.Errorf("toolchain checksum mismatch: got %s", detectedChecksum)
	}
	staging, err := os.MkdirTemp(parent, ".toolchain-extract-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := extractToolchainTarGzip(archivePath, staging); err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		if !pathWithin(parent, target) {
			return fmt.Errorf("refuse managed toolchain target outside cache root: %s", target)
		}
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("remove incomplete managed toolchain: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(staging, target); err != nil {
		return err
	}
	return nil
}

func extractToolchainTarGzip(archivePath, destinationRoot string) error {
	archive, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archive.Close()
	compressed, err := gzip.NewReader(archive)
	if err != nil {
		return err
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if filepath.IsAbs(header.Name) {
			return fmt.Errorf("toolchain archive contains absolute path: %s", header.Name)
		}
		name := filepath.ToSlash(filepath.Clean(header.Name))
		if name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("toolchain archive path escapes destination: %s", header.Name)
		}
		_, relative, found := strings.Cut(name, "/")
		if !found || relative == "" || relative == "." {
			continue
		}
		destination := filepath.Join(destinationRoot, filepath.FromSlash(relative))
		if !pathWithin(destinationRoot, destination) {
			return fmt.Errorf("toolchain archive path escapes destination: %s", header.Name)
		}
		mode := os.FileMode(uint32(header.Mode) & 0o777) //nolint:gosec // tar modes are deliberately restricted to Unix permission bits.
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destination, mode); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, io.LimitReader(reader, toolchainDownloadLimit+1))
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink:
			if filepath.IsAbs(header.Linkname) {
				return fmt.Errorf("toolchain archive contains absolute symlink: %s", header.Name)
			}
			resolvedTarget := filepath.Clean(filepath.Join(filepath.Dir(destination), filepath.FromSlash(header.Linkname)))
			if !pathWithin(destinationRoot, resolvedTarget) {
				return fmt.Errorf("toolchain archive symlink escapes destination: %s", header.Name)
			}
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(filepath.FromSlash(header.Linkname), destination); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported toolchain archive entry type %d for %s", header.Typeflag, header.Name)
		}
	}
}

func toolchainHTTPClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Minute}
}

func toolchainRequest(ctx context.Context, url string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "HitKeep-hk/managed-toolchain")
	return request, nil
}
