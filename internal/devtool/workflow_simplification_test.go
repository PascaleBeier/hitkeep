package devtool

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestMCPPublicationWorkflowRejectsUnvalidatedPublisherExecution(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "publish-mcp.yml"))
	if err != nil {
		t.Fatal(err)
	}

	valid := string(raw)
	if got := publisherExecutions(valid); got != 1 {
		t.Fatalf("valid workflow would execute publisher %d times, want 1", got)
	}

	for name, broken := range map[string]string{
		"floating action ref":        strings.Replace(valid, "@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1", "@v7.0.1", 1),
		"wrong SHA/comment":          strings.Replace(valid, "# v7.0.1", "# v6.0.0", 1),
		"corrupted publisher bytes":  strings.Replace(valid, "a06c9096dcb9727c13555b6be26c7effa707b01f06a4c561ba7a3635443cf2cc", "bad", 1),
		"missing publisher checksum": strings.Replace(valid, "printf '%s  %s\\n' \"$publisher_sha256\" \"$publisher_archive\" | sha256sum --check --status", "", 1),
		"manual non-tag ref":         strings.Replace(valid, "^v[0-9]+\\.[0-9]+\\.[0-9]+$", "^main$", 1),
		"tag/server mismatch":        strings.Replace(valid, "server_version=\"$(jq -r '.version' server.json)\"", "server_version=wrong", 1),
		"validation bypass":          strings.Replace(valid, "if: needs.validate.result == 'success'", "if: always()", 1),
		"publisher command removed":  strings.Replace(valid, "run: ./mcp-publisher publish server.json", "run: true", 1),
		"publisher command in validate": strings.Replace(
			strings.Replace(valid, "run: ./mcp-publisher publish server.json", "run: true", 1),
			"run: ./hk qa pr --gate mcp-schema", "run: ./mcp-publisher publish server.json", 1,
		),
	} {
		t.Run(name, func(t *testing.T) {
			if got := publisherExecutions(broken); got != 0 {
				t.Fatalf("rejected workflow would execute publisher %d times, want 0", got)
			}
		})
	}
}

func TestReleaseValidationWorkflowSecurityContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release-validation.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for name, results := range map[string]map[string]string{
		"intentionally skipped work": {
			"homebrew-candidate": "skipped",
			"cross-cgo":          "skipped",
			"helm-lifecycle":     "skipped",
		},
		"selected work": {
			"homebrew-candidate": "success",
			"cross-cgo":          "success",
			"helm-lifecycle":     "success",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := releaseValidationRequiredCheck(string(raw), results); got != "success" {
				t.Fatalf("required check result = %q, want success", got)
			}
		})
	}
	for name, results := range map[string]map[string]string{
		"selected failure": {
			"homebrew-candidate": "success",
			"cross-cgo":          "failure",
			"helm-lifecycle":     "success",
		},
		"selected cancellation": {
			"homebrew-candidate": "success",
			"cross-cgo":          "cancelled",
			"helm-lifecycle":     "success",
		},
		"selector failure": {
			"select":             "failure",
			"homebrew-candidate": "skipped",
			"cross-cgo":          "skipped",
			"helm-lifecycle":     "skipped",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := releaseValidationRequiredCheck(string(raw), results); got == "success" {
				t.Fatal("required check incorrectly succeeds")
			}
		})
	}
	for name, broken := range map[string]string{
		"missing cancellation": strings.Replace(string(raw), "cancel-in-progress: true", "cancel-in-progress: false", 1),
		"package write":        strings.Replace(string(raw), "packages: read", "packages: write", 1),
		"OIDC":                 strings.Replace(string(raw), "contents: read", "contents: read\n      id-token: write", 1),
		"secret token":         strings.Replace(string(raw), "contents: read", "contents: read\n    env:\n      TOKEN: ${{ secrets.GHT }}", 1),
		"failure allowed":      strings.Replace(string(raw), "success|skipped) ;;", "success|skipped|failure) ;;", 1),
		"workflow path filter": strings.Replace(string(raw), "pull_request:\n", "pull_request:\n    paths:\n      - Dockerfile\n", 1),
		"selector output":      strings.Replace(string(raw), "steps.paths.outputs.selected", "steps.paths.outputs.other", 1),
		"selector gate":        strings.Replace(string(raw), "needs.select.outputs.selected == 'true'", "true", 1),
		"selector token scope": strings.Replace(
			string(raw),
			"if: github.event_name == 'pull_request'\n    runs-on:",
			"if: github.event_name == 'pull_request'\n    env:\n      GITHUB_TOKEN: ${{ github.token }}\n    runs-on:",
			1,
		),
		"selector pagination":         strings.Replace(string(raw), "page > 100", "page > 0", 1),
		"selector changed-file count": strings.Replace(string(raw), "github.event.pull_request.changed_files", "0", 1),
		"selector file cap":           strings.Replace(string(raw), "CHANGED_FILES > 3000", "CHANGED_FILES > 3001", 1),
		"selector response type":      strings.Replace(string(raw), "type == \"array\"", "type == \"object\"", 1),
		"selector observed count":     strings.Replace(string(raw), "observed == CHANGED_FILES", "true", 1),
		"selector exact path":         strings.Replace(string(raw), "Dockerfile", "Containerfile", 1),
		"selector prefix":             strings.Replace(string(raw), "frontend/dashboard/", "frontend/app/", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if got := releaseValidationRequiredCheck(broken, map[string]string{
				"homebrew-candidate": "success",
				"cross-cgo":          "success",
				"helm-lifecycle":     "success",
			}); got == "success" {
				t.Fatal("unsafe workflow was accepted")
			}
		})
	}

	snapshot, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "snapshot.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !snapshotPublicationIsTrusted(string(snapshot)) {
		t.Fatal("snapshot publication accepts pull-request work or can be cancelled")
	}
}

