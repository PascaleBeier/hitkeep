package devtool

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestReleaseCallerMatchesCurrentDocsV2Receiver(t *testing.T) {
	raw := readReleaseWorkflow(t)
	// The release workflow owns the reviewed pin; do not duplicate its digest here.
	pins := regexp.MustCompile(`(?m)^          DOCS_WORKFLOW_SHA256: ([a-f0-9]{64})$`).FindAllStringSubmatch(raw, -1)
	if len(pins) != 1 || strings.Count(raw, "attestation_schema=hitkeep.docs-attestation/v2") != 2 {
		t.Fatal("release caller must have one immutable docs receiver pin and use the v2 schema for both dispatches")
	}
	pin := pins[0][1]
	docsWorkflow, err := os.ReadFile(filepath.Join("..", "..", "hitkeep-docs", ".github", "workflows", "sync-hitkeep-release.yml"))
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("optional cross-repository check requires the private docs checkout; producer contracts are checked independently")
	}
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(docsWorkflow)
	want := hex.EncodeToString(digest[:])
	if pin != want {
		t.Fatalf("release workflow docs receiver pin = %s, want %s; review the receiver change before updating the workflow pin", pin, want)
	}
}

func TestPostpublicationFailureLeavesValidEarlyReceiptPending(t *testing.T) {
	raw := readReleaseWorkflow(t)
	script := releaseStepScript(t, raw, "sync-docs-release", "Dispatch hitkeep-docs release synchronization")
	sourceCommit := strings.Repeat("a", 40)
	docsBase := strings.Repeat("b", 40)
	sourceWorkflow := strings.Repeat("c", 64)
	docsWorkflow := strings.Repeat("d", 64)
	docsArtifact := strings.Repeat("e", 64)
	receipt := fmt.Sprintf(`{"schema_version":"hitkeep.docs-postpublication-receipt/v1","source":{"repository":"PascaleBeier/hitkeep","run_id":"1.1","commit":"%s","tag":"v1.2.3","workflow_sha256":"%s","catalog_sha256":"%s","example_sha256":"%s","manifest_sha256":"%s"},"docs":{"repository":"PascaleBeier/hitkeep-docs","base_sha":"%s","workflow_sha256":"%s","prepublication_run_id":10,"prepublication_run_attempt":2,"prepublication_attestation_artifact_sha256":"%s"},"receipt":{"run_id":42,"run_attempt":3}}`, sourceCommit, sourceWorkflow, strings.Repeat("f", 64), strings.Repeat("1", 64), strings.Repeat("2", 64), docsBase, docsWorkflow, docsArtifact)

	for _, scenario := range []struct {
		name        string
		token       string
		mode        string
		wantSuccess bool
	}{
		{name: "missing token remains recoverable", wantSuccess: true},
		{name: "dispatch failure remains recoverable", token: "token", mode: "dispatch-failure", wantSuccess: true},
		{name: "failed downstream rejects valid early receipt", token: "token", mode: "failed-downstream"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			receiptZip := filepath.Join(root, "receipt.zip")
			receiptDigest := writeReleaseZip(t, receiptZip, "hitkeep-docs-postpublication-receipt.json", receipt, false)
			runJSON := filepath.Join(root, "run.json")
			artifactsJSON := filepath.Join(root, "artifacts.json")
			writeReleaseFixture(t, runJSON, fmt.Sprintf(`{"id":42,"run_attempt":3,"repository":{"full_name":"PascaleBeier/hitkeep-docs"},"path":".github/workflows/sync-hitkeep-release.yml","event":"workflow_dispatch","head_branch":"main","head_sha":"%s","status":"completed","conclusion":"failure","html_url":"https://example.invalid/run/42"}`, docsBase))
			writeReleaseFixture(t, artifactsJSON, fmt.Sprintf(`{"artifacts":[{"id":7,"name":"hitkeep-docs-postpublication-receipt-42-3","expired":false,"digest":"sha256:%s"}]}`, receiptDigest))
			gh := filepath.Join(root, "gh")
			writeReleaseFixture(t, gh, strings.NewReplacer(
				"{{mode}}", scenario.mode,
				"{{run}}", runJSON,
				"{{artifacts}}", artifactsJSON,
				"{{receipt}}", receiptZip,
			).Replace(`#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  *"workflow run sync-hitkeep-release.yml"*)
    [[ "{{mode}}" == "dispatch-failure" ]] && exit 1
    exit 0
    ;;
  *"run list"*)
    printf '%s\n' '[{"databaseId":42,"displayTitle":"Finalize HitKeep v1.2.3 (publish) · source 1.1","createdAt":"2026-01-01T00:00:00Z"}]'
    ;;
  *"run watch"*)
    [[ "{{mode}}" == "failed-downstream" ]] && exit 1
    exit 0
    ;;
  *"api repos/PascaleBeier/hitkeep-docs/actions/runs/42/artifacts"*) cat "{{artifacts}}" ;;
  *"api repos/PascaleBeier/hitkeep-docs/actions/artifacts/7/zip"*) cat "{{receipt}}" ;;
  *"api repos/PascaleBeier/hitkeep-docs/actions/runs/42"*) cat "{{run}}" ;;
  *) echo "unexpected gh invocation: $*" >&2; exit 99 ;;
esac
`))
			if err := os.Chmod(gh, 0o755); err != nil {
				t.Fatal(err)
			}

			output := filepath.Join(root, "github-output")
			cmd := exec.Command("bash", "-c", script)
			cmd.Dir = root
			cmd.Env = releaseTestEnv(map[string]string{
				"PATH":                             root + string(os.PathListSeparator) + os.Getenv("PATH"),
				"GH_TOKEN":                         scenario.token,
				"GITHUB_OUTPUT":                    output,
				"GITHUB_RUN_ID":                    "1",
				"GITHUB_RUN_ATTEMPT":               "1",
				"GITHUB_SHA":                       sourceCommit,
				"DOWNSTREAM_REPOSITORY":            "PascaleBeier/hitkeep-docs",
				"TAG":                              "v1.2.3",
				"SOURCE_WORKFLOW_SHA256":           sourceWorkflow,
				"SOURCE_CATALOG_SHA256":            strings.Repeat("f", 64),
				"SOURCE_EXAMPLE_SHA256":            strings.Repeat("1", 64),
				"SOURCE_MANIFEST_SHA256":           strings.Repeat("2", 64),
				"DOCS_BASE_SHA":                    docsBase,
				"DOCS_WORKFLOW_SHA256":             docsWorkflow,
				"DOCS_RUN_ID":                      "10",
				"DOCS_RUN_ATTEMPT":                 "2",
				"DOCS_ATTESTATION_ARTIFACT_SHA256": docsArtifact,
			})
			combined, err := cmd.CombinedOutput()
			if gotSuccess := err == nil; gotSuccess != scenario.wantSuccess {
				t.Fatalf("success = %t, want %t: %v\n%s", gotSuccess, scenario.wantSuccess, err, combined)
			}
			state, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(state), "status=pending") || strings.Contains(string(state), "status=verified") {
				t.Fatalf("publication state must remain pending: %s", state)
			}
		})
	}
}

