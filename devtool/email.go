package devtool

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"
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