func releaseValidationRequiredCheck(raw string, results map[string]string) string {
	var workflow map[string]any
	if yaml.Unmarshal([]byte(raw), &workflow) != nil || !releaseValidationSelectsPaths(workflow) {
		return ""
	}
	concurrency, ok := workflow["concurrency"].(map[string]any)
	if !ok || concurrency["group"] != "${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}" || concurrency["cancel-in-progress"] != true {
		return ""
	}
	jobs, ok := workflow["jobs"].(map[string]any)
	if !ok {
		return ""
	}
	selected := []string{"homebrew-candidate", "cross-cgo", "helm-lifecycle"}
	for _, name := range append(selected, "required") {
		job, ok := jobs[name].(map[string]any)
		if !ok || !jobIsReadOnly(job) || jobContainsSecret(job) {
			return ""
		}
	}
	required, ok := jobs["required"].(map[string]any)
	if !ok || fmt.Sprint(required["if"]) != "always()" || fmt.Sprint(required["needs"]) != "[select homebrew-candidate cross-cgo helm-lifecycle]" {
		return ""
	}
	steps, ok := required["steps"].([]any)
	if !ok || len(steps) != 1 {
		return ""
	}
	run, ok := steps[0].(map[string]any)["run"].(string)
	if !ok || !strings.Contains(run, "success|skipped) ;;") || !strings.Contains(run, "exit 1") {
		return ""
	}
	for _, name := range append([]string{"select"}, selected...) {
		if !strings.Contains(run, "needs."+name+".result") {
			return ""
		}
		result := results[name]
		if result == "" {
			result = "success"
		}
		if result != "success" && result != "skipped" {
			return result
		}
	}
	return "success"
}

func releaseValidationSelectsPaths(workflow map[string]any) bool {
	trigger, ok := workflow["on"].(map[string]any)
	if !ok || trigger["pull_request"] != nil {
		return false
	}
	jobs, ok := workflow["jobs"].(map[string]any)
	if !ok {
		return false
	}
	selectJob, ok := jobs["select"].(map[string]any)
	if !ok || selectJob["if"] != "github.event_name == 'pull_request'" || selectJob["env"] != nil {
		return false
	}
	permissions, ok := selectJob["permissions"].(map[string]any)
	if !ok || len(permissions) != 1 || permissions["pull-requests"] != "read" {
		return false
	}
	outputs, ok := selectJob["outputs"].(map[string]any)
	if !ok || outputs["selected"] != "${{ steps.paths.outputs.selected }}" {
		return false
	}
	steps, ok := selectJob["steps"].([]any)
	if !ok || len(steps) != 1 {
		return false
	}
	step, ok := steps[0].(map[string]any)
	if !ok || step["id"] != "paths" {
		return false
	}
	env, ok := step["env"].(map[string]any)
	if !ok || env["GITHUB_TOKEN"] != "${{ github.token }}" || env["PR_NUMBER"] != "${{ github.event.pull_request.number }}" || env["CHANGED_FILES"] != "${{ github.event.pull_request.changed_files }}" {
		return false
	}
	run, ok := step["run"].(string)
	if !ok || !strings.Contains(run, "/pulls/$PR_NUMBER/files?per_page=100&page=$page") || !strings.Contains(run, "page=$((page + 1))") || !strings.Contains(run, "page > 100") || !strings.Contains(run, "[[ \"$CHANGED_FILES\" =~ ^[0-9]+$ ]]") || !strings.Contains(run, "CHANGED_FILES > 3000") || !strings.Contains(run, "type == \"array\"") || !strings.Contains(run, "observed=$((observed + count))") || !strings.Contains(run, "observed == CHANGED_FILES") || !strings.Contains(run, "Dockerfile") || !strings.Contains(run, "frontend/dashboard/") {
		return false
	}
	for _, name := range []string{"homebrew-candidate", "cross-cgo", "helm-lifecycle"} {
		job, ok := jobs[name].(map[string]any)
		if !ok || job["needs"] != "select" || !strings.Contains(fmt.Sprint(job["if"]), "needs.select.outputs.selected == 'true'") {
			return false
		}
	}
	return true
}

