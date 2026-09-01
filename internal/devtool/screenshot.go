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
	"unicode"

	"github.com/google/uuid"

	json "hitkeep/jsonapi"
)

const (
	ScreenshotSchemaVersion = "hk.dev/screenshot/v1"
	MaxScreenshotRoutes     = 8
	maxScreenshotRouteBytes = 2048
	maxScreenshotSelector   = 500
)

type ScreenshotRequest struct {
	Routes    []string `json:"routes,omitempty"`
	Viewport  string   `json:"viewport,omitempty"`
	Theme     string   `json:"theme,omitempty"`
	Scale     int      `json:"scale,omitempty"`
	WaitMS    int      `json:"wait_ms,omitempty"`
	FullPage  bool     `json:"full_page,omitempty"`
	Selector  string   `json:"selector,omitempty"`
	Anonymous bool     `json:"anonymous,omitempty"`
}

type ScreenshotViewport struct {
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Scale  int    `json:"scale"`
}

type ScreenshotArtifact struct {
	Route             string `json:"route"`
	FinalRoute        string `json:"final_route"`
	Path              string `json:"path"`
	MIMEType          string `json:"mime_type"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	Bytes             int64  `json:"bytes"`
	SHA256            string `json:"sha256"`
	DurationMS        int64  `json:"duration_ms"`
	ConsoleErrorCount int    `json:"console_error_count,omitempty"`
	PageErrorCount    int    `json:"page_error_count,omitempty"`
}

type ScreenshotTimings struct {
	BrowserLaunchMS  int64 `json:"browser_launch_ms"`
	AuthenticationMS int64 `json:"authentication_ms,omitempty"`
	TotalMS          int64 `json:"total_ms"`
}

type ScreenshotResult struct {
	SchemaVersion string               `json:"schema_version"`
	Viewport      ScreenshotViewport   `json:"viewport"`
	Theme         string               `json:"theme"`
	FullPage      bool                 `json:"full_page"`
	Selector      string               `json:"selector,omitempty"`
	Anonymous     bool                 `json:"anonymous,omitempty"`
	ManifestPath  string               `json:"manifest_path"`
	Artifacts     []ScreenshotArtifact `json:"artifacts"`
	Timings       ScreenshotTimings    `json:"timings"`
}

func normalizeScreenshotRequest(request ScreenshotRequest) ScreenshotRequest {
	if len(request.Routes) == 0 {
		request.Routes = []string{"/dashboard"}
	}
	request.Routes = append([]string(nil), request.Routes...)
	for index := range request.Routes {
		request.Routes[index] = strings.TrimSpace(request.Routes[index])
	}
	if request.Viewport == "" {
		request.Viewport = "desktop"
	}
	if request.Theme == "" {
		request.Theme = "light"
	}
	if request.Scale == 0 {
		request.Scale = 1
	}
	if request.WaitMS == 0 {
		request.WaitMS = 200
	}
	request.Selector = strings.TrimSpace(request.Selector)
	return request
}

func ValidateScreenshotRequest(request ScreenshotRequest) error {
	request = normalizeScreenshotRequest(request)
	if len(request.Routes) > MaxScreenshotRoutes {
		return fmt.Errorf("capture at most %d routes per screenshot request", MaxScreenshotRoutes)
	}
	seen := make(map[string]struct{}, len(request.Routes))
	for _, route := range request.Routes {
		if err := validateScreenshotRoute(route); err != nil {
			return err
		}
		if _, exists := seen[route]; exists {
			return fmt.Errorf("duplicate screenshot route %q", route)
		}
		seen[route] = struct{}{}
	}
	if request.Viewport != "desktop" && request.Viewport != "mobile" {
		return errors.New("screenshot viewport must be desktop or mobile")
	}
	if request.Theme != "light" && request.Theme != "dark" {
		return errors.New("screenshot theme must be light or dark")
	}
	if request.Scale != 1 && request.Scale != 2 {
		return errors.New("screenshot scale must be 1 or 2")
	}
	if request.WaitMS < 0 || request.WaitMS > 5000 {
		return errors.New("screenshot wait must be between 0 and 5000 milliseconds")
	}
	if len(request.Selector) > maxScreenshotSelector || containsControl(request.Selector) {
		return fmt.Errorf("screenshot selector must contain at most %d printable characters", maxScreenshotSelector)
	}
	if request.Selector != "" && len(request.Routes) != 1 {
		return errors.New("a screenshot selector requires exactly one route")
	}
	if request.Selector != "" && request.FullPage {
		return errors.New("a screenshot selector cannot be combined with full-page capture")
	}
	return nil
}

func validateScreenshotRoute(route string) error {
	if route == "" || len(route) > maxScreenshotRouteBytes || containsControl(route) {
		return fmt.Errorf("screenshot route must contain 1 through %d printable characters", maxScreenshotRouteBytes)
	}
	if !strings.HasPrefix(route, "/") || strings.HasPrefix(route, "//") {
		return fmt.Errorf("screenshot route %q must be a local absolute path", route)
	}
	if strings.Contains(route, "#") {
		return fmt.Errorf("screenshot route %q must not contain a fragment", route)
	}
	return nil
}

func containsControl(value string) bool {
	return strings.ContainsFunc(value, unicode.IsControl)
}

func (a *App) CaptureScreenshots(ctx context.Context, request ScreenshotRequest) (ScreenshotResult, error) {
	request = normalizeScreenshotRequest(request)
	if err := ValidateScreenshotRequest(request); err != nil {
		return ScreenshotResult{}, err
	}
	status, err := a.DevStatus(ctx)
	if err != nil {
		return ScreenshotResult{}, err
	}
	if status.State != DevStateReady && status.State != DevStateDegraded {
		return ScreenshotResult{}, errors.New("development must be ready before screenshot capture; start a seeded workspace session")
	}
	if strings.TrimSpace(status.URLs.Web) == "" {
		return ScreenshotResult{}, errors.New("development did not report a dashboard URL")
	}

	captureID := time.Now().UTC().Format("20060102T150405") + "-" + uuid.NewString()[:8]
	outputDir := filepath.Join(a.workspace.StateDir, "artifacts", "screenshots", captureID)
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return ScreenshotResult{}, fmt.Errorf("create screenshot artifact directory: %w", err)
	}
	keepArtifacts := false
	defer func() {
		if !keepArtifacts {
			_ = os.RemoveAll(outputDir)
		}
	}()

	manifestPath := filepath.Join(outputDir, "manifest.json")
	args := []string{
		"node", "frontend/dashboard/scripts/hitkeep-screenshot.mjs",
		"--base-url", status.URLs.Web,
		"--output-dir", outputDir,
		"--manifest", manifestPath,
		"--viewport", request.Viewport,
		"--theme", request.Theme,
		"--scale", fmt.Sprintf("%d", request.Scale),
		"--wait-ms", fmt.Sprintf("%d", request.WaitMS),
	}
	if request.FullPage {
		args = append(args, "--full-page")
	}
	if request.Selector != "" {
		args = append(args, "--selector", request.Selector)
	}
	if request.Anonymous {
		args = append(args, "--anonymous")
	}
	for _, route := range request.Routes {
		args = append(args, "--route", route)
	}

	environment := []string{
		"HITKEEP_SCREENSHOT_EMAIL=" + environmentOrDefault("HITKEEP_SEED_EMAIL", "demo@example.com"),
		"HITKEEP_SCREENSHOT_PASSWORD=" + environmentOrDefault("HITKEEP_SEED_PASSWORD", "demo1234"),
	}
	var commandLog bytes.Buffer
	if err := a.runCommand(ctx, &commandLog, commandSpec{
		Args:    args,
		Env:     environment,
		Display: fmt.Sprintf("node frontend/dashboard/scripts/hitkeep-screenshot.mjs [%d local route(s)]", len(request.Routes)),
	}); err != nil {
		detail := screenshotCommandDetail(commandLog.String())
		if detail != "" {
			return ScreenshotResult{}, fmt.Errorf("capture screenshots: %w: %s", err, detail)
		}
		return ScreenshotResult{}, fmt.Errorf("capture screenshots: %w", err)
	}

	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return ScreenshotResult{}, fmt.Errorf("read screenshot manifest: %w", err)
	}
	var result ScreenshotResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return ScreenshotResult{}, fmt.Errorf("decode screenshot manifest: %w", err)
	}
	result.ManifestPath = manifestPath
	if err := validateScreenshotResult(outputDir, request, result); err != nil {
		return ScreenshotResult{}, err
	}
	keepArtifacts = true
	paths := []string{result.ManifestPath}
	for _, artifact := range result.Artifacts {
		paths = append(paths, artifact.Path)
	}
	fingerprint, _ := DeveloperSourceFingerprint(a.workspace.Root)
	if err := a.registerArtifactPaths("screenshot", captureID, fingerprint, paths); err != nil {
		return ScreenshotResult{}, fmt.Errorf("index screenshot artifacts: %w", err)
	}
	_ = a.maintainArtifacts()
	return result, nil
}

func environmentOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func screenshotCommandDetail(log string) string {
	lines := strings.Split(strings.TrimSpace(log), "\n")
	for _, line := range slices.Backward(lines) {
		line := strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "$") {
			continue
		}
		if len(line) > 500 {
			line = line[:500] + "…"
		}
		return redactError(line)
	}
	return ""
}

func validateScreenshotResult(outputDir string, request ScreenshotRequest, result ScreenshotResult) error {
	if result.SchemaVersion != ScreenshotSchemaVersion {
		return fmt.Errorf("unknown screenshot manifest schema %q", result.SchemaVersion)
	}
	expectedViewport := ScreenshotViewport{Name: request.Viewport, Width: 1440, Height: 1024, Scale: request.Scale}
	if request.Viewport == "mobile" {
		expectedViewport.Width = 390
		expectedViewport.Height = 844
	}
	if result.Viewport != expectedViewport || result.Theme != request.Theme || result.FullPage != request.FullPage || result.Selector != request.Selector || result.Anonymous != request.Anonymous {
		return errors.New("screenshot manifest does not match the validated capture request")
	}
	if len(result.Artifacts) != len(request.Routes) {
		return fmt.Errorf("screenshot manifest contains %d artifacts for %d routes", len(result.Artifacts), len(request.Routes))
	}
	for index, artifact := range result.Artifacts {
		if artifact.Route != request.Routes[index] {
			return fmt.Errorf("screenshot manifest route %d is %q, want %q", index, artifact.Route, request.Routes[index])
		}
		if err := validateScreenshotRoute(artifact.FinalRoute); err != nil {
			return fmt.Errorf("screenshot artifact %q has invalid final route: %w", artifact.Path, err)
		}
		if artifact.MIMEType != "image/png" || artifact.Width <= 0 || artifact.Height <= 0 || artifact.Bytes <= 0 {
			return fmt.Errorf("screenshot artifact %q has invalid image metadata", artifact.Path)
		}
		if len(artifact.SHA256) != 64 {
			return fmt.Errorf("screenshot artifact %q has an invalid digest", artifact.Path)
		}
		absolute, err := filepath.Abs(artifact.Path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(outputDir, absolute)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("screenshot artifact escapes managed output: %q", artifact.Path)
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return fmt.Errorf("inspect screenshot artifact: %w", err)
		}
		if !info.Mode().IsRegular() || info.Size() != artifact.Bytes {
			return fmt.Errorf("screenshot artifact %q does not match its manifest", artifact.Path)
		}
	}
	return nil
}
