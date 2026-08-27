// upgrade-fixture drives only public HTTP behavior for the cross-release smoke.
// Dockerfile never copies tests/ into a production image.
package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"hitkeep/internal/database"
)

const protocol = "upgrade-fixture-v1"

type manifest struct {
	SchemaVersion         int       `json:"schema_version"`
	Protocol              string    `json:"protocol"`
	SupportedUpgradeFloor string    `json:"supported_upgrade_floor"`
	Fixtures              []fixture `json:"fixtures"`
}

type fixture struct {
	PreviousImage       string `json:"previous_image"`
	PreviousChart       string `json:"previous_chart"`
	PreviousChartDigest string `json:"previous_chart_digest"`
	Platform            string `json:"platform"`
	PreviousVersion     string `json:"previous_version"`
	Fixture         struct {
		Email        string `json:"email"`
		Password     string `json:"password"`
		Domain       string `json:"domain"`
		Path         string `json:"path"`
		ExpectedHits int    `json:"expected_hits"`
		SessionID    string `json:"session_id"`
		PageID       string `json:"page_id"`
	} `json:"fixture"`
}

func (f fixture) chartValid() error {
	expected := "oci://ghcr.io/pascalebeier/charts/hitkeep:" + f.PreviousVersion
	if f.PreviousChart != expected {
		return fmt.Errorf("previous chart %q must be %q", f.PreviousChart, expected)
	}
	digest := strings.TrimPrefix(f.PreviousChartDigest, "sha256:")
	if len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
		return fmt.Errorf("previous chart digest %q is not immutable", f.PreviousChartDigest)
	}
	return nil
}

type imageInspection struct {
	OS           string `json:"Os"`
	Architecture string `json:"Architecture"`
	Config       struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
}

type fileMetadata struct {
	UID  int
	GID  int
	Mode int64
}

func main() {
	seed := flag.Bool("seed", false, "create the fixture through the previous image's HTTP API")
	verify := flag.Bool("verify", false, "verify the migrated fixture through the candidate HTTP API")
	verifyImage := flag.Bool("verify-image", false, "verify the pulled previous image identity")
	verifyStorageFlag := flag.Bool("verify-storage", false, "verify stopped candidate storage after migration")
	verifyLegacyStorageFlag := flag.Bool("verify-legacy-storage", false, "verify stopped restored pre-split storage")
	manifestPath := flag.String("manifest", "", "path to release fixture manifest")
	previousImage := flag.String("previous-image", "", "immutable previous image digest")
	platform := flag.String("platform", "", "Docker platform for the previous image")
	baseURL := flag.String("url", "", "HTTP base URL for the running image")
	image := flag.String("image", "", "pulled Docker image to inspect")
	dataPath := flag.String("data-path", "", "copied /var/lib/hitkeep/data directory")
	metadataPath := flag.String("metadata", "", "tar stream from docker cp for ownership and mode verification")
	flag.Parse()

	selected := 0
	for _, enabled := range []bool{*seed, *verify, *verifyImage, *verifyStorageFlag, *verifyLegacyStorageFlag} {
		if enabled {
			selected++
		}
	}
	if selected != 1 || strings.TrimSpace(*manifestPath) == "" || strings.TrimSpace(*previousImage) == "" || strings.TrimSpace(*platform) == "" {
		fatalf("exactly one action plus --manifest, --previous-image, and --platform is required")
	}
	ctx := context.Background()
	entry, err := loadFixture(*manifestPath, *previousImage, *platform)
	if err != nil {
		fatalf("loading upgrade fixture: %v", err)
	}
	switch {
	case *seed:
		if strings.TrimSpace(*baseURL) == "" {
			fatalf("--seed requires --url")
		}
		err = seedFixture(ctx, *baseURL, entry)
	case *verify:
		if strings.TrimSpace(*baseURL) == "" {
			fatalf("--verify requires --url")
		}
		err = verifyFixture(ctx, *baseURL, entry)
	case *verifyImage:
		if strings.TrimSpace(*image) == "" {
			fatalf("--verify-image requires --image")
		}
		err = verifyPulledImage(ctx, *image, entry)
	case *verifyStorageFlag:
		if strings.TrimSpace(*dataPath) == "" || strings.TrimSpace(*metadataPath) == "" {
			fatalf("--verify-storage requires --data-path and --metadata")
		}
		err = verifyStorage(ctx, *dataPath, *metadataPath, entry)
	case *verifyLegacyStorageFlag:
		if strings.TrimSpace(*dataPath) == "" || strings.TrimSpace(*metadataPath) == "" {
			fatalf("--verify-legacy-storage requires --data-path and --metadata")
		}
		err = verifyLegacyStorage(*dataPath, *metadataPath)
	}
	if err != nil {
		fatalf("upgrade fixture: %v", err)
	}
}

