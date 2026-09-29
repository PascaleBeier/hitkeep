package devtool

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"hitkeep/jsonapi"
)

// EmailClientsResult locates the rendered email review output.
type EmailClientsResult struct {
	Mailpit        string `json:"mailpit"`
	StartedMailpit bool   `json:"started_mailpit"`
	OutputDir      string `json:"output_dir"`
	ContactSheet   string `json:"contact_sheet"`
}

// RunEmailClients renders every email fixture, delivers it to the workspace
// Mailpit, and checks Mailpit's client compatibility report against the
// committed baseline. A running development session's Mailpit is reused;
// otherwise only the Compose mailpit service is started and stopped again.
func (a *App) RunEmailClients(ctx context.Context, writer io.Writer, update bool) (EmailClientsResult, error) {
	cloud, _ := VariantByID("cloud")
	outputDir := filepath.Join(a.workspace.StateDir, "artifacts", "email-preview")
	result := EmailClientsResult{
		Mailpit:      a.workspace.URLs.Mailpit,
		OutputDir:    outputDir,
		ContactSheet: filepath.Join(outputDir, "index.html"),
	}

	if !mailpitReady(ctx, result.Mailpit) {
		compose := commandSpec{Args: a.composeArgs("up", "-d", "--wait", "mailpit"), Env: a.ComposeEnvironment(cloud)}
		if err := a.runCommand(ctx, writer, compose); err != nil {
			return result, fmt.Errorf("start Mailpit: %w", err)
		}
		result.StartedMailpit = true
		defer func() {
			stop := commandSpec{Args: a.composeArgs("rm", "--stop", "--force", "mailpit"), Env: a.ComposeEnvironment(cloud)}
			_ = a.runCommand(context.WithoutCancel(ctx), writer, stop)
		}()
		if !waitForMailpit(ctx, result.Mailpit, 30*time.Second) {
			return result, fmt.Errorf("mailpit did not become ready at %s", result.Mailpit)
		}
	}

	mode := "-check"
	if update {
		mode = "-update"
	}
	args := []string{"go", "run", "./cmd/email-preview", "-out", outputDir, "-mailpit", result.Mailpit, mode}
	err := a.runCommand(ctx, writer, commandSpec{Args: args, Env: []string{"GOFLAGS=" + goFlagsForTags(cloud.BuildTags)}})
	return result, err
}

// EmailReviewResult locates the visual email review output.
type EmailReviewResult struct {
	BaseRef   string         `json:"base_ref,omitempty"`
	OutputDir string         `json:"output_dir"`
	Index     string         `json:"index"`
	Compared  bool           `json:"compared"`
	Counts    map[string]int `json:"counts"`
}

// RunEmailReview screenshots every email fixture for human review. With a
// base ref it also renders that commit (extracted with git archive, never a
// worktree) and writes before/after/diff images for changed screenshots.
func (a *App) RunEmailReview(ctx context.Context, writer io.Writer, baseRef string) (EmailReviewResult, error) {
	cloud, _ := VariantByID("cloud")
	goflags := []string{"GOFLAGS=" + goFlagsForTags(cloud.BuildTags)}
	outputDir := filepath.Join(a.workspace.StateDir, "artifacts", "email-review")
	result := EmailReviewResult{BaseRef: baseRef, OutputDir: outputDir, Index: filepath.Join(outputDir, "screenshots", "index.html")}
	if err := os.RemoveAll(outputDir); err != nil {
		return result, err
	}

	headRender := filepath.Join(outputDir, "head-render")
	if err := a.runCommand(ctx, writer, commandSpec{Args: []string{"go", "run", "./cmd/email-preview", "-out", headRender}, Env: goflags}); err != nil {
		return result, fmt.Errorf("render head emails: %w", err)
	}

	screenshots := []string{"node", "frontend/dashboard/scripts/email-screenshots.mjs", "--head", headRender, "--out", filepath.Join(outputDir, "screenshots")}
	if baseRef != "" {
		baseSource := filepath.Join(outputDir, "base-src")
		if err := a.extractTree(ctx, baseRef, baseSource); err != nil {
			return result, err
		}
		if _, err := os.Stat(filepath.Join(baseSource, "cmd", "email-preview")); err == nil {
			baseRender := filepath.Join(outputDir, "base-render")
			if err := a.runCommand(ctx, writer, commandSpec{Args: []string{"go", "-C", baseSource, "run", "./cmd/email-preview", "-out", baseRender}, Env: goflags}); err != nil {
				return result, fmt.Errorf("render %s emails: %w", baseRef, err)
			}
			screenshots = append(screenshots, "--base", baseRender)
		} else {
			_, _ = fmt.Fprintf(writer, "%s has no email preview command; showing current screenshots only\n", baseRef)
		}
	}

	if err := a.runCommand(ctx, writer, commandSpec{Args: screenshots}); err != nil {
		return result, fmt.Errorf("screenshot emails: %w", err)
	}
	var summary struct {
		Compared bool           `json:"compared"`
		Counts   map[string]int `json:"counts"`
	}
	data, err := os.ReadFile(filepath.Join(outputDir, "screenshots", "summary.json"))
	if err != nil {
		return result, err
	}
	if err := jsonapi.Unmarshal(data, &summary); err != nil {
		return result, err
	}
	result.Compared, result.Counts = summary.Compared, summary.Counts
	return result, nil
}

// extractTree writes the Go sources of ref into dir without touching the
// working tree or creating a worktree.
func (a *App) extractTree(ctx context.Context, ref, dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	// ref is passed as a single argument after --end-of-options; git validates it.
	archive := exec.CommandContext(ctx, "git", "archive", "--format=tar", "--end-of-options", ref, ".", ":(exclude)frontend", ":(exclude)docs") // #nosec G204
	archive.Dir = a.workspace.Root
	stdout, err := archive.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	archive.Stderr = &stderr
	if err := archive.Start(); err != nil {
		return err
	}
	extractErr := extractTar(stdout, dir)
	// Drain so git never blocks on a full pipe after an extraction error.
	_, _ = io.Copy(io.Discard, stdout)
	if err := archive.Wait(); err != nil {
		return fmt.Errorf("git archive %s: %w: %s", ref, err, strings.TrimSpace(stderr.String()))
	}
	return extractErr
}

func extractTar(reader io.Reader, dir string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	archive := tar.NewReader(reader)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(header.Name, 0o750); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := root.MkdirAll(path.Dir(header.Name), 0o750); err != nil {
				return err
			}
			file, err := root.OpenFile(header.Name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, archive) // #nosec G110 -- trusted local git archive
			closeErr := file.Close()
			if err := errors.Join(copyErr, closeErr); err != nil {
				return err
			}
		}
	}
}

func waitForMailpit(ctx context.Context, baseURL string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if mailpitReady(ctx, baseURL) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(500 * time.Millisecond):
		}
	}
	return false
}

func mailpitReady(ctx context.Context, baseURL string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/info", nil)
	if err != nil {
		return false
	}
	response, err := http.DefaultClient.Do(request) // #nosec G704 -- workspace-owned loopback Mailpit URL
	if err != nil {
		return false
	}
	_ = response.Body.Close()
	return response.StatusCode == http.StatusOK
}
