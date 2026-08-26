// upgrade-fixture drives only public HTTP behavior for the cross-release smoke.
// Dockerfile never copies tests/ into a production image.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"time"
)

const protocol = "upgrade-fixture-v1"

type manifest struct {
	SchemaVersion         int       `json:"schema_version"`
	Protocol              string    `json:"protocol"`
	SupportedUpgradeFloor string    `json:"supported_upgrade_floor"`
	Fixtures              []fixture `json:"fixtures"`
}

type fixture struct {
	PreviousImage string `json:"previous_image"`
	Platform      string `json:"platform"`
	Fixture       struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		Domain      string `json:"domain"`
		Path        string `json:"path"`
		MinimumHits int    `json:"minimum_hits"`
	} `json:"fixture"`
}

func main() {
	seed := flag.Bool("seed", false, "create the fixture through the previous image's HTTP API")
	verify := flag.Bool("verify", false, "verify the migrated fixture through the candidate HTTP API")
	manifestPath := flag.String("manifest", "", "path to release fixture manifest")
	previousImage := flag.String("previous-image", "", "immutable previous image digest")
	platform := flag.String("platform", "", "Docker platform for the previous image")
	baseURL := flag.String("url", "", "HTTP base URL for the running image")
	flag.Parse()

	if *seed == *verify || strings.TrimSpace(*manifestPath) == "" || strings.TrimSpace(*previousImage) == "" || strings.TrimSpace(*platform) == "" || strings.TrimSpace(*baseURL) == "" {
		fatalf("exactly one of --seed or --verify plus --manifest, --previous-image, --platform, and --url is required")
	}
	entry, err := loadFixture(*manifestPath, *previousImage, *platform)
	if err != nil {
		fatalf("loading upgrade fixture: %v", err)
	}
	if *seed {
		err = seedFixture(*baseURL, entry)
	} else {
		err = verifyFixture(*baseURL, entry)
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
			return entry, nil
		}
	}
	return fixture{}, fmt.Errorf("no fixture declared for previous image %q on %q", previousImage, platform)
}

func (f fixture) valid() error {
	for _, value := range []string{f.PreviousImage, f.Platform, f.Fixture.Email, f.Fixture.Password, f.Fixture.Domain, f.Fixture.Path} {
		if strings.TrimSpace(value) == "" {
			return errors.New("fixture contains an empty required value")
		}
	}
	if !strings.Contains(f.PreviousImage, "@sha256:") {
		return errors.New("previous image must be digest pinned")
	}
	if f.Fixture.MinimumHits < 1 {
		return errors.New("fixture minimum_hits must be positive")
	}
	return nil
}

func seedFixture(baseURL string, f fixture) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	if err := doJSON(client, http.MethodPost, baseURL+"/api/initial-user", map[string]string{
		"email": f.Fixture.Email, "password": f.Fixture.Password, "given_name": "Issue", "last_name": "Fixture",
	}, http.StatusCreated, nil); err != nil {
		return fmt.Errorf("creating fixture user: %w", err)
	}
	var site struct {
		ID string `json:"id"`
	}
	if err := doJSON(client, http.MethodPost, baseURL+"/api/sites", map[string]string{"domain": f.Fixture.Domain}, http.StatusOK, &site); err != nil {
		return fmt.Errorf("creating fixture site: %w", err)
	}
	if strings.TrimSpace(site.ID) == "" {
		return errors.New("fixture site response omitted ID")
	}
	if err := doJSONWithHeaders(client, http.MethodPost, baseURL+"/ingest", map[string]string{
		"path": f.Fixture.Path, "session_id": "10000000-0000-4000-8000-000000000288", "page_id": "20000000-0000-4000-8000-000000000288",
	}, http.StatusAccepted, nil, http.Header{"Origin": {"https://" + f.Fixture.Domain}}); err != nil {
		return fmt.Errorf("ingesting fixture hit: %w", err)
	}
	return awaitHits(client, baseURL, site.ID, f.Fixture.MinimumHits)
}

func verifyFixture(baseURL string, f fixture) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	if err := doJSON(client, http.MethodPost, baseURL+"/api/login", map[string]any{"email": f.Fixture.Email, "password": f.Fixture.Password, "remember_me": false}, http.StatusOK, nil); err != nil {
		return fmt.Errorf("logging in fixture user: %w", err)
	}
	var sites []struct {
		ID     string `json:"id"`
		Domain string `json:"domain"`
	}
	if err := doJSON(client, http.MethodGet, baseURL+"/api/sites", nil, http.StatusOK, &sites); err != nil {
		return fmt.Errorf("listing fixture sites: %w", err)
	}
	for _, site := range sites {
		if site.Domain == f.Fixture.Domain {
			return awaitHits(client, baseURL, site.ID, f.Fixture.MinimumHits)
		}
	}
	return fmt.Errorf("fixture site %q was not preserved", f.Fixture.Domain)
}

func awaitHits(client *http.Client, baseURL, siteID string, minimum int) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		var page struct {
			Total int `json:"total"`
		}
		err := doJSON(client, http.MethodGet, baseURL+"/api/sites/"+siteID+"/hits?limit=1", nil, http.StatusOK, &page)
		if err == nil && page.Total >= minimum {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("reading fixture hits: %w", err)
			}
			return fmt.Errorf("fixture hit count = %d, want at least %d", page.Total, minimum)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func newClient() (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &http.Client{Jar: jar, Timeout: 10 * time.Second}, nil
}

func doJSON(client *http.Client, method, url string, input any, wantStatus int, output any) error {
	return doJSONWithHeaders(client, method, url, input, wantStatus, output, nil)
}

func doJSONWithHeaders(client *http.Client, method, url string, input any, wantStatus int, output any, headers http.Header) error {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, body)
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