func loadFixture(path, previousImage, platform string) (fixture, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fixture{}, err
	}
	var parsed manifest
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fixture{}, fmt.Errorf("decoding manifest: %w", err)
	}
	if parsed.SchemaVersion != 1 || parsed.Protocol != protocol || strings.TrimSpace(parsed.SupportedUpgradeFloor) == "" {
		return fixture{}, fmt.Errorf("unsupported fixture manifest schema=%d protocol=%q", parsed.SchemaVersion, parsed.Protocol)
	}
	for _, entry := range parsed.Fixtures {
		if entry.PreviousImage == previousImage && entry.Platform == platform {
			if err := entry.valid(); err != nil {
				return fixture{}, err
			}
			if err := entry.chartValid(); err != nil {
				return fixture{}, err
			}
			return entry, nil
		}
	}
	return fixture{}, fmt.Errorf("no fixture declared for previous image %q on %q", previousImage, platform)
}

func (f fixture) valid() error {
	for _, value := range []string{f.PreviousImage, f.Platform, f.PreviousVersion, f.Fixture.Email, f.Fixture.Password, f.Fixture.Domain, f.Fixture.Path, f.Fixture.SessionID, f.Fixture.PageID} {
		if strings.TrimSpace(value) == "" {
			return errors.New("fixture contains an empty required value")
		}
	}
	if !strings.Contains(f.PreviousImage, "@sha256:") {
		return errors.New("previous image must be digest pinned")
	}
	if f.Fixture.ExpectedHits < 1 {
		return errors.New("fixture expected_hits must be positive")
	}
	return nil
}

func verifyPulledImage(ctx context.Context, image string, f fixture) error {
	raw, err := exec.CommandContext(ctx, "docker", "image", "inspect", image).Output()
	if err != nil {
		return fmt.Errorf("inspect pulled image: %w", err)
	}
	return verifyImageInspection(raw, f)
}

func verifyImageInspection(raw []byte, f fixture) error {
	var inspections []imageInspection
	if err := json.Unmarshal(raw, &inspections); err != nil {
		return fmt.Errorf("decode image inspection: %w", err)
	}
	if len(inspections) != 1 {
		return fmt.Errorf("image inspection count = %d, want 1", len(inspections))
	}
	inspection := inspections[0]
	if actual := inspection.OS + "/" + inspection.Architecture; actual != f.Platform {
		return fmt.Errorf("previous image platform = %q, want %q", actual, f.Platform)
	}
	if actual := inspection.Config.Labels["org.opencontainers.image.version"]; actual != f.PreviousVersion {
		return fmt.Errorf("previous image version = %q, want %q", actual, f.PreviousVersion)
	}
	return nil
}