func jobIsReadOnly(job map[string]any) bool {
	permissions, ok := job["permissions"].(map[string]any)
	if !ok {
		return false
	}
	for _, value := range permissions {
		if value == "write" {
			return false
		}
	}
	return permissions["id-token"] != "write"
}

func jobContainsSecret(value any) bool {
	switch value := value.(type) {
	case string:
		return strings.Contains(value, "secrets.")
	case map[string]any:
		for _, child := range value {
			if jobContainsSecret(child) {
				return true
			}
		}
	case []any:
		if slices.ContainsFunc(value, jobContainsSecret) {
			return true
		}
	}
	return false
}

func snapshotPublicationIsTrusted(raw string) bool {
	var workflow map[string]any
	if yaml.Unmarshal([]byte(raw), &workflow) != nil {
		return false
	}
	trigger, ok := workflow["on"].(map[string]any)
	if !ok || trigger["pull_request"] != nil {
		return false
	}
	concurrency, ok := workflow["concurrency"].(map[string]any)
	return ok && concurrency["cancel-in-progress"] == false
}

func publisherExecutions(raw string) int {
	if !allExternalActionsArePinned(raw) || !publisherDownloadIsVerified(raw) || !manualPublicationUsesTag(raw) || !tagMatchesServerVersion(raw) {
		return 0
	}

	var workflow map[string]any
	if yaml.Unmarshal([]byte(raw), &workflow) != nil {
		return 0
	}
	jobs, ok := workflow["jobs"].(map[string]any)
	if !ok || len(jobs) != 2 {
		return 0
	}
	validate, ok := jobs["validate"].(map[string]any)
	if !ok || hasOIDC(validate) {
		return 0
	}
	publish, ok := jobs["publish"].(map[string]any)
	if !ok || !hasOIDC(publish) || fmt.Sprint(publish["needs"]) != "validate" || fmt.Sprint(publish["if"]) != "needs.validate.result == 'success'" {
		return 0
	}
	if publisherCommandCount(validate, "./mcp-publisher ") != 0 || publisherCommandCount(publish, "./mcp-publisher publish ") != 1 {
		return 0
	}
	return 1
}

func publisherCommandCount(job map[string]any, prefix string) int {
	steps, ok := job["steps"].([]any)
	if !ok {
		return 0
	}
	count := 0
	for _, step := range steps {
		run, ok := step.(map[string]any)["run"].(string)
		if !ok {
			continue
		}
		for line := range strings.SplitSeq(run, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), prefix) {
				count++
			}
		}
	}
	return count
}

func allExternalActionsArePinned(raw string) bool {
	for want, count := range map[string]int{
		"actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1": 2,
		"actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0": 1,
	} {
		if strings.Count(raw, want) != count {
			return false
		}
	}
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- uses:") || strings.Contains(line, "$/.github/") {
			continue
		}
		if !regexp.MustCompile(`^-[[:space:]]+uses:[[:space:]]+[^@[:space:]]+@[0-9a-f]{40}[[:space:]]+# v[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(line) {
			return false
		}
	}
	return true
}

func publisherDownloadIsVerified(raw string) bool {
	return strings.Contains(raw, "mcp-publisher_linux_amd64.tar.gz") &&
		strings.Contains(raw, "publisher_sha256=\"a06c9096dcb9727c13555b6be26c7effa707b01f06a4c561ba7a3635443cf2cc\"") &&
		strings.Contains(raw, "curl --fail --silent --show-error --location \"$publisher_url\" --output \"$publisher_archive\"") &&
		strings.Contains(raw, "printf '%s  %s\\n' \"$publisher_sha256\" \"$publisher_archive\" | sha256sum --check --status") &&
		strings.Index(raw, "sha256sum --check --status") < strings.Index(raw, "tar xz")
}

func manualPublicationUsesTag(raw string) bool {
	return strings.Contains(raw, "release_tag:") &&
		strings.Contains(raw, "^v[0-9]+\\.[0-9]+\\.[0-9]+$") &&
		strings.Contains(raw, "refs/tags/$release_tag")
}

func tagMatchesServerVersion(raw string) bool {
	return strings.Contains(raw, "server_version=\"$(jq -r '.version' server.json)\"") &&
		strings.Contains(raw, `[[ "$tag_version" != "$server_version" ]]`)
}

func hasOIDC(job map[string]any) bool {
	permissions, ok := job["permissions"].(map[string]any)
	return ok && permissions["id-token"] == "write"
}
