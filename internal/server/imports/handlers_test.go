package imports

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/importables"
	"hitkeep/internal/server/shared"
)

func setupImportHandlerTest(t *testing.T) (*handler, *database.Store, *api.Site) {
	t.Helper()

	tmpDir := t.TempDir()
	store := database.NewStore(filepath.Join(tmpDir, "hitkeep.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	userID, err := store.CreateUser(context.Background(), "import-api@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(context.Background(), userID, "import-api.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	ctx := &shared.Context{
		Store: store,
		Config: &config.Config{
			DataPath:            filepath.Join(tmpDir, "data"),
			ImportMaxStageBytes: 1 << 20,
		},
	}
	h := &handler{
		ctx: ctx,
		registry: importables.NewRegistry(
			importables.NewPlausibleProvider(),
			importables.NewSimpleAnalyticsProvider(),
		),
	}
	return h, store, site
}

type testUploadFile struct {
	filename string
	body     string
	sha256   string
}

func createImportUpload(t *testing.T, h *handler, site *api.Site, files []testUploadFile) api.ImportUploadCreateResponse {
	t.Helper()
	return createImportUploadForProvider(t, h, site, "plausible", files)
}

func createImportUploadForProvider(t *testing.T, h *handler, site *api.Site, provider string, files []testUploadFile) api.ImportUploadCreateResponse {
	t.Helper()

	inputs := make([]api.ImportUploadFileInput, 0, len(files))
	for _, file := range files {
		inputs = append(inputs, api.ImportUploadFileInput{
			Filename:  file.filename,
			SizeBytes: int64(len(file.body)),
			SHA256:    file.sha256,
		})
	}
	body, err := json.Marshal(api.ImportUploadCreateRequest{Files: inputs})
	if err != nil {
		t.Fatalf("marshal create upload request: %v", err)
	}
	createReq := httptest.NewRequest(http.MethodPost, "/api/sites/"+site.ID.String()+"/imports/"+provider+"/uploads", bytes.NewReader(body))
	createReq.SetPathValue("id", site.ID.String())
	createReq.SetPathValue("provider", provider)
	createW := httptest.NewRecorder()
	h.handleCreateUpload().ServeHTTP(createW, createReq)
	if createW.Code != http.StatusOK {
		t.Fatalf("create upload status = %d, body = %s", createW.Code, createW.Body.String())
	}

	var upload api.ImportUploadCreateResponse
	if err := json.NewDecoder(createW.Body).Decode(&upload); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	return upload
}

func uploadImportChunk(t *testing.T, h *handler, siteID, importID, fileID fmt.Stringer, offset int64, body string) *httptest.ResponseRecorder {
	t.Helper()

	chunkReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/sites/%s/imports/uploads/%s/files/%s/chunks?offset=%d", siteID, importID, fileID, offset), bytes.NewBufferString(body))
	chunkReq.SetPathValue("id", siteID.String())
	chunkReq.SetPathValue("importID", importID.String())
	chunkReq.SetPathValue("fileID", fileID.String())
	chunkW := httptest.NewRecorder()
	h.handleUploadChunk().ServeHTTP(chunkW, chunkReq)
	return chunkW
}

func uploadFileByName(t *testing.T, h *handler, site *api.Site, upload api.ImportUploadCreateResponse, filename string, body string) api.ImportUploadFile {
	t.Helper()

	for _, file := range upload.Files {
		if file.Filename != filename {
			continue
		}
		chunkW := uploadImportChunk(t, h, site.ID, upload.ImportID, file.ID, 0, body)
		if chunkW.Code != http.StatusOK {
			t.Fatalf("upload chunk %s status = %d, body = %s", file.Filename, chunkW.Code, chunkW.Body.String())
		}
		return file
	}
	t.Fatalf("file %s not found in upload response", filename)
	return api.ImportUploadFile{}
}

func validateImportUpload(t *testing.T, h *handler, site *api.Site, importID fmt.Stringer) (*httptest.ResponseRecorder, *api.ImportJob) {
	t.Helper()

	validateReq := httptest.NewRequest(http.MethodPost, "/api/sites/"+site.ID.String()+"/imports/uploads/"+importID.String()+"/validate", nil)
	validateReq.SetPathValue("id", site.ID.String())
	validateReq.SetPathValue("importID", importID.String())
	validateW := httptest.NewRecorder()
	h.handleValidateUpload().ServeHTTP(validateW, validateReq)
	if validateW.Code != http.StatusOK {
		return validateW, nil
	}
	var validated api.ImportJob
	if err := json.NewDecoder(validateW.Body).Decode(&validated); err != nil {
		t.Fatalf("decode validated import: %v", err)
	}
	return validateW, &validated
}

func TestImportUploadValidateRunAndDeleteLifecycle(t *testing.T) {
	h, store, site := setupImportHandlerTest(t)
	defer store.Close()

	visitorsCSV := "date,visitors,pageviews,bounces,visits,visit_duration\n2026-04-01,5,7,1,3,90\n"
	eventsCSV := "date,name,link_url,path,visitors,events\n2026-04-01,Signup,,/pricing,2,4\n"
	upload := createImportUpload(t, h, site, []testUploadFile{
		{filename: "imported_visitors.csv", body: visitorsCSV},
		{filename: "imported_custom_events.csv", body: eventsCSV},
	})
	if upload.ImportID.String() == "" || len(upload.Files) != 2 {
		t.Fatalf("unexpected upload response: %+v", upload)
	}

	chunks := map[string]string{
		"imported_visitors.csv":      visitorsCSV,
		"imported_custom_events.csv": eventsCSV,
	}
	for _, file := range upload.Files {
		body := chunks[file.Filename]
		chunkW := uploadImportChunk(t, h, site.ID, upload.ImportID, file.ID, 0, body)
		if chunkW.Code != http.StatusOK {
			t.Fatalf("upload chunk %s status = %d, body = %s", file.Filename, chunkW.Code, chunkW.Body.String())
		}
	}

	validateW, validated := validateImportUpload(t, h, site, upload.ImportID)
	if validateW.Code != http.StatusOK {
		t.Fatalf("validate status = %d, body = %s", validateW.Code, validateW.Body.String())
	}
	if validated.Status != database.ImportStatusValidated {
		t.Fatalf("expected validated status, got %s", validated.Status)
	}
	if validated.Manifest == nil || validated.Manifest.RowsScanned != 2 || validated.Manifest.RowsAccepted != 2 {
		t.Fatalf("unexpected manifest after validation: %+v", validated.Manifest)
	}
	if validated.SourceHash == "" {
		t.Fatalf("expected validation to pin a source hash")
	}

	start := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24*time.Hour - time.Nanosecond)
	names, err := store.GetEventNames(context.Background(), api.EventNamesParams{SiteID: site.ID, Start: start, End: end})
	if err != nil {
		t.Fatalf("event names before import: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("validation should not commit imported analytics rows, got names %v", names)
	}

	h.runImport(site.ID, upload.ImportID)
	completed, err := store.GetSiteImport(context.Background(), site.ID, upload.ImportID)
	if err != nil {
		t.Fatalf("get completed import: %v", err)
	}
	if completed.Status != database.ImportStatusCompleted || completed.RowsImported == 0 {
		t.Fatalf("expected completed import with imported rows, got %+v", completed)
	}

	names, err = store.GetEventNames(context.Background(), api.EventNamesParams{SiteID: site.ID, Start: start, End: end})
	if err != nil {
		t.Fatalf("event names after import: %v", err)
	}
	if len(names) != 1 || names[0] != "Signup" {
		t.Fatalf("expected imported event name to be queryable, got %v", names)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/sites/"+site.ID.String()+"/imports/"+upload.ImportID.String(), nil)
	deleteReq.SetPathValue("id", site.ID.String())
	deleteReq.SetPathValue("importID", upload.ImportID.String())
	deleteW := httptest.NewRecorder()
	h.handleDeleteImport().ServeHTTP(deleteW, deleteReq)
	if deleteW.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteW.Code, deleteW.Body.String())
	}

	names, err = store.GetEventNames(context.Background(), api.EventNamesParams{SiteID: site.ID, Start: start, End: end})
	if err != nil {
		t.Fatalf("event names after delete: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("delete should remove imported analytics rows, got names %v", names)
	}
}

func TestSimpleAnalyticsUploadValidateRunLifecycle(t *testing.T) {
	h, store, site := setupImportHandlerTest(t)
	defer store.Close()

	datapointsCSV := "added_iso,country_code,datapoint,device_type,document_referrer,duration_seconds,is_unique,path,session_id,utm_source,browser_name,browser_version,lang_language,lang_region\n" +
		"2026-04-01T10:00:00.000Z,DE,pageview,desktop,https://google.com/,12,true,/pricing,s1,chatgpt.com,Firefox,125.0,de,DE\n" +
		"2026-04-01T10:02:00.000Z,US,pageview,mobile,,4,false,/docs,s2,,Safari,17.4,en,US\n"
	upload := createImportUploadForProvider(t, h, site, "simpleanalytics", []testUploadFile{
		{filename: "2026-05-01_example_com_datapoints.csv", body: datapointsCSV},
	})
	uploadFileByName(t, h, site, upload, "2026-05-01_example_com_datapoints.csv", datapointsCSV)
	validateW, validated := validateImportUpload(t, h, site, upload.ImportID)
	if validateW.Code != http.StatusOK {
		t.Fatalf("validate status = %d, body = %s", validateW.Code, validateW.Body.String())
	}
	if validated.Manifest == nil || validated.Manifest.Provider != "simpleanalytics" || validated.Manifest.RowsAccepted != 2 {
		t.Fatalf("unexpected simple analytics manifest: %+v", validated.Manifest)
	}

	h.runImport(site.ID, upload.ImportID)
	completed, err := store.GetSiteImport(context.Background(), site.ID, upload.ImportID)
	if err != nil {
		t.Fatalf("get completed import: %v", err)
	}
	if completed.Status != database.ImportStatusCompleted || completed.RowsImported == 0 {
		t.Fatalf("expected completed simple analytics import, got %+v", completed)
	}

	start := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	stats, err := store.GetSiteStats(context.Background(), api.AnalyticsParams{SiteID: site.ID, Start: start, End: start.AddDate(0, 0, 7)})
	if err != nil {
		t.Fatalf("get site stats: %v", err)
	}
	if stats.TotalPageviews != 2 || stats.UniqueSessions != 1 {
		t.Fatalf("expected simple analytics traffic in stats, got pageviews=%d visitors=%d", stats.TotalPageviews, stats.UniqueSessions)
	}
	requireImportMetric(t, stats.TopPages, "pages", "/pricing", 1)
	requireImportMetric(t, stats.TopReferrers, "referrers", "google.com", 1)
	requireImportMetric(t, stats.TopUTMSources, "UTM sources", "chatgpt.com", 1)
	requireImportMetric(t, stats.TopBrowsers, "browsers", "Firefox", 1)
	requireImportMetric(t, stats.TopBrowsers, "browsers", "Safari", 1)
	requireImportMetric(t, stats.TopLanguages, "languages", "de", 1)
	requireImportMetric(t, stats.TopLanguages, "languages", "en", 1)
}

func TestStartImportUsesRunnerQueue(t *testing.T) {
	h, store, site := setupImportHandlerTest(t)
	defer store.Close()

	visitorsCSV := "date,visitors,pageviews,bounces,visits,visit_duration\n2026-04-01,5,7,1,3,90\n"
	upload := createImportUpload(t, h, site, []testUploadFile{{filename: "imported_visitors.csv", body: visitorsCSV}})
	uploadFileByName(t, h, site, upload, "imported_visitors.csv", visitorsCSV)
	validateW, _ := validateImportUpload(t, h, site, upload.ImportID)
	if validateW.Code != http.StatusOK {
		t.Fatalf("validate status = %d, body = %s", validateW.Code, validateW.Body.String())
	}

	runnerCtx := t.Context()
	h.runner = newImportRunner(h)
	h.runner.Start(runnerCtx)

	startReq := httptest.NewRequest(http.MethodPost, "/api/sites/"+site.ID.String()+"/imports/"+upload.ImportID.String()+"/start", nil)
	startReq.SetPathValue("id", site.ID.String())
	startReq.SetPathValue("importID", upload.ImportID.String())
	startW := httptest.NewRecorder()
	h.handleStartImport().ServeHTTP(startW, startReq)
	if startW.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", startW.Code, startW.Body.String())
	}

	completed := waitForImportStatus(t, store, site.ID, upload.ImportID, database.ImportStatusCompleted)
	if completed.RowsImported == 0 {
		t.Fatalf("expected runner to import rows, got %+v", completed)
	}
}

func TestRunImportIgnoresCompletedDuplicateQueueItem(t *testing.T) {
	h, store, site := setupImportHandlerTest(t)
	defer store.Close()

	visitorsCSV := "date,visitors,pageviews,bounces,visits,visit_duration\n2026-04-01,5,7,1,3,90\n"
	upload := createImportUpload(t, h, site, []testUploadFile{{filename: "imported_visitors.csv", body: visitorsCSV}})
	uploadFileByName(t, h, site, upload, "imported_visitors.csv", visitorsCSV)
	validateW, _ := validateImportUpload(t, h, site, upload.ImportID)
	if validateW.Code != http.StatusOK {
		t.Fatalf("validate status = %d, body = %s", validateW.Code, validateW.Body.String())
	}

	h.runImport(site.ID, upload.ImportID)
	completed, err := store.GetSiteImport(context.Background(), site.ID, upload.ImportID)
	if err != nil {
		t.Fatalf("get completed import: %v", err)
	}
	if completed == nil || completed.Status != database.ImportStatusCompleted || completed.RowsImported == 0 {
		t.Fatalf("expected first run to complete import, got %+v", completed)
	}

	h.runImport(site.ID, upload.ImportID)
	stillCompleted, err := store.GetSiteImport(context.Background(), site.ID, upload.ImportID)
	if err != nil {
		t.Fatalf("get duplicate import status: %v", err)
	}
	if stillCompleted == nil || stillCompleted.Status != database.ImportStatusCompleted || stillCompleted.Error != "" {
		t.Fatalf("duplicate run should leave completed import alone, got %+v", stillCompleted)
	}
}

func TestImportRunnerFailsRecoveredJobWithMissingStagedFiles(t *testing.T) {
	h, store, site := setupImportHandlerTest(t)
	defer store.Close()

	visitorsCSV := "date,visitors,pageviews,bounces,visits,visit_duration\n2026-04-01,5,7,1,3,90\n"
	upload := createImportUpload(t, h, site, []testUploadFile{{filename: "imported_visitors.csv", body: visitorsCSV}})
	uploadFileByName(t, h, site, upload, "imported_visitors.csv", visitorsCSV)
	validateW, _ := validateImportUpload(t, h, site, upload.ImportID)
	if validateW.Code != http.StatusOK {
		t.Fatalf("validate status = %d, body = %s", validateW.Code, validateW.Body.String())
	}
	files, err := store.ListImportFiles(context.Background(), upload.ImportID)
	if err != nil || len(files) != 1 {
		t.Fatalf("list import files: %v files=%d", err, len(files))
	}
	if err := os.Remove(h.stagedPath(files[0].RelativePath)); err != nil {
		t.Fatalf("remove staged file: %v", err)
	}
	if err := store.MarkImportQueued(context.Background(), site.ID, upload.ImportID); err != nil {
		t.Fatalf("mark queued: %v", err)
	}

	runnerCtx := t.Context()
	h.runner = newImportRunner(h)
	h.runner.Start(runnerCtx)

	failed := waitForImportStatus(t, store, site.ID, upload.ImportID, database.ImportStatusFailed)
	if !strings.Contains(failed.Error, "missing") {
		t.Fatalf("expected missing staged file error, got %+v", failed)
	}
}

func TestUploadChunkRejectsSparseOutOfOrderProgress(t *testing.T) {
	h, store, site := setupImportHandlerTest(t)
	defer store.Close()

	visitorsCSV := "date,visitors,pageviews,bounces,visits,visit_duration\n2026-04-01,5,7,1,3,90\n"
	upload := createImportUpload(t, h, site, []testUploadFile{{filename: "imported_visitors.csv", body: visitorsCSV}})
	file := upload.Files[0]

	chunkW := uploadImportChunk(t, h, site.ID, upload.ImportID, file.ID, int64(len(visitorsCSV)-1), visitorsCSV[len(visitorsCSV)-1:])
	if chunkW.Code != http.StatusConflict {
		t.Fatalf("expected sparse chunk to be rejected with 409, got %d body=%s", chunkW.Code, chunkW.Body.String())
	}

	staged, err := store.GetImportFile(context.Background(), upload.ImportID, file.ID)
	if err != nil {
		t.Fatalf("get import file: %v", err)
	}
	if staged.BytesReceived != 0 || staged.Status != database.ImportFileStatusPending {
		t.Fatalf("sparse chunk should not advance upload progress, got %+v", staged.ImportUploadFile)
	}
}

func waitForImportStatus(t *testing.T, store *database.Store, siteID, importID uuid.UUID, want string) *api.ImportJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last *api.ImportJob
	for time.Now().Before(deadline) {
		job, err := store.GetSiteImport(context.Background(), siteID, importID)
		if err != nil {
			t.Fatalf("get import: %v", err)
		}
		last = job
		if job.Status == want {
			return job
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for status %s, last=%+v", want, last)
	return last
}

func TestUploadChunkAllowsDuplicatePrefixRetry(t *testing.T) {
	h, store, site := setupImportHandlerTest(t)
	defer store.Close()

	visitorsCSV := "date,visitors,pageviews,bounces,visits,visit_duration\n2026-04-01,5,7,1,3,90\n"
	upload := createImportUpload(t, h, site, []testUploadFile{{filename: "imported_visitors.csv", body: visitorsCSV}})
	file := upload.Files[0]
	half := len(visitorsCSV) / 2

	first := uploadImportChunk(t, h, site.ID, upload.ImportID, file.ID, 0, visitorsCSV[:half])
	if first.Code != http.StatusOK {
		t.Fatalf("first chunk status = %d body=%s", first.Code, first.Body.String())
	}
	duplicate := uploadImportChunk(t, h, site.ID, upload.ImportID, file.ID, 0, visitorsCSV[:half])
	if duplicate.Code != http.StatusOK {
		t.Fatalf("duplicate chunk status = %d body=%s", duplicate.Code, duplicate.Body.String())
	}
	var duplicateResp api.ImportChunkResponse
	if err := json.NewDecoder(duplicate.Body).Decode(&duplicateResp); err != nil {
		t.Fatalf("decode duplicate response: %v", err)
	}
	if duplicateResp.BytesReceived != int64(half) || duplicateResp.Complete {
		t.Fatalf("duplicate chunk should keep prefix progress only, got %+v", duplicateResp)
	}

	second := uploadImportChunk(t, h, site.ID, upload.ImportID, file.ID, int64(half), visitorsCSV[half:])
	if second.Code != http.StatusOK {
		t.Fatalf("second chunk status = %d body=%s", second.Code, second.Body.String())
	}
	var secondResp api.ImportChunkResponse
	if err := json.NewDecoder(second.Body).Decode(&secondResp); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if !secondResp.Complete || secondResp.BytesReceived != int64(len(visitorsCSV)) {
		t.Fatalf("expected completed upload after second chunk, got %+v", secondResp)
	}
}

func TestUploadProgressDoesNotRegressAfterCompletion(t *testing.T) {
	h, store, site := setupImportHandlerTest(t)
	defer store.Close()

	visitorsCSV := "date,visitors,pageviews,bounces,visits,visit_duration\n2026-04-01,5,7,1,3,90\n"
	upload := createImportUpload(t, h, site, []testUploadFile{{filename: "imported_visitors.csv", body: visitorsCSV}})
	file := uploadFileByName(t, h, site, upload, "imported_visitors.csv", visitorsCSV)

	if err := store.UpdateImportFileProgress(context.Background(), upload.ImportID, file.ID, 1, ""); err != nil {
		t.Fatalf("regress import file progress: %v", err)
	}
	staged, err := store.GetImportFile(context.Background(), upload.ImportID, file.ID)
	if err != nil {
		t.Fatalf("get import file: %v", err)
	}
	if staged.BytesReceived != int64(len(visitorsCSV)) || staged.Status != database.ImportFileStatusUploaded {
		t.Fatalf("completed upload progress should be monotonic, got %+v", staged.ImportUploadFile)
	}
}

func TestValidateRejectsDeclaredChecksumMismatch(t *testing.T) {
	h, store, site := setupImportHandlerTest(t)
	defer store.Close()

	visitorsCSV := "date,visitors,pageviews,bounces,visits,visit_duration\n2026-04-01,5,7,1,3,90\n"
	upload := createImportUpload(t, h, site, []testUploadFile{{
		filename: "imported_visitors.csv",
		body:     visitorsCSV,
		sha256:   strings.Repeat("0", 64),
	}})
	uploadFileByName(t, h, site, upload, "imported_visitors.csv", visitorsCSV)

	validateW, _ := validateImportUpload(t, h, site, upload.ImportID)
	if validateW.Code != http.StatusBadRequest {
		t.Fatalf("expected checksum mismatch to fail validation with 400, got %d body=%s", validateW.Code, validateW.Body.String())
	}
	if !strings.Contains(validateW.Body.String(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch error, got %s", validateW.Body.String())
	}

	job, err := store.GetSiteImport(context.Background(), site.ID, upload.ImportID)
	if err != nil {
		t.Fatalf("get import: %v", err)
	}
	if job.Status != database.ImportStatusValidationFailed {
		t.Fatalf("expected validation_failed status, got %s", job.Status)
	}
}

func TestRunImportFailsWhenStagedFileChangesAfterValidation(t *testing.T) {
	h, store, site := setupImportHandlerTest(t)
	defer store.Close()

	visitorsCSV := "date,visitors,pageviews,bounces,visits,visit_duration\n2026-04-01,5,7,1,3,90\n"
	upload := createImportUpload(t, h, site, []testUploadFile{{filename: "imported_visitors.csv", body: visitorsCSV}})
	uploadFileByName(t, h, site, upload, "imported_visitors.csv", visitorsCSV)

	validateW, validated := validateImportUpload(t, h, site, upload.ImportID)
	if validateW.Code != http.StatusOK {
		t.Fatalf("validate status = %d, body = %s", validateW.Code, validateW.Body.String())
	}
	if validated.SourceHash == "" {
		t.Fatalf("expected source hash after validation")
	}

	files, err := store.ListImportFiles(context.Background(), upload.ImportID)
	if err != nil {
		t.Fatalf("list import files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one staged file, got %d", len(files))
	}
	mutated := strings.Replace(visitorsCSV, "2026-04-01", "2026-04-02", 1)
	if len(mutated) != len(visitorsCSV) {
		t.Fatalf("test mutation must preserve declared file size")
	}
	if err := os.WriteFile(h.stagedPath(files[0].RelativePath), []byte(mutated), 0600); err != nil {
		t.Fatalf("mutate staged file: %v", err)
	}

	h.runImport(site.ID, upload.ImportID)
	job, err := store.GetSiteImport(context.Background(), site.ID, upload.ImportID)
	if err != nil {
		t.Fatalf("get import: %v", err)
	}
	if job.Status != database.ImportStatusFailed {
		t.Fatalf("expected failed status after hash mismatch, got %+v", job)
	}
	if !strings.Contains(job.Error, "changed after validation") && !strings.Contains(job.Error, "checksum mismatch") {
		t.Fatalf("expected changed-after-validation or checksum mismatch error, got %q", job.Error)
	}
	if job.RowsImported != 0 {
		t.Fatalf("hash mismatch should not import rows, got %d", job.RowsImported)
	}
}

func TestValidateRejectsDuplicateCompletedSourceHash(t *testing.T) {
	h, store, site := setupImportHandlerTest(t)
	defer store.Close()

	visitorsCSV := "date,visitors,pageviews,bounces,visits,visit_duration\n2026-04-01,5,7,1,3,90\n"
	firstUpload := createImportUpload(t, h, site, []testUploadFile{{filename: "imported_visitors.csv", body: visitorsCSV}})
	uploadFileByName(t, h, site, firstUpload, "imported_visitors.csv", visitorsCSV)
	validateW, _ := validateImportUpload(t, h, site, firstUpload.ImportID)
	if validateW.Code != http.StatusOK {
		t.Fatalf("first validate status = %d, body = %s", validateW.Code, validateW.Body.String())
	}
	h.runImport(site.ID, firstUpload.ImportID)
	firstJob, err := store.GetSiteImport(context.Background(), site.ID, firstUpload.ImportID)
	if err != nil {
		t.Fatalf("get first import: %v", err)
	}
	if firstJob.Status != database.ImportStatusCompleted || firstJob.SourceHash == "" {
		t.Fatalf("expected first import completed with source hash, got %+v", firstJob)
	}

	secondUpload := createImportUpload(t, h, site, []testUploadFile{{filename: "traffic.csv", body: visitorsCSV}})
	uploadFileByName(t, h, site, secondUpload, "traffic.csv", visitorsCSV)
	validateW, _ = validateImportUpload(t, h, site, secondUpload.ImportID)
	if validateW.Code != http.StatusConflict {
		t.Fatalf("expected duplicate source validation to return 409, got %d body=%s", validateW.Code, validateW.Body.String())
	}
	if !strings.Contains(validateW.Body.String(), "already been imported") {
		t.Fatalf("expected duplicate import error, got %s", validateW.Body.String())
	}

	secondJob, err := store.GetSiteImport(context.Background(), site.ID, secondUpload.ImportID)
	if err != nil {
		t.Fatalf("get second import: %v", err)
	}
	if secondJob.Status != database.ImportStatusValidationFailed || secondJob.RowsImported != 0 {
		t.Fatalf("duplicate import should not validate or import rows, got %+v", secondJob)
	}
}

func TestRunImportSkipsOverlappingAggregateRows(t *testing.T) {
	h, store, site := setupImportHandlerTest(t)
	defer store.Close()

	firstCSV := "date,visitors,pageviews,bounces,visits,visit_duration\n2026-04-01,5,7,1,3,90\n"
	firstUpload := createImportUpload(t, h, site, []testUploadFile{{filename: "imported_visitors.csv", body: firstCSV}})
	uploadFileByName(t, h, site, firstUpload, "imported_visitors.csv", firstCSV)
	validateW, _ := validateImportUpload(t, h, site, firstUpload.ImportID)
	if validateW.Code != http.StatusOK {
		t.Fatalf("first validate status = %d, body = %s", validateW.Code, validateW.Body.String())
	}
	h.runImport(site.ID, firstUpload.ImportID)
	firstJob, err := store.GetSiteImport(context.Background(), site.ID, firstUpload.ImportID)
	if err != nil {
		t.Fatalf("get first import: %v", err)
	}
	if firstJob.Status != database.ImportStatusCompleted || firstJob.RowsImported != 1 {
		t.Fatalf("expected first import to write one row, got %+v", firstJob)
	}

	secondCSV := "date,visitors,pageviews,bounces,visits,visit_duration\n2026-04-01,8,14,2,4,120\n2026-04-02,6,9,1,3,95\n"
	secondUpload := createImportUpload(t, h, site, []testUploadFile{{filename: "imported_visitors.csv", body: secondCSV}})
	uploadFileByName(t, h, site, secondUpload, "imported_visitors.csv", secondCSV)
	validateW, _ = validateImportUpload(t, h, site, secondUpload.ImportID)
	if validateW.Code != http.StatusOK {
		t.Fatalf("second validate status = %d, body = %s", validateW.Code, validateW.Body.String())
	}
	h.runImport(site.ID, secondUpload.ImportID)
	secondJob, err := store.GetSiteImport(context.Background(), site.ID, secondUpload.ImportID)
	if err != nil {
		t.Fatalf("get second import: %v", err)
	}
	if secondJob.Status != database.ImportStatusCompleted || secondJob.RowsImported != 1 {
		t.Fatalf("expected second import to write only the non-overlapping row, got %+v", secondJob)
	}

	var count int64
	var pageviews int64
	if err := store.DB().QueryRowContext(context.Background(), `
		SELECT COUNT(*), COALESCE(SUM(pageviews), 0)
		FROM imported_traffic_daily
		WHERE site_id = ?
	`, site.ID).Scan(&count, &pageviews); err != nil {
		t.Fatalf("query imported traffic: %v", err)
	}
	if count != 2 || pageviews != 16 {
		t.Fatalf("expected two unique imported traffic rows with original overlap preserved, got count=%d pageviews=%d", count, pageviews)
	}
}

func TestImportPermissionAllowsSiteAdminsAndInstanceAdmins(t *testing.T) {
	h, store, site := setupImportHandlerTest(t)
	defer store.Close()
	ctx := context.Background()
	ownerID := site.UserID

	siteAdminID, err := store.CreateUser(ctx, "import-site-admin@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("create site admin: %v", err)
	}
	viewerID, err := store.CreateUser(ctx, "import-site-viewer@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("create site viewer: %v", err)
	}
	instanceAdminID, err := store.CreateUser(ctx, "import-instance-admin@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("create instance admin: %v", err)
	}
	tenantID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get site tenant: %v", err)
	}
	for _, userID := range []uuid.UUID{siteAdminID, viewerID} {
		if err := store.AddTeamMember(ctx, tenantID, userID, database.TenantRoleMember, ownerID); err != nil {
			t.Fatalf("add team member %s: %v", userID, err)
		}
	}
	if err := store.AddSiteMember(ctx, site.ID, siteAdminID, authcore.SiteAdmin, ownerID); err != nil {
		t.Fatalf("add site admin: %v", err)
	}
	if err := store.AddSiteMember(ctx, site.ID, viewerID, authcore.SiteViewer, ownerID); err != nil {
		t.Fatalf("add site viewer: %v", err)
	}
	if err := store.UpdateInstanceRole(ctx, instanceAdminID, authcore.InstanceAdmin, ownerID); err != nil {
		t.Fatalf("promote instance admin: %v", err)
	}

	gate := h.ctx.RequireSiteOrInstancePermission(authcore.PermSiteManageData, authcore.PermInstanceManageImports)
	cases := []struct {
		name string
		user uuid.UUID
		want int
	}{
		{name: "site admin", user: siteAdminID, want: http.StatusOK},
		{name: "instance admin", user: instanceAdminID, want: http.StatusOK},
		{name: "site viewer", user: viewerID, want: http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/sites/"+site.ID.String()+"/importers", nil)
			req.SetPathValue("id", site.ID.String())
			req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, tc.user))
			w := httptest.NewRecorder()

			gate(h.handleListImporters()).ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("expected status %d, got %d: %s", tc.want, w.Code, w.Body.String())
			}
			if tc.want == http.StatusOK {
				assertCompactImporterDescriptors(t, w.Body.Bytes())
			}
		})
	}
}