func verifyLegacyStorage(dataPath, metadataPath string) error {
	controlPath := filepath.Join(dataPath, "hitkeep.db")
	if err := verifyRegularFile(controlPath); err != nil {
		return fmt.Errorf("control database: %w", err)
	}
	if _, err := os.Stat(filepath.Join(dataPath, "tenants")); err == nil {
		return errors.New("legacy storage contains split tenant data")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect legacy tenant directory: %w", err)
	}
	for _, path := range []string{filepath.Join(dataPath, "archive"), filepath.Join(dataPath, "recovery")} {
		if err := verifyCleanOptionalDirectory(path); err != nil {
			return err
		}
	}
	metadata, err := loadFileMetadata(metadataPath)
	if err != nil {
		return err
	}
	actual, ok := metadata["data/hitkeep.db"]
	if !ok {
		return errors.New("tar metadata omitted \"data/hitkeep.db\"")
	}
	if actual.UID != 65532 || actual.GID != 65532 || actual.Mode != 0644 {
		return fmt.Errorf("data/hitkeep.db ownership/mode = %d:%d:%#o, want 65532:65532:0644", actual.UID, actual.GID, actual.Mode)
	}
	return nil
}

func verifyStorage(ctx context.Context, dataPath, metadataPath string, f fixture) error {
	controlPath := filepath.Join(dataPath, "hitkeep.db")
	if err := verifyRegularFile(controlPath); err != nil {
		return fmt.Errorf("control database: %w", err)
	}
	store := database.NewStore(controlPath, database.WithAutomaticRecovery(false, filepath.Join(dataPath, "recovery")))
	if err := store.Connect(); err != nil {
		return fmt.Errorf("connect copied control database: %w", err)
	}
	defer store.Close()
	complete, err := store.DefaultTenantSplitComplete(ctx)
	if err != nil {
		return err
	}
	if !complete {
		return errors.New("default tenant split markers are incomplete")
	}
	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		return fmt.Errorf("resolve default tenant: %w", err)
	}
	tenantPath := filepath.Join(dataPath, "tenants", tenantID.String(), "hitkeep.db")
	if err := verifyRegularFile(tenantPath); err != nil {
		return fmt.Errorf("default tenant database: %w", err)
	}
	for _, path := range []string{filepath.Join(dataPath, "archive"), filepath.Join(dataPath, "recovery")} {
		if err := verifyCleanOptionalDirectory(path); err != nil {
			return err
		}
	}
	metadata, err := loadFileMetadata(metadataPath)
	if err != nil {
		return err
	}
	for _, path := range []string{"data/hitkeep.db", filepath.ToSlash(filepath.Join("data", "tenants", tenantID.String(), "hitkeep.db"))} {
		if actual, ok := metadata[path]; !ok {
			return fmt.Errorf("tar metadata omitted %q", path)
		} else if actual.UID != 65532 || actual.GID != 65532 || actual.Mode != 0644 {
			return fmt.Errorf("%s ownership/mode = %d:%d:%#o, want 65532:65532:0644", path, actual.UID, actual.GID, actual.Mode)
		}
	}
	return nil
}

func verifyRegularFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("not a non-empty regular file")
	}
	return nil
}

func verifyCleanOptionalDirectory(path string) error {
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("storage state %q: %w", path, err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("storage state %q contains unexpected entries", path)
	}
	return nil
}

func loadFileMetadata(path string) (map[string]fileMetadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	entries := make(map[string]fileMetadata)
	reader := tar.NewReader(file)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return entries, nil
		}
		if err != nil {
			return nil, err
		}
		entries[filepath.ToSlash(header.Name)] = fileMetadata{UID: header.Uid, GID: header.Gid, Mode: int64(header.FileInfo().Mode().Perm())}
	}
}

