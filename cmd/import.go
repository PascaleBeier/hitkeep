package hitkeepcmd

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	runtimeconfig "hitkeep/config"
	"hitkeep/internal/api"
	json "hitkeep/jsonapi"
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

func (*repeatedStrings) Type() string { return "stringSlice" }

type importCommand struct {
	ctx    context.Context
	in     io.Reader
	out    io.Writer
	errOut io.Writer
	apiURL string
	token  string
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

type importOperation func(importCommand, importCLIOptions, []string) error

func newImportCommandRoute(logger *slog.Logger) *cobra.Command {
	command := &cobra.Command{
		Use:           "import",
		Short:         "Import historical analytics data",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	command.AddCommand(
		newImportOperationCommand("validate <provider>", "Validate an import", cobra.ExactArgs(1), logger, func(c importCommand, opts importCLIOptions, args []string) error {
			return c.runValidate(args[0], opts)
		}),
		newImportOperationCommand("plausible", "Import Plausible analytics data", cobra.NoArgs, logger, func(c importCommand, opts importCLIOptions, _ []string) error {
			return c.runProvider("plausible", opts)
		}),
		newImportOperationCommand("simpleanalytics", "Import Simple Analytics data", cobra.NoArgs, logger, func(c importCommand, opts importCLIOptions, _ []string) error {
			return c.runProvider("simpleanalytics", opts)
		}),
		newImportOperationCommand("start", "Start a validated import", cobra.NoArgs, logger, func(c importCommand, opts importCLIOptions, _ []string) error {
			return c.runStart(opts)
		}),
		newImportOperationCommand("status", "Show import status", cobra.NoArgs, logger, func(c importCommand, opts importCLIOptions, _ []string) error {
			return c.runStatus(opts)
		}),
		newImportOperationCommand("list", "List imports", cobra.NoArgs, logger, func(c importCommand, opts importCLIOptions, _ []string) error {
			return c.runList(opts)
		}),
		newImportOperationCommand("delete", "Delete an import", cobra.NoArgs, logger, func(c importCommand, opts importCLIOptions, _ []string) error {
			return c.runDelete(opts)
		}),
	)
	return command
}

func newImportOperationCommand(use, short string, args cobra.PositionalArgs, logger *slog.Logger, operation importOperation) *cobra.Command {
	var opts importCLIOptions
	command := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  args,
		RunE: func(command *cobra.Command, args []string) error {
			importer, err := newImportExecutor(command.Context(), command.InOrStdin(), command.OutOrStdout(), command.ErrOrStderr(), rootConfigFile(command.Context()), logger)
			if err != nil {
				return err
			}
			opts, err = importer.resolveOptions(opts)
			if err != nil {
				return err
			}
			return operation(importer, opts, args)
		},
	}
	bindImportFlags(command, &opts)
	return command
}

func bindImportFlags(command *cobra.Command, opts *importCLIOptions) {
	command.Flags().StringVar(&opts.siteID, "site", "", "Site ID")
	command.Flags().StringVar(&opts.importID, "import-id", "", "Import ID")
	command.Flags().Var(&opts.files, "file", "ZIP or CSV file (repeatable)")
	command.Flags().StringVar(&opts.dir, "dir", "", "Directory containing import CSV or ZIP files")
	command.Flags().StringVar(&opts.apiURL, "url", "", "HitKeep base URL")
	command.Flags().StringVar(&opts.apiURL, "api-url", "", "HitKeep API URL (deprecated alias for --url)")
	command.Flags().StringVar(&opts.token, "token", "", "API client token")
	command.Flags().BoolVar(&opts.wait, "wait", false, "Wait for import completion")
	command.Flags().BoolVar(&opts.yes, "yes", false, "Start without confirmation")
}

func newImportExecutor(ctx context.Context, in io.Reader, out, errOut io.Writer, configFile string, logger *slog.Logger) (importCommand, error) {
	conf, err := runtimeconfig.LoadArgs(nil, configFile, logger)
	if err != nil {
		return importCommand{}, fmt.Errorf("load import configuration: %w", err)
	}
	return importCommand{
		ctx:    ctx,
		in:     in,
		out:    out,
		errOut: errOut,
		apiURL: normalizeImportAPIURL(cmp.Or(conf.ImportAPIURL, conf.PublicURL)),
		token:  conf.ImportAPIToken,
	}, nil
}

func (c importCommand) resolveOptions(opts importCLIOptions) (importCLIOptions, error) {
	opts.apiURL = normalizeImportAPIURL(cmp.Or(opts.apiURL, c.apiURL))
	opts.token = cmp.Or(opts.token, c.token)
	if opts.siteID == "" {
		_, _ = fmt.Fprintln(c.errOut, "--site is required")
		return opts, &ExitError{Code: 2}
	}
	if opts.token == "" {
		_, _ = fmt.Fprintln(c.errOut, "--token or HITKEEP_API_TOKEN is required")
		return opts, &ExitError{Code: 2}
	}
	return opts, nil
}

func (c importCommand) runValidate(provider string, opts importCLIOptions) error {
	paths, err := c.paths(opts)
	if err != nil {
		return err
	}
	job, err := newImportAPIClient(opts.apiURL, opts.token).uploadAndValidate(c.ctx, provider, opts.siteID, paths)
	if err := c.check(err); err != nil {
		return err
	}
	printImportJob(c.out, job)
	return nil
}

func (c importCommand) runProvider(provider string, opts importCLIOptions) error {
	client := newImportAPIClient(opts.apiURL, opts.token)
	paths, err := c.paths(opts)
	if err != nil {
		return err
	}
	job, err := client.uploadAndValidate(c.ctx, provider, opts.siteID, paths)
	if err := c.check(err); err != nil {
		return err
	}
	printImportJob(c.out, job)
	if !opts.yes && !confirmImport(c.in, c.out) {
		_, _ = fmt.Fprintln(c.errOut, "Import left validated but not started.")
		return nil
	}
	job, err = client.start(c.ctx, opts.siteID, job.ID.String())
	if err := c.check(err); err != nil {
		return err
	}
	if opts.wait {
		job, err = client.wait(c.ctx, opts.siteID, job.ID.String())
		if err := c.check(err); err != nil {
			return err
		}
	}
	printImportJob(c.out, job)
	return nil
}

func (c importCommand) runStart(opts importCLIOptions) error {
	if opts.importID == "" {
		_, _ = fmt.Fprintln(c.errOut, "--import-id is required")
		return &ExitError{Code: 2}
	}
	client := newImportAPIClient(opts.apiURL, opts.token)
	job, err := client.start(c.ctx, opts.siteID, opts.importID)
	if err := c.check(err); err != nil {
		return err
	}
	if opts.wait {
		job, err = client.wait(c.ctx, opts.siteID, opts.importID)
		if err := c.check(err); err != nil {
			return err
		}
	}
	printImportJob(c.out, job)
	return nil
}

func (c importCommand) runStatus(opts importCLIOptions) error {
	if opts.importID == "" {
		_, _ = fmt.Fprintln(c.errOut, "--import-id is required")
		return &ExitError{Code: 2}
	}
	job, err := newImportAPIClient(opts.apiURL, opts.token).get(c.ctx, opts.siteID, opts.importID)
	if err := c.check(err); err != nil {
		return err
	}
	printImportJob(c.out, job)
	return nil
}

func (c importCommand) runList(opts importCLIOptions) error {
	list, err := newImportAPIClient(opts.apiURL, opts.token).list(c.ctx, opts.siteID)
	if err := c.check(err); err != nil {
		return err
	}
	for _, job := range list.Imports {
		_, _ = fmt.Fprintf(c.out, "%s  %-16s  %-10s  %s\n", job.ID, job.Provider, job.Status, job.CreatedAt.Format(time.RFC3339))
	}
	return nil
}

func (c importCommand) runDelete(opts importCLIOptions) error {
	if opts.importID == "" {
		_, _ = fmt.Fprintln(c.errOut, "--import-id is required")
		return &ExitError{Code: 2}
	}
	if err := c.check(newImportAPIClient(opts.apiURL, opts.token).delete(c.ctx, opts.siteID, opts.importID)); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(c.out, "Import deleted.")
	return nil
}

func (c importCommand) paths(o importCLIOptions) ([]string, error) {
	paths := append([]string{}, o.files...)
	if o.dir != "" {
		entries, err := os.ReadDir(o.dir)
		if err := c.check(err); err != nil {
			return nil, err
		}
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
		_, _ = fmt.Fprintln(c.errOut, "At least one --file or --dir is required")
		return nil, &ExitError{Code: 2}
	}
	return paths, nil
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
		client:  &http.Client{},
	}
}

