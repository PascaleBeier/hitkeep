package hitkeepcmd

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hitkeep/internal/api"
)

const importCLIChunkSize = 8 << 20
const defaultImportAPIURL = "http://localhost:8080"

type repeatedStrings []string

func (r *repeatedStrings) String() string {
	return strings.Join(*r, ",")
}

func (r *repeatedStrings) Set(value string) error {
	*r = append(*r, value)
	return nil
}

func Import(args []string) {
	if len(args) == 0 {
		printImportUsage()
		os.Exit(2)
	}

	switch args[0] {
	case "validate":
		runImportValidate(args[1:])
	case "plausible":
		runImportProvider("plausible", args[1:])
	case "simpleanalytics":
		runImportProvider("simpleanalytics", args[1:])
	case "start":
		runImportStart(args[1:])
	case "status":
		runImportStatus(args[1:])
	case "list":
		runImportList(args[1:])
	case "delete":
		runImportDelete(args[1:])
	default:
		printImportUsage()
		os.Exit(2)
	}
}

func printImportUsage() {
	fmt.Fprintln(os.Stderr, `Usage:
  hitkeep import validate plausible --site <site-id> --file export.zip
  hitkeep import validate plausible --site <site-id> --file imported_visitors.csv --file imported_custom_events.csv
  hitkeep import validate plausible --site <site-id> --dir ./plausible-export
  hitkeep import validate simpleanalytics --site <site-id> --file datapoints.csv
  hitkeep import plausible --site <site-id> --file export.zip --wait
  hitkeep import simpleanalytics --site <site-id> --file datapoints.csv --wait
  hitkeep import start --site <site-id> --import-id <import-id> --wait
  hitkeep import status --site <site-id> --import-id <import-id>
  hitkeep import list --site <site-id>
  hitkeep import delete --site <site-id> --import-id <import-id>

Environment:
  HITKEEP_API_TOKEN  API client token with site.manage_data
  HITKEEP_PUBLIC_URL reused when present; otherwise defaults to http://localhost:8080
  HITKEEP_API_URL    optional compatibility override for remote API targets`)
}

func runImportValidate(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "validate requires an importer")
		os.Exit(2)
	}
	provider := args[0]
	opts := parseImportOptions(args[1:])
	client := newImportAPIClient(opts.apiURL, opts.token)
	job, err := client.uploadAndValidate(provider, opts.siteID, opts.paths())
	checkCLI(err)
	printImportJob(job)
}

func runImportProvider(provider string, args []string) {
	opts := parseImportOptions(args)
	client := newImportAPIClient(opts.apiURL, opts.token)
	job, err := client.uploadAndValidate(provider, opts.siteID, opts.paths())
	checkCLI(err)
	printImportJob(job)
	if !opts.yes && !confirmImport() {
		fmt.Fprintln(os.Stderr, "Import left validated but not started.")
		return
	}
	job, err = client.start(opts.siteID, job.ID.String())
	checkCLI(err)
	if opts.wait {
		job, err = client.wait(opts.siteID, job.ID.String())
		checkCLI(err)
	}
	printImportJob(job)
}

func runImportStart(args []string) {
	opts := parseImportOptions(args)
	if opts.importID == "" {
		fmt.Fprintln(os.Stderr, "--import-id is required")
		os.Exit(2)
	}
	client := newImportAPIClient(opts.apiURL, opts.token)
	job, err := client.start(opts.siteID, opts.importID)
	checkCLI(err)
	if opts.wait {
		job, err = client.wait(opts.siteID, opts.importID)
		checkCLI(err)
	}
	printImportJob(job)
}

func runImportStatus(args []string) {
	opts := parseImportOptions(args)
	if opts.importID == "" {
		fmt.Fprintln(os.Stderr, "--import-id is required")
		os.Exit(2)
	}
	client := newImportAPIClient(opts.apiURL, opts.token)
	job, err := client.get(opts.siteID, opts.importID)
	checkCLI(err)
	printImportJob(job)
}

func runImportList(args []string) {
	opts := parseImportOptions(args)
	client := newImportAPIClient(opts.apiURL, opts.token)
	list, err := client.list(opts.siteID)
	checkCLI(err)
	for _, job := range list.Imports {
		fmt.Printf("%s  %-16s  %-10s  %s\n", job.ID, job.Provider, job.Status, job.CreatedAt.Format(time.RFC3339))
	}
}

func runImportDelete(args []string) {
	opts := parseImportOptions(args)
	if opts.importID == "" {
		fmt.Fprintln(os.Stderr, "--import-id is required")
		os.Exit(2)
	}
	client := newImportAPIClient(opts.apiURL, opts.token)
	checkCLI(client.delete(opts.siteID, opts.importID))
	fmt.Println("Import deleted.")
}