func assertCompactImporterDescriptors(t *testing.T, body []byte) {
	t.Helper()
	var descriptors []api.ImportProviderDescriptor
	if err := json.Unmarshal(body, &descriptors); err != nil {
		t.Fatalf("decode importers: %v", err)
	}
	requireImporterDescriptorCount(t, descriptors, 2)
	requireCompactImporterDescriptors(t, descriptors)
	requireNoImporterDescriptorCopy(t, string(body))
}

func requireImporterDescriptorCount(t *testing.T, descriptors []api.ImportProviderDescriptor, want int) {
	t.Helper()
	if len(descriptors) != want {
		t.Fatalf("expected %d importer descriptor(s), got %+v", want, descriptors)
	}
}

func requireCompactImporterDescriptor(t *testing.T, descriptor api.ImportProviderDescriptor) {
	t.Helper()
	if descriptor.Key == "" || descriptor.Name == "" || len(descriptor.AcceptedExtensions) == 0 {
		t.Fatalf("expected compact importer descriptor, got %+v", descriptor)
	}
}

func requireCompactImporterDescriptors(t *testing.T, descriptors []api.ImportProviderDescriptor) {
	t.Helper()
	requireCompactImporterDescriptor(t, descriptors[0])
	requireCompactImporterDescriptor(t, descriptors[1])
}

func requireNoImporterDescriptorCopy(t *testing.T, body string) {
	t.Helper()
	if strings.Contains(body, "description") || strings.Contains(body, "accepted_file_prefixes") {
		t.Fatalf("importer descriptors should not expose copy or filename prefixes: %s", body)
	}
}

func containsImportMetric(metrics []api.MetricStat, name string, value int) bool {
	for _, metric := range metrics {
		if metric.Name == name && metric.Value == value {
			return true
		}
	}
	return false
}

func requireImportMetric(t *testing.T, metrics []api.MetricStat, label string, name string, value int) {
	t.Helper()
	if !containsImportMetric(metrics, name, value) {
		t.Fatalf("expected %s metric %q=%d, got %+v", label, name, value, metrics)
	}
}