func (c *importAPIClient) uploadAndValidate(ctx context.Context, provider, siteID string, paths []string) (*api.ImportJob, error) {
	files := make([]api.ImportUploadFileInput, 0, len(paths))
	for _, path := range paths {
		stat, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat import file %s: %w", path, err)
		}
		sum, err := hashLocalImportFile(path)
		if err != nil {
			return nil, fmt.Errorf("hash import file %s: %w", path, err)
		}
		files = append(files, api.ImportUploadFileInput{Filename: filepath.Base(path), SizeBytes: stat.Size(), SHA256: sum})
	}
	var upload api.ImportUploadCreateResponse
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/sites/%s/imports/%s/uploads", siteID, provider), api.ImportUploadCreateRequest{Files: files}, &upload); err != nil {
		return nil, err
	}
	for idx, file := range upload.Files {
		if err := c.uploadFile(ctx, siteID, upload.ImportID.String(), file.ID.String(), paths[idx], upload.ChunkSize); err != nil {
			return nil, err
		}
	}
	var job api.ImportJob
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/sites/%s/imports/uploads/%s/validate", siteID, upload.ImportID), nil, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func hashLocalImportFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (c *importAPIClient) uploadFile(ctx context.Context, siteID, importID, fileID, path string, chunkSize int64) error {
	if chunkSize <= 0 {
		chunkSize = importCLIChunkSize
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open import file %s: %w", path, err)
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat import file %s: %w", path, err)
	}
	for offset := int64(0); offset < stat.Size(); offset += chunkSize {
		size := chunkSize
		if remaining := stat.Size() - offset; remaining < size {
			size = remaining
		}
		section := io.NewSectionReader(file, offset, size)
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("%s/api/sites/%s/imports/uploads/%s/files/%s/chunks?offset=%d", c.baseURL, siteID, importID, fileID, offset), section)
		if err != nil {
			return fmt.Errorf("build import chunk request: %w", err)
		}
		req.ContentLength = size
		c.authorize(req)
		resp, err := c.client.Do(req)
		if err != nil {
			return fmt.Errorf("upload import file %s at offset %d: %w", path, offset, err)
		}
		if err := checkResponse(resp); err != nil {
			return fmt.Errorf("upload import file %s at offset %d: %w", path, offset, err)
		}
		_ = resp.Body.Close()
	}
	return nil
}