type importCLIOptions struct {
	siteID   string
	importID string
	files    repeatedStrings
	dir      string
	apiURL   string
	token    string
	wait     bool
	yes      bool
}

func parseImportOptions(args []string) importCLIOptions {
	var opts importCLIOptions
	opts.apiURL = resolveImportAPIURL()
	opts.token = os.Getenv("HITKEEP_API_TOKEN")

	fs := flag.NewFlagSet("import", flag.ExitOnError)
	fs.StringVar(&opts.siteID, "site", "", "Site ID")
	fs.StringVar(&opts.importID, "import-id", "", "Import ID")
	fs.Var(&opts.files, "file", "ZIP or CSV file (repeatable)")
	fs.StringVar(&opts.dir, "dir", "", "Directory containing import CSV or ZIP files")
	fs.StringVar(&opts.apiURL, "url", opts.apiURL, "HitKeep base URL")
	fs.StringVar(&opts.apiURL, "api-url", opts.apiURL, "HitKeep API URL (deprecated alias for --url)")
	fs.StringVar(&opts.token, "token", opts.token, "API client token")
	fs.BoolVar(&opts.wait, "wait", false, "Wait for import completion")
	fs.BoolVar(&opts.yes, "yes", false, "Start without confirmation")
	_ = fs.Parse(args)
	opts.apiURL = normalizeImportAPIURL(opts.apiURL)

	if opts.siteID == "" {
		fmt.Fprintln(os.Stderr, "--site is required")
		os.Exit(2)
	}
	if opts.token == "" {
		fmt.Fprintln(os.Stderr, "--token or HITKEEP_API_TOKEN is required")
		os.Exit(2)
	}
	return opts
}

func (o importCLIOptions) paths() []string {
	paths := append([]string{}, o.files...)
	if o.dir != "" {
		entries, err := os.ReadDir(o.dir)
		checkCLI(err)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			ext := strings.ToLower(filepath.Ext(name))
			if ext == ".csv" || ext == ".zip" {
				paths = append(paths, filepath.Join(o.dir, name))
			}
		}
	}
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "At least one --file or --dir is required")
		os.Exit(2)
	}
	return paths
}

type importAPIClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func newImportAPIClient(baseURL, token string) *importAPIClient {
	return &importAPIClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		client:  &http.Client{Timeout: 0},
	}
}

func (c *importAPIClient) uploadAndValidate(provider, siteID string, paths []string) (*api.ImportJob, error) {
	files := make([]api.ImportUploadFileInput, 0, len(paths))
	for _, path := range paths {
		stat, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		sum, err := hashLocalImportFile(path)
		if err != nil {
			return nil, err
		}
		files = append(files, api.ImportUploadFileInput{Filename: filepath.Base(path), SizeBytes: stat.Size(), SHA256: sum})
	}
	var upload api.ImportUploadCreateResponse
	if err := c.doJSON(http.MethodPost, fmt.Sprintf("/api/sites/%s/imports/%s/uploads", siteID, provider), api.ImportUploadCreateRequest{Files: files}, &upload); err != nil {
		return nil, err
	}
	for idx, file := range upload.Files {
		if err := c.uploadFile(siteID, upload.ImportID.String(), file.ID.String(), paths[idx], upload.ChunkSize); err != nil {
			return nil, err
		}
	}
	var job api.ImportJob
	if err := c.doJSON(http.MethodPost, fmt.Sprintf("/api/sites/%s/imports/uploads/%s/validate", siteID, upload.ImportID), nil, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func hashLocalImportFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (c *importAPIClient) uploadFile(siteID, importID, fileID, path string, chunkSize int64) error {
	if chunkSize <= 0 {
		chunkSize = importCLIChunkSize
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	for offset := int64(0); offset < stat.Size(); offset += chunkSize {
		size := chunkSize
		if remaining := stat.Size() - offset; remaining < size {
			size = remaining
		}
		section := io.NewSectionReader(file, offset, size)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, fmt.Sprintf("%s/api/sites/%s/imports/uploads/%s/files/%s/chunks?offset=%d", c.baseURL, siteID, importID, fileID, offset), section)
		if err != nil {
			return err
		}
		req.ContentLength = size
		c.authorize(req)
		resp, err := c.client.Do(req)
		if err != nil {
			return err
		}
		if err := checkResponse(resp); err != nil {
			return err
		}
		_ = resp.Body.Close()
	}
	return nil
}

func (c *importAPIClient) start(siteID, importID string) (*api.ImportJob, error) {
	var job api.ImportJob
	err := c.doJSON(http.MethodPost, fmt.Sprintf("/api/sites/%s/imports/%s/start", siteID, importID), nil, &job)
	return &job, err
}

func (c *importAPIClient) get(siteID, importID string) (*api.ImportJob, error) {
	var job api.ImportJob
	err := c.doJSON(http.MethodGet, fmt.Sprintf("/api/sites/%s/imports/%s", siteID, importID), nil, &job)
	return &job, err
}

func (c *importAPIClient) list(siteID string) (*api.ImportListResponse, error) {
	var list api.ImportListResponse
	err := c.doJSON(http.MethodGet, fmt.Sprintf("/api/sites/%s/imports", siteID), nil, &list)
	return &list, err
}

func (c *importAPIClient) delete(siteID, importID string) error {
	return c.doJSON(http.MethodDelete, fmt.Sprintf("/api/sites/%s/imports/%s", siteID, importID), nil, nil)
}

func (c *importAPIClient) wait(siteID, importID string) (*api.ImportJob, error) {
	for {
		job, err := c.get(siteID, importID)
		if err != nil {
			return nil, err
		}
		switch job.Status {
		case "completed", "failed", "validation_failed", "deleted":
			return job, nil
		}
		time.Sleep(2 * time.Second)
	}
}

func (c *importAPIClient) doJSON(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.authorize(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	if err := checkResponse(resp); err != nil {
		return err
	}
	defer resp.Body.Close()
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *importAPIClient) authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token)
}

func checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
}

