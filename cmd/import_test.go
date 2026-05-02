package hitkeepcmd

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func TestImportAPIClientUploadAndValidateStreamsRepeatedFiles(t *testing.T) {
	dir := t.TempDir()
	firstBody := "date,visitors,pageviews,bounces,visits,visit_duration\n2026-04-01,5,7,1,3,90\n"
	secondBody := "date,name,link_url,path,visitors,events\n2026-04-01,Signup,,/pricing,2,4\n"
	firstHash := sha256.Sum256([]byte(firstBody))
	secondHash := sha256.Sum256([]byte(secondBody))
	firstPath := filepath.Join(dir, "imported_visitors.csv")
	secondPath := filepath.Join(dir, "imported_custom_events.csv")
	if err := os.WriteFile(firstPath, []byte(firstBody), 0600); err != nil {
		t.Fatalf("write first csv: %v", err)
	}
	if err := os.WriteFile(secondPath, []byte(secondBody), 0600); err != nil {
		t.Fatalf("write second csv: %v", err)
	}

	siteID := uuid.New().String()
	importID := uuid.New()
	fileIDs := []uuid.UUID{uuid.New(), uuid.New()}
	uploaded := map[string]string{}
	chunkOffsets := map[string][]int64{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("unexpected auth header %q", got)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/sites/"+siteID+"/imports/plausible/uploads":
			var req api.ImportUploadCreateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode create upload request: %v", err)
			}
			if len(req.Files) != 2 {
				t.Fatalf("expected two files, got %+v", req.Files)
			}
			if req.Files[0].Filename != "imported_visitors.csv" || req.Files[0].SizeBytes != int64(len(firstBody)) {
				t.Fatalf("unexpected first file metadata: %+v", req.Files[0])
			}
			if req.Files[0].SHA256 != fmt.Sprintf("%x", firstHash) {
				t.Fatalf("unexpected first checksum: %q", req.Files[0].SHA256)
			}
			if req.Files[1].Filename != "imported_custom_events.csv" || req.Files[1].SizeBytes != int64(len(secondBody)) {
				t.Fatalf("unexpected second file metadata: %+v", req.Files[1])
			}
			if req.Files[1].SHA256 != fmt.Sprintf("%x", secondHash) {
				t.Fatalf("unexpected second checksum: %q", req.Files[1].SHA256)
			}
			writeTestJSON(t, w, api.ImportUploadCreateResponse{
				ImportID:  importID,
				Provider:  "plausible",
				Status:    "uploading",
				ChunkSize: 11,
				Files: []api.ImportUploadFile{
					{ID: fileIDs[0], Filename: req.Files[0].Filename, SizeBytes: req.Files[0].SizeBytes},
					{ID: fileIDs[1], Filename: req.Files[1].Filename, SizeBytes: req.Files[1].SizeBytes},
				},
			})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/sites/"+siteID+"/imports/uploads/"+importID.String()+"/files/"):
			parts := strings.Split(r.URL.Path, "/")
			fileID := parts[len(parts)-2]
			offset, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
			if err != nil {
				t.Fatalf("parse offset: %v", err)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read chunk body: %v", err)
			}
			chunkOffsets[fileID] = append(chunkOffsets[fileID], offset)
			uploaded[fileID] += string(body)
			writeTestJSON(t, w, api.ImportChunkResponse{ImportID: importID, FileID: uuid.MustParse(fileID), BytesReceived: int64(len(uploaded[fileID])), Complete: len(body) < 11})
		case r.Method == http.MethodPost && r.URL.Path == "/api/sites/"+siteID+"/imports/uploads/"+importID.String()+"/validate":
			if uploaded[fileIDs[0].String()] != firstBody {
				t.Fatalf("unexpected uploaded first body: %q", uploaded[fileIDs[0].String()])
			}
			if uploaded[fileIDs[1].String()] != secondBody {
				t.Fatalf("unexpected uploaded second body: %q", uploaded[fileIDs[1].String()])
			}
			writeTestJSON(t, w, api.ImportJob{
				ID:        importID,
				SiteID:    uuid.MustParse(siteID),
				Provider:  "plausible",
				Status:    "validated",
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newImportAPIClient(server.URL, "test-token")
	job, err := client.uploadAndValidate("plausible", siteID, []string{firstPath, secondPath})
	if err != nil {
		t.Fatalf("upload and validate: %v", err)
	}
	if job.Status != "validated" {
		t.Fatalf("expected validated job, got %+v", job)
	}
	if len(chunkOffsets[fileIDs[0].String()]) < 2 || chunkOffsets[fileIDs[0].String()][0] != 0 {
		t.Fatalf("expected first file to upload in chunks from offset 0, got %v", chunkOffsets[fileIDs[0].String()])
	}
	if len(chunkOffsets[fileIDs[1].String()]) < 2 || chunkOffsets[fileIDs[1].String()][0] != 0 {
		t.Fatalf("expected second file to upload in chunks from offset 0, got %v", chunkOffsets[fileIDs[1].String()])
	}
}

func TestResolveImportAPIURLUsesExistingHitKeepConfig(t *testing.T) {
	t.Setenv("HITKEEP_API_URL", "")
	t.Setenv("HITKEEP_URL", "")
	t.Setenv("HITKEEP_PUBLIC_URL", "https://analytics.example.com/")

	if got := resolveImportAPIURL(); got != "https://analytics.example.com" {
		t.Fatalf("expected public URL to drive import API URL, got %q", got)
	}
}

func TestResolveImportAPIURLPrecedence(t *testing.T) {
	t.Setenv("HITKEEP_API_URL", "https://api.example.com/")
	t.Setenv("HITKEEP_URL", "https://short.example.com/")
	t.Setenv("HITKEEP_PUBLIC_URL", "https://public.example.com/")

	if got := resolveImportAPIURL(); got != "https://api.example.com" {
		t.Fatalf("expected HITKEEP_API_URL to win for compatibility, got %q", got)
	}
}

func TestNormalizeImportAPIURL(t *testing.T) {
	tests := map[string]string{
		"":                       defaultImportAPIURL,
		"localhost:8080":         "http://localhost:8080",
		"127.0.0.1:8080/":        "http://127.0.0.1:8080",
		"analytics.example.com/": "https://analytics.example.com",
		"https://example.com/":   "https://example.com",
	}

	for input, want := range tests {
		if got := normalizeImportAPIURL(input); got != want {
			t.Fatalf("normalizeImportAPIURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