func seedFixture(ctx context.Context, baseURL string, f fixture) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	if err := doJSON(ctx, client, http.MethodPost, baseURL+"/api/initial-user", map[string]string{
		"email": f.Fixture.Email, "password": f.Fixture.Password, "given_name": "Issue", "last_name": "Fixture",
	}, http.StatusCreated, nil); err != nil {
		return fmt.Errorf("creating fixture user: %w", err)
	}
	var site struct {
		ID string `json:"id"`
	}
	if err := doJSON(ctx, client, http.MethodPost, baseURL+"/api/sites", map[string]string{"domain": f.Fixture.Domain}, http.StatusOK, &site); err != nil {
		return fmt.Errorf("creating fixture site: %w", err)
	}
	if strings.TrimSpace(site.ID) == "" {
		return errors.New("fixture site response omitted ID")
	}
	if err := doJSONWithHeaders(ctx, client, http.MethodPost, baseURL+"/ingest", map[string]string{
		"path": f.Fixture.Path, "session_id": f.Fixture.SessionID, "page_id": f.Fixture.PageID,
	}, http.StatusAccepted, nil, http.Header{"Origin": {"https://" + f.Fixture.Domain}}); err != nil {
		return fmt.Errorf("ingesting fixture hit: %w", err)
	}
	return awaitHits(ctx, client, baseURL, site.ID, f)
}

func verifyFixture(ctx context.Context, baseURL string, f fixture) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	if err := doJSON(ctx, client, http.MethodPost, baseURL+"/api/login", map[string]any{"email": f.Fixture.Email, "password": f.Fixture.Password, "remember_me": false}, http.StatusOK, nil); err != nil {
		return fmt.Errorf("logging in fixture user: %w", err)
	}
	var sites []struct {
		ID     string `json:"id"`
		Domain string `json:"domain"`
	}
	if err := doJSON(ctx, client, http.MethodGet, baseURL+"/api/sites", nil, http.StatusOK, &sites); err != nil {
		return fmt.Errorf("listing fixture sites: %w", err)
	}
	for _, site := range sites {
		if site.Domain == f.Fixture.Domain {
			return awaitHits(ctx, client, baseURL, site.ID, f)
		}
	}
	return fmt.Errorf("fixture site %q was not preserved", f.Fixture.Domain)
}

func awaitHits(ctx context.Context, client *http.Client, baseURL, siteID string, f fixture) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		var page struct {
			Data []struct {
				Path      string `json:"path"`
				SessionID string `json:"session_id"`
				PageID    string `json:"page_id"`
			} `json:"data"`
			Total int `json:"total"`
		}
		err := doJSON(ctx, client, http.MethodGet, baseURL+"/api/sites/"+siteID+"/hits?limit="+fmt.Sprint(f.Fixture.ExpectedHits), nil, http.StatusOK, &page)
		if err == nil && page.Total == f.Fixture.ExpectedHits && containsFixtureHit(page.Data, f) {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("reading fixture hits: %w", err)
			}
			return fmt.Errorf("fixture hits = total %d/data %#v, want exactly %d with path=%q session_id=%q page_id=%q", page.Total, page.Data, f.Fixture.ExpectedHits, f.Fixture.Path, f.Fixture.SessionID, f.Fixture.PageID)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func containsFixtureHit(hits []struct {
	Path      string `json:"path"`
	SessionID string `json:"session_id"`
	PageID    string `json:"page_id"`
}, f fixture) bool {
	for _, hit := range hits {
		if hit.Path == f.Fixture.Path && hit.SessionID == f.Fixture.SessionID && hit.PageID == f.Fixture.PageID {
			return true
		}
	}
	return false
}

func newClient() (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &http.Client{Jar: jar, Timeout: 10 * time.Second}, nil
}

func doJSON(ctx context.Context, client *http.Client, method, url string, input any, wantStatus int, output any) error {
	return doJSONWithHeaders(ctx, client, method, url, input, wantStatus, output, nil)
}

func doJSONWithHeaders(ctx context.Context, client *http.Client, method, url string, input any, wantStatus int, output any, headers http.Header) error {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return fmt.Errorf("%s %s: status %d, want %d: %s", method, req.URL.Path, response.StatusCode, wantStatus, strings.TrimSpace(string(message)))
	}
	if output != nil {
		return json.NewDecoder(response.Body).Decode(output)
	}
	return nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
