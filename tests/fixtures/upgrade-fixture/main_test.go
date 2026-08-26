package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSeedAndVerifyFixtureUseOnlyHTTPContracts(t *testing.T) {
	fixture := testFixture()
	var created, ingested bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, _ := r.Cookie("session")
		authenticated := cookie != nil && cookie.Value == "fixture"
		switch r.URL.Path {
		case "/api/initial-user":
			if r.Method != http.MethodPost {
				t.Fatalf("initial user method = %s", r.Method)
			}
			created = true
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "fixture"})
			w.WriteHeader(http.StatusCreated)
		case "/api/login":
			if r.Method != http.MethodPost || !created {
				t.Fatalf("unexpected login")
			}
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "fixture"})
			w.WriteHeader(http.StatusOK)
		case "/api/sites":
			if !authenticated {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.Method == http.MethodPost {
				json.NewEncoder(w).Encode(map[string]string{"id": "site-288"})
				return
			}
			json.NewEncoder(w).Encode([]map[string]string{{"id": "site-288", "domain": fixture.Fixture.Domain}})
		case "/ingest":
			if r.Header.Get("Origin") != "https://"+fixture.Fixture.Domain {
				t.Fatalf("origin = %q", r.Header.Get("Origin"))
			}
			ingested = true
			w.WriteHeader(http.StatusAccepted)
		case "/api/sites/site-288/hits":
			if !authenticated {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.URL.Query().Get("limit") != "1" {
				t.Fatalf("hits limit = %q", r.URL.Query().Get("limit"))
			}
			response := map[string]any{"total": boolToInt(ingested), "data": []map[string]string{}}
			if ingested {
				response["data"] = []map[string]string{{"path": fixture.Fixture.Path, "session_id": fixture.Fixture.SessionID, "page_id": fixture.Fixture.PageID}}
			}
			json.NewEncoder(w).Encode(response)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	if err := seedFixture(server.URL, fixture); err != nil {
		t.Fatal(err)
	}
	if err := verifyFixture(server.URL, fixture); err != nil {
		t.Fatal(err)
	}
}

func testFixture() fixture {
	var fixture fixture
	fixture.PreviousImage = "example.test/hitkeep@sha256:fixture"
	fixture.Platform = "linux/amd64"
	fixture.PreviousVersion = "2.12.0"
	fixture.Fixture.Email = "fixture@example.test"
	fixture.Fixture.Password = "fixture-password"
	fixture.Fixture.Domain = "fixture.example.test"
	fixture.Fixture.Path = "/fixture"
	fixture.Fixture.ExpectedHits = 1
	fixture.Fixture.SessionID = "10000000-0000-4000-8000-000000000288"
	fixture.Fixture.PageID = "20000000-0000-4000-8000-000000000288"
	return fixture
}

func TestReleaseFixtureManifestPinsIssue288UpgradeFloor(t *testing.T) {
	entry, err := loadFixture("../release-fixtures.json", "ghcr.io/pascalebeier/hitkeep:2.12.0@sha256:4c1ece6cb9a953847e2e2ff1414a540b107a223d59d2ad2aab1cbe68de6fadde", "linux/amd64")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Fixture.Domain != "issue-288-upgrade-fixture.invalid" || entry.PreviousVersion != "2.12.0" || entry.Fixture.ExpectedHits != 1 || entry.Fixture.SessionID != "10000000-0000-4000-8000-000000000288" || entry.Fixture.PageID != "20000000-0000-4000-8000-000000000288" {
		t.Fatalf("unexpected fixture: %#v", entry)
	}
	if _, err := loadFixture("../release-fixtures.json", "ghcr.io/pascalebeier/hitkeep:2.12.0@sha256:4c1ece6cb9a953847e2e2ff1414a540b107a223d59d2ad2aab1cbe68de6fadde", "linux/arm64"); err != nil {
		t.Fatalf("arm64 fixture: %v", err)
	}
	if _, err := loadFixture("../release-fixtures.json", "ghcr.io/pascalebeier/hitkeep:2.12.0", "linux/amd64"); err == nil {
		t.Fatal("expected unpinned image lookup to fail")
	}
}

func TestVerifyImageInspectionRequiresPinnedPlatformAndVersion(t *testing.T) {
	fixture := testFixture()
	raw := []byte(`[{"Os":"linux","Architecture":"amd64","Config":{"Labels":{"org.opencontainers.image.version":"2.12.0"}}}]`)
	if err := verifyImageInspection(raw, fixture); err != nil {
		t.Fatal(err)
	}
	if err := verifyImageInspection([]byte(`[{"Os":"linux","Architecture":"arm64","Config":{"Labels":{"org.opencontainers.image.version":"2.12.0"}}}]`), fixture); err == nil {
		t.Fatal("expected platform mismatch")
	}
	if err := verifyImageInspection([]byte(`[{"Os":"linux","Architecture":"amd64","Config":{"Labels":{"org.opencontainers.image.version":"2.12.1"}}}]`), fixture); err == nil {
		t.Fatal("expected version mismatch")
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