func (c *importAPIClient) start(ctx context.Context, siteID, importID string) (*api.ImportJob, error) {
	var job api.ImportJob
	err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/sites/%s/imports/%s/start", siteID, importID), nil, &job)
	return &job, err
}

func (c *importAPIClient) get(ctx context.Context, siteID, importID string) (*api.ImportJob, error) {
	var job api.ImportJob
	err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/sites/%s/imports/%s", siteID, importID), nil, &job)
	return &job, err
}

func (c *importAPIClient) list(ctx context.Context, siteID string) (*api.ImportListResponse, error) {
	var list api.ImportListResponse
	err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/sites/%s/imports", siteID), nil, &list)
	return &list, err
}

func (c *importAPIClient) delete(ctx context.Context, siteID, importID string) error {
	return c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/api/sites/%s/imports/%s", siteID, importID), nil, nil)
}

func (c *importAPIClient) wait(ctx context.Context, siteID, importID string) (*api.ImportJob, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		job, err := c.get(ctx, siteID, importID)
		if err != nil {
			return nil, err
		}
		switch job.Status {
		case "completed", "failed", "validation_failed", "deleted":
			return job, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *importAPIClient) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode %s %s request: %w", method, path, err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build %s %s request: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.authorize(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send %s %s request: %w", method, path, err)
	}
	if err := checkResponse(resp); err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.UnmarshalRead(resp.Body, out); err != nil {
			return fmt.Errorf("decode %s %s response: %w", method, path, err)
		}
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