func TestDocsAttestationRejectsMismatchedArtifactZIPBeforeFinalization(t *testing.T) {
	raw := readReleaseWorkflow(t)
	script := releaseStepScript(t, raw, "docs-attestation", "Dispatch and verify exact documentation attestation")
	start := strings.Index(script, "artifact_name=")
	if start < 0 {
		t.Fatal("docs attestation has no artifact selection")
	}
	script = "set -euo pipefail\nworkdir=\"$WORKDIR\"\n" + script[start:]

	root := t.TempDir()
	sourceCommit := strings.Repeat("a", 40)
	docsBase := strings.Repeat("b", 40)
	docsWorkflow := strings.Repeat("d", 64)
	catalog := strings.Repeat("e", 64)
	example := strings.Repeat("f", 64)
	manifest := strings.Repeat("1", 64)
	attestation := fmt.Sprintf(`{"schema_version":"hitkeep.docs-attestation/v2","source":{"repository":"PascaleBeier/hitkeep","run_id":"1.1","commit":"%s","tag":"v1.2.3","catalog_sha256":"%s","example_sha256":"%s","manifest_sha256":"%s"},"docs":{"repository":"PascaleBeier/hitkeep-docs","base_sha":"%s","workflow_sha256":"%s","run_id":12,"run_attempt":3}}`, sourceCommit, catalog, example, manifest, docsBase, docsWorkflow)
	expectedZIP := filepath.Join(root, "expected.zip")
	expectedDigest := writeReleaseZip(t, expectedZIP, "hitkeep-docs-release-attestation.json", attestation, false)
	mismatchedZIP := filepath.Join(root, "mismatched.zip")
	writeReleaseZip(t, mismatchedZIP, "hitkeep-docs-release-attestation.json", attestation, true)
	artifactsJSON := filepath.Join(root, "artifacts.json")
	writeReleaseFixture(t, artifactsJSON, fmt.Sprintf(`{"artifacts":[{"id":7,"name":"hitkeep-docs-release-attestation-12-3","expired":false,"digest":"sha256:%s"}]}`, expectedDigest))
	gh := filepath.Join(root, "gh")
	writeReleaseFixture(t, gh, strings.NewReplacer(
		"{{artifacts}}", artifactsJSON,
		"{{mismatch}}", mismatchedZIP,
	).Replace(`#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  *"api repos/PascaleBeier/hitkeep-docs/actions/runs/12/artifacts"*) cat "{{artifacts}}" ;;
  *"api repos/PascaleBeier/hitkeep-docs/actions/artifacts/7/zip"*) cat "{{mismatch}}" ;;
  *"run download"*)
    dir=""
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --dir) dir="$2"; shift 2 ;;
        *) shift ;;
      esac
    done
    mkdir -p "$dir"
    unzip -p "{{mismatch}}" hitkeep-docs-release-attestation.json > "$dir/hitkeep-docs-release-attestation.json"
    ;;
  *) echo "unexpected gh invocation: $*" >&2; exit 99 ;;
esac
`))
	if err := os.Chmod(gh, 0o755); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(root, "github-output")
	workdir := filepath.Join(root, "work")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-c", script)
	cmd.Dir = root
	cmd.Env = releaseTestEnv(map[string]string{
		"PATH":                 root + string(os.PathListSeparator) + os.Getenv("PATH"),
		"WORKDIR":              workdir,
		"GITHUB_OUTPUT":        output,
		"TAG":                  "v1.2.3",
		"GITHUB_SHA":           sourceCommit,
		"DOCS_REPOSITORY":      "PascaleBeier/hitkeep-docs",
		"docs_run_id":          "12",
		"docs_run_attempt":     "3",
		"docs_head_sha":        docsBase,
		"source_run_id":        "1.1",
		"catalog_sha256":       catalog,
		"example_sha256":       example,
		"manifest_sha256":      manifest,
		"docs_workflow_sha256": docsWorkflow,
	})
	combined, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(combined), "prepublication attestation artifact digest mismatch") {
		t.Fatalf("mismatched artifact ZIP did not fail its digest check: %v\n%s", err, combined)
	}
	state, _ := os.ReadFile(output)
	if strings.Contains(string(state), "docs_attestation_artifact_sha256=") {
		t.Fatalf("mismatched artifact produced an attestation output: %s", state)
	}
}