func printImportJob(job *api.ImportJob) {
	if job == nil {
		return
	}
	fmt.Printf("Import %s (%s): %s\n", job.ID, job.Provider, job.Status)
	if job.Error != "" {
		fmt.Printf("Error: %s\n", job.Error)
	}
	if job.Manifest != nil {
		m := job.Manifest
		fmt.Printf("Rows: scanned=%d accepted=%d skipped=%d\n", m.RowsScanned, m.RowsAccepted, m.RowsSkipped)
		if m.DateStart != nil && m.DateEnd != nil {
			fmt.Printf("Date range: %s to %s\n", m.DateStart.Format(time.DateOnly), m.DateEnd.Format(time.DateOnly))
		}
		if len(m.EventCoverage.EventNames) > 0 || m.EventCoverage.Events > 0 {
			fmt.Printf("Events: rows=%d events=%d names=%s\n", m.EventCoverage.RowsAccepted, m.EventCoverage.Events, strings.Join(m.EventCoverage.EventNames, ", "))
		}
		if m.EventPropertyCoverage.UnattributedRows > 0 || m.EventPropertyCoverage.AttributedRows > 0 {
			fmt.Printf("Event properties: attributed_rows=%d unattributed_rows=%d unattributed_events=%d\n", m.EventPropertyCoverage.AttributedRows, m.EventPropertyCoverage.UnattributedRows, m.EventPropertyCoverage.UnattributedEvents)
		}
		if len(m.EventDimensionCoverage.Unavailable) > 0 {
			fmt.Printf("Unavailable event dimensions: %s\n", strings.Join(m.EventDimensionCoverage.Unavailable, ", "))
		}
		if m.Overlap.Policy != "" && (m.Overlap.NativeTrafficDays > 0 || m.Overlap.NativeEventDays > 0 || m.Overlap.EstimatedSkippedRows > 0) {
			fmt.Printf("Overlap policy: %s skipped_rows=%d skipped_pageviews=%d skipped_events=%d\n", m.Overlap.Policy, m.Overlap.EstimatedSkippedRows, m.Overlap.EstimatedSkippedPageviews, m.Overlap.EstimatedSkippedEvents)
		}
		for _, warning := range m.Warnings {
			if warning.File != "" {
				fmt.Printf("Warning [%s] %s: %s\n", warning.Code, warning.File, warning.Message)
			} else {
				fmt.Printf("Warning [%s]: %s\n", warning.Code, warning.Message)
			}
		}
	}
}

func confirmImport() bool {
	fmt.Print("Start this import now? [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}

func resolveImportAPIURL() string {
	for _, key := range []string{"HITKEEP_API_URL", "HITKEEP_URL", "HITKEEP_PUBLIC_URL"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return normalizeImportAPIURL(value)
		}
	}
	return defaultImportAPIURL
}

func normalizeImportAPIURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultImportAPIURL
	}
	if !strings.Contains(value, "://") {
		if isLocalImportHost(value) {
			value = "http://" + value
		} else {
			value = "https://" + value
		}
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return strings.TrimRight(value, "/")
	}
	return strings.TrimRight(value, "/")
}

func isLocalImportHost(value string) bool {
	host := strings.ToLower(value)
	host = strings.TrimPrefix(host, "[")
	return strings.HasPrefix(host, "localhost") ||
		strings.HasPrefix(host, "127.") ||
		strings.HasPrefix(host, "::1") ||
		strings.HasPrefix(host, "[::1]")
}

func checkCLI(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
