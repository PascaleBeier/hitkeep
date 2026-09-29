package devtool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// readmeScreenshots are the docs screenshots mirrored into the repository
// README assets by `hk screenshot docs --sync`. Hand-made README images such
// as .github/assets/mcp.png are not captured and stay untouched.
var readmeScreenshots = []string{
	"dashboard-overview.png",
	"analytics-ecommerce.png",
	"analytics-search-console.png",
	"analytics-ai-visibility.png",
	"feature-ask-ai-answer.png",
}

// docsScreenshotTargets are the optional subsets the curated script supports.
var docsScreenshotTargets = []string{"", "ask-ai"}

// DocsScreenshotRequest selects the curated docs screenshot run.
type DocsScreenshotRequest struct {
	Target string `json:"target,omitempty"`
	Sync   bool   `json:"sync,omitempty"`
}

// DocsScreenshotResult lists the captured docs screenshots and, when synced,
// the repository and docs paths that were updated.
type DocsScreenshotResult struct {
	OutputDir string   `json:"output_dir"`
	Files     []string `json:"files"`
	Synced    []string `json:"synced,omitempty"`
	DocsDir   string   `json:"docs_dir,omitempty"`
}

// CaptureDocsScreenshots runs the curated docs and README screenshot set
// against the ready, seeded development session. Sync is an explicit,
// CLI-only source update; captures otherwise stay in workspace artifacts.
func (a *App) CaptureDocsScreenshots(ctx context.Context, request DocsScreenshotRequest) (DocsScreenshotResult, error) {
	request.Target = strings.ToLower(strings.TrimSpace(request.Target))
	if !slices.Contains(docsScreenshotTargets, request.Target) {
		return DocsScreenshotResult{}, fmt.Errorf("unknown docs screenshot target %q; supported: ask-ai", request.Target)
	}
	status, err := a.DevStatus(ctx)
	if err != nil {
		return DocsScreenshotResult{}, err
	}
	if status.State != DevStateReady && status.State != DevStateDegraded {
		return DocsScreenshotResult{}, errors.New("development must be ready before docs screenshots; start a seeded workspace session")
	}

	captureID := "docs-" + time.Now().UTC().Format("20060102T150405") + "-" + uuid.NewString()[:8]
	outputDir := filepath.Join(a.workspace.StateDir, "artifacts", "screenshots", captureID)
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return DocsScreenshotResult{}, fmt.Errorf("create docs screenshot directory: %w", err)
	}

	environment := []string{
		"HITKEEP_URL=" + status.URLs.Web,
		"HITKEEP_EMAIL=" + environmentOrDefault("HITKEEP_SEED_EMAIL", "demo@example.com"),
		"HITKEEP_PASSWORD=" + environmentOrDefault("HITKEEP_SEED_PASSWORD", "demo1234"),
		"HITKEEP_SCREENSHOT_SITE=" + environmentOrDefault("HITKEEP_SEED_DOMAIN", "acme-analytics.io"),
		"OUTPUT_DIR=" + outputDir,
		"SCREENSHOT_TARGET=" + request.Target,
	}
	var commandLog bytes.Buffer
	if err := a.runCommand(ctx, &commandLog, commandSpec{
		Args:    []string{"node", "frontend/dashboard/scripts/docs-screenshots.mjs"},
		Env:     environment,
		Display: "node frontend/dashboard/scripts/docs-screenshots.mjs [curated docs set]",
	}); err != nil {
		if detail := screenshotCommandDetail(commandLog.String()); detail != "" {
			return DocsScreenshotResult{}, fmt.Errorf("capture docs screenshots: %w: %s", err, detail)
		}
		return DocsScreenshotResult{}, fmt.Errorf("capture docs screenshots: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(outputDir, "*.png"))
	if err != nil || len(files) == 0 {
		return DocsScreenshotResult{}, fmt.Errorf("docs screenshot run produced no images in %s", outputDir)
	}
	slices.Sort(files)
	result := DocsScreenshotResult{OutputDir: outputDir, Files: files}
	fingerprint, _ := DeveloperSourceFingerprint(a.workspace.Root)
	if err := a.registerArtifactPaths("screenshot", captureID, fingerprint, files); err != nil {
		return result, fmt.Errorf("index docs screenshot artifacts: %w", err)
	}
	_ = a.maintainArtifacts()

	if request.Sync {
		docsDir := filepath.Join(filepath.Dir(a.workspace.Root), "hitkeep-docs", "src", "assets", "screenshots")
		// A targeted subset never replaces the README set.
		readme := readmeScreenshots
		if request.Target != "" {
			readme = nil
		}
		synced, err := syncDocsScreenshots(outputDir, filepath.Join(a.workspace.Root, ".github", "assets"), docsDir, readme)
		result.Synced = synced
		if info, statErr := os.Stat(docsDir); statErr == nil && info.IsDir() {
			result.DocsDir = docsDir
		}
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

// syncDocsScreenshots copies every capture into docsDir when that directory
// exists (the private docs repository is optional) and the README subset into
// readmeDir. It returns the destination paths it wrote.
func syncDocsScreenshots(sourceDir, readmeDir, docsDir string, readme []string) ([]string, error) {
	var written []string
	source, err := os.OpenRoot(sourceDir)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	// Roots confine reads and writes to their directories, so a capture name
	// can never escape the destination.
	copyFile := func(name, destinationDir string) error {
		data, err := source.ReadFile(name)
		if err != nil {
			return err
		}
		destination, err := os.OpenRoot(destinationDir)
		if err != nil {
			return err
		}
		defer destination.Close()
		if err := destination.WriteFile(name, data, 0o644); err != nil { // #nosec G306 -- committed image assets
			return err
		}
		written = append(written, filepath.Join(destinationDir, name))
		return nil
	}

	if info, err := os.Stat(docsDir); err == nil && info.IsDir() {
		files, _ := filepath.Glob(filepath.Join(sourceDir, "*.png"))
		for _, file := range files {
			if err := copyFile(filepath.Base(file), docsDir); err != nil {
				return written, fmt.Errorf("sync docs screenshot: %w", err)
			}
		}
	}
	if len(readme) > 0 {
		if err := os.MkdirAll(readmeDir, 0o750); err != nil {
			return written, err
		}
		for _, name := range readme {
			if _, err := os.Stat(filepath.Join(sourceDir, name)); err != nil {
				return written, fmt.Errorf("README screenshot %s was not captured", name)
			}
			if err := copyFile(name, readmeDir); err != nil {
				return written, fmt.Errorf("sync README screenshot: %w", err)
			}
		}
	}
	return written, nil
}