func readReleaseWorkflow(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func releaseStepScript(t *testing.T, raw, job, name string) string {
	t.Helper()
	start := strings.Index(raw, "  "+job+":\n")
	if start < 0 {
		t.Fatalf("missing %s", job)
	}
	block := raw[start:]
	for index := 1; index+3 < len(block); index++ {
		if block[index] == '\n' && block[index+1] == ' ' && block[index+2] == ' ' && block[index+3] != ' ' {
			block = block[:index]
			break
		}
	}
	marker := "      - name: " + name + "\n"
	start = strings.Index(block, marker)
	if start < 0 {
		t.Fatalf("missing %s step", name)
	}
	block = block[start+len(marker):]
	start = strings.Index(block, "        run: |\n")
	if start < 0 {
		t.Fatalf("%s has no shell script", name)
	}
	block = block[start+len("        run: |\n"):]
	if end := strings.Index(block, "\n      - "); end >= 0 {
		block = block[:end]
	}
	lines := make([]string, 0)
	for line := range strings.SplitSeq(block, "\n") {
		lines = append(lines, strings.TrimPrefix(line, "          "))
	}
	return strings.Join(lines, "\n")
}

func writeReleaseFixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeReleaseZip(t *testing.T, path, name, contents string, extra bool) string {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entry, err := archive.Create(name)
	if err == nil {
		_, err = entry.Write([]byte(contents))
	}
	if err == nil && extra {
		entry, err = archive.Create("ignored")
		if err == nil {
			_, err = entry.Write([]byte("different ZIP bytes"))
		}
	}
	if closeErr := archive.Close(); err == nil {
		err = closeErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(bytes)
	return hex.EncodeToString(digest[:])
}

func releaseTestEnv(overrides map[string]string) []string {
	env := os.Environ()
	for key, value := range overrides {
		prefix := key + "="
		replaced := false
		for index, candidate := range env {
			if strings.HasPrefix(candidate, prefix) {
				env[index] = prefix + value
				replaced = true
				break
			}
		}
		if !replaced {
			env = append(env, prefix+value)
		}
	}
	return env
}