func printImportJob(out io.Writer, job *api.ImportJob) {
	if job == nil {
		return
	}
	_, _ = fmt.Fprintf(out, "Import %s (%s): %s\n", job.ID, job.Provider, job.Status)
	if job.Error != "" {
		_, _ = fmt.Fprintf(out, "Error: %s\n", job.Error)
	}
	if job.Manifest != nil {
		m := job.Manifest
		_, _ = fmt.Fprintf(out, "Rows: scanned=%d accepted=%d skipped=%d\n", m.RowsScanned, m.RowsAccepted, m.RowsSkipped)
		if m.DateStart != nil && m.DateEnd != nil {
			_, _ = fmt.Fprintf(out, "Date range: %s to %s\n", m.DateStart.Format(time.DateOnly), m.DateEnd.Format(time.DateOnly))
		}
		if len(m.EventCoverage.EventNames) > 0 || m.EventCoverage.Events > 0 {
			_, _ = fmt.Fprintf(out, "Events: rows=%d events=%d names=%s\n", m.EventCoverage.RowsAccepted, m.EventCoverage.Events, strings.Join(m.EventCoverage.EventNames, ", "))
		}
		if m.EventPropertyCoverage.UnattributedRows > 0 || m.EventPropertyCoverage.AttributedRows > 0 {
			_, _ = fmt.Fprintf(out, "Event properties: attributed_rows=%d unattributed_rows=%d unattributed_events=%d\n", m.EventPropertyCoverage.AttributedRows, m.EventPropertyCoverage.UnattributedRows, m.EventPropertyCoverage.UnattributedEvents)
		}
		if len(m.EventDimensionCoverage.Unavailable) > 0 {
			_, _ = fmt.Fprintf(out, "Unavailable event dimensions: %s\n", strings.Join(m.EventDimensionCoverage.Unavailable, ", "))
		}
		if m.Overlap.Policy != "" && (m.Overlap.NativeTrafficDays > 0 || m.Overlap.NativeEventDays > 0 || m.Overlap.EstimatedSkippedRows > 0) {
			_, _ = fmt.Fprintf(out, "Overlap policy: %s skipped_rows=%d skipped_pageviews=%d skipped_events=%d\n", m.Overlap.Policy, m.Overlap.EstimatedSkippedRows, m.Overlap.EstimatedSkippedPageviews, m.Overlap.EstimatedSkippedEvents)
		}
		for _, warning := range m.Warnings {
			if warning.File != "" {
				_, _ = fmt.Fprintf(out, "Warning [%s] %s: %s\n", warning.Code, warning.File, warning.Message)
			} else {
				_, _ = fmt.Fprintf(out, "Warning [%s]: %s\n", warning.Code, warning.Message)
			}
		}
	}
}

func confirmImport(in io.Reader, out io.Writer) bool {
	_, _ = fmt.Fprint(out, "Start this import now? [y/N] ")
	reader := bufio.NewReader(in)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
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

func (c importCommand) check(err error) error {
	if err == nil {
		return nil
	}
	_, _ = fmt.Fprintln(c.errOut, err)
	return &ExitError{Code: 1}
}
