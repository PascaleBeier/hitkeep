package devtool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateScreenshotRequestKeepsCaptureLocalAndBounded(t *testing.T) {
	valid := ScreenshotRequest{
		Routes:   []string{"/admin/status", "/dashboard?range=7d"},
		Viewport: "desktop",
		Theme:    "dark",
		Scale:    1,
		WaitMS:   250,
	}
	if err := ValidateScreenshotRequest(valid); err != nil {
		t.Fatal(err)
	}

	for name, request := range map[string]ScreenshotRequest{
		"remote URL":     {Routes: []string{"https://example.com/admin"}},
		"scheme-less":    {Routes: []string{"//example.com/admin"}},
		"fragment":       {Routes: []string{"/admin/status#database"}},
		"duplicates":     {Routes: []string{"/dashboard", "/dashboard"}},
		"too many":       {Routes: []string{"/1", "/2", "/3", "/4", "/5", "/6", "/7", "/8", "/9"}},
		"batch selector": {Routes: []string{"/one", "/two"}, Selector: "main"},
		"selector page":  {Routes: []string{"/one"}, Selector: "main", FullPage: true},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateScreenshotRequest(request); err == nil {
				t.Fatalf("request was accepted: %+v", request)
			}
		})
	}
}

func TestNormalizeScreenshotRequestUsesFastAgentDefaults(t *testing.T) {
	request := normalizeScreenshotRequest(ScreenshotRequest{})
	if strings.Join(request.Routes, ",") != "/dashboard" || request.Viewport != "desktop" || request.Theme != "light" || request.Scale != 1 || request.WaitMS != 200 {
		t.Fatalf("unexpected screenshot defaults: %+v", request)
	}
}

func TestValidateScreenshotResultRejectsManagedPathEscapes(t *testing.T) {
	outputDir := t.TempDir()
	inside := filepath.Join(outputDir, "capture.png")
	if err := os.WriteFile(inside, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := normalizeScreenshotRequest(ScreenshotRequest{Routes: []string{"/admin/status"}})
	result := ScreenshotResult{
		SchemaVersion: ScreenshotSchemaVersion,
		Viewport:      ScreenshotViewport{Name: "desktop", Width: 1440, Height: 1024, Scale: 1},
		Theme:         "light",
		Artifacts: []ScreenshotArtifact{{
			Route: "/admin/status", FinalRoute: "/admin/status", Path: inside, MIMEType: "image/png", Width: 1440, Height: 1024, Bytes: 3,
			SHA256: strings.Repeat("a", 64),
		}},
	}
	if err := validateScreenshotResult(outputDir, request, result); err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	result.Artifacts[0].Path = outside
	if err := validateScreenshotResult(outputDir, request, result); err == nil {
		t.Fatal("screenshot artifact outside managed output was accepted")
	}
}

func TestScreenshotCommandDetailIsBoundedAndRedacted(t *testing.T) {
	detail := screenshotCommandDetail("$ node helper\nPASSWORD=hunter2 capture failed\n")
	if detail != "PASSWORD=[redacted] capture failed" {
		t.Fatalf("unexpected screenshot detail: %q", detail)
	}
}
