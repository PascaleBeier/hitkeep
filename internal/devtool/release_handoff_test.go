package devtool

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const reviewedDocsV2Pin = "812c7896202be86362a5c384c17b8e87c461fac1353ec44210c79b15b4e58c08"

type releaseGraph struct {
	Jobs map[string]releaseGraphJob
}

type releaseGraphJob struct {
	Needs []string
	If    string
}

type docsSubject struct {
	Schema, Base, Workflow, RunID, Attempt, Artifact string
}

func TestReleaseGraphBlocksNonSuccessAndUntrustedHandoffs(t *testing.T) {
	workflow, raw := parseReleaseGraph(t)
	outcomes := map[string]string{
		"release-please": "success", "build-release": "success", "upgrade-from-supported-floor": "success",
		"publish-helm": "success", "verify-tracker-package": "success", "docs-attestation": "success", "finalize-release": "success",
	}
	subject := docsSubject{Schema: "hitkeep.docs-attestation/v2", Base: strings.Repeat("a", 40), Workflow: reviewedDocsV2Pin, RunID: "1", Attempt: "1", Artifact: strings.Repeat("b", 64)}
	checksum := strings.Repeat("c", 64)
	if finalize, deploy := releaseAdmission(workflow, outcomes, subject, checksum); !finalize || !deploy {
		t.Fatalf("all-success trusted handoffs must admit finalization and deployment: subject=%t docs=%#v finalize=%#v deploy=%#v", validDocsSubject(subject), workflow.Jobs["docs-attestation"], workflow.Jobs["finalize-release"], workflow.Jobs["deploy-cloud"])
	}
	for _, want := range []string{
		"DOCS_WORKFLOW_SHA256: " + reviewedDocsV2Pin,
		"attestation_schema=hitkeep.docs-attestation/v2",
		"docs_attestation_artifact_sha256=\"${DOCS_ATTESTATION_ARTIFACT_SHA256}\"",
		"--pattern checksums.txt",
		"artifact_sha256=\"${ARTIFACT_SHA256}\"",
		"gh run watch \"$downstream_run_id\"",
		".run_attempt == $run_attempt",
		"source_catalog_sha256=\"${SOURCE_CATALOG_SHA256}\"",
		"docs_run_attempt=\"${DOCS_RUN_ATTEMPT}\"",
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("release workflow is missing trusted handoff %q", want)
		}
	}

	for _, job := range []string{"docs-attestation", "finalize-release", "deploy-cloud"} {
		for _, prerequisite := range workflow.Jobs[job].Needs {
			for _, result := range []string{"failure", "cancelled", "skipped"} {
				mutated := maps.Clone(outcomes)
				mutated[prerequisite] = result
				finalize, deploy := releaseAdmission(workflow, mutated, subject, checksum)
				if job == "docs-attestation" || job == "finalize-release" {
					if finalize {
						t.Errorf("%s %s must block finalization", prerequisite, result)
					}
				}
				if deploy {
					t.Errorf("%s %s must block cloud deployment", prerequisite, result)
				}
			}
		}
	}

	for _, invalid := range []docsSubject{
		{Schema: "hitkeep.docs-attestation/v2", Base: strings.Repeat("a", 40), Workflow: strings.Repeat("d", 64), RunID: "1", Attempt: "1", Artifact: strings.Repeat("b", 64)},
		{Schema: "hitkeep.docs-attestation/v2", Base: strings.Repeat("a", 40), Workflow: reviewedDocsV2Pin, RunID: "1", Attempt: "1", Artifact: "not-a-digest"},
		{Schema: "hitkeep.docs-attestation/v2", Base: strings.Repeat("a", 40), Workflow: reviewedDocsV2Pin, RunID: "1", Attempt: "1", Artifact: strings.Repeat("d", 64)},
	} {
		finalize, deploy := releaseAdmission(workflow, outcomes, invalid, checksum)
		if finalize || deploy {
			t.Errorf("untrusted docs subject %#v admitted release", invalid)
		}
	}
	for _, invalid := range []string{"", "not-a-digest", strings.Repeat("d", 64)} {
		_, deploy := releaseAdmission(workflow, outcomes, subject, invalid)
		if deploy {
			t.Errorf("untrusted cloud checksum %q admitted deployment", invalid)
		}
	}
	if !strings.Contains(raw, "release-summary:") || !strings.Contains(raw, "npm_status") || !strings.Contains(raw, "docs_status") || !strings.Contains(raw, "aws_status") {
		t.Fatal("release workflow must expose machine-readable partial-publication outcomes")
	}
}

func TestReleaseCallerMatchesCurrentDocsV2Receiver(t *testing.T) {
	_, raw := parseReleaseGraph(t)
	docsWorkflow, err := os.ReadFile(filepath.Join("..", "..", "..", "hitkeep-docs", ".github", "workflows", "sync-hitkeep-release.yml"))
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("optional cross-repository check requires the private docs checkout; producer contracts are checked independently")
	}
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(docsWorkflow)
	want := hex.EncodeToString(digest[:])
	if reviewedDocsV2Pin != want {
		t.Fatalf("reviewed docs v2 pin = %s, want %s", reviewedDocsV2Pin, want)
	}
	if !strings.Contains(raw, "DOCS_WORKFLOW_SHA256: "+want) || strings.Count(raw, "attestation_schema=hitkeep.docs-attestation/v2") != 2 {
		t.Fatal("release caller is not pinned to the v2 docs receiver")
	}
}

func TestPostpublicationCorrelationRejectsCollisions(t *testing.T) {
	_, raw := parseReleaseGraph(t)
	receiptVerifier := []string{
		`receipt_name="hitkeep-docs-postpublication-receipt-${downstream_run_id}-${downstream_run_attempt}"`,
		`receipt_id="$(jq -r --arg name "$receipt_name" '[.artifacts[] | select(.name == $name and .expired == false and (.digest | test("^sha256:[a-f0-9]{64}$")))] | if length == 1 then .[0].id else empty end' downstream-artifacts.json)"`,
		`receipt_digest="$(jq -r --argjson receipt_id "$receipt_id" '[.artifacts[] | select(.id == $receipt_id)] | if length == 1 then .[0].digest | ltrimstr("sha256:") else empty end' downstream-artifacts.json)"`,
		`"$(sha256sum downstream-receipt.zip | awk '{print $1}')" != "$receipt_digest"`,
		"actions/runs/$downstream_run_id/artifacts",
		"actions/artifacts/$receipt_id/zip",
		"postpublication receipt digest mismatch",
		`.schema_version == "hitkeep.docs-postpublication-receipt/v1"`,
		`.source.repository == "PascaleBeier/hitkeep"`,
		".source.run_id == $source_run_id",
		".source.commit == $source_commit",
		".source.tag == $tag",
		".source.workflow_sha256 == $source_workflow_sha256",
		".source.catalog_sha256 == $catalog_sha256",
		".source.example_sha256 == $example_sha256",
		".source.manifest_sha256 == $manifest_sha256",
		`.docs.repository == "PascaleBeier/hitkeep-docs"`,
		".docs.base_sha == $docs_base_sha",
		".docs.workflow_sha256 == $docs_workflow_sha256",
		".docs.prepublication_run_id == $docs_run_id",
		".docs.prepublication_run_attempt == $docs_run_attempt",
		".docs.prepublication_attestation_artifact_sha256 == $docs_attestation_artifact_sha256",
		".receipt.run_id == $receipt_run_id",
		".receipt.run_attempt == $receipt_run_attempt",
	}
	if err := validateReleaseWorkflowGraph([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	for _, want := range receiptVerifier {
		if !strings.Contains(raw, want) {
			t.Errorf("postpublication receipt verifier is missing %q", want)
			continue
		}
		if err := validateReleaseWorkflowGraph([]byte(replacePostpublicationReceipt(raw, want))); err == nil {
			t.Errorf("release validator accepted missing receipt verification %q", want)
		}
	}
	expected := postpublicationRun{Repository: "PascaleBeier/hitkeep-docs", Workflow: ".github/workflows/sync-hitkeep-release.yml", Event: "workflow_dispatch", Base: strings.Repeat("a", 40), RunID: "42", Attempt: "3", Subject: "source-and-prepublication-subject"}
	for _, candidate := range []postpublicationRun{
		{Repository: expected.Repository, Workflow: expected.Workflow, Event: expected.Event, Base: expected.Base, RunID: expected.RunID, Attempt: "4", Subject: expected.Subject},
		{Repository: expected.Repository, Workflow: expected.Workflow, Event: expected.Event, Base: expected.Base, RunID: "43", Attempt: expected.Attempt, Subject: expected.Subject},
		{Repository: expected.Repository, Workflow: expected.Workflow, Event: expected.Event, Base: expected.Base, RunID: expected.RunID, Attempt: expected.Attempt, Subject: "wrong-subject"},
	} {
		if matchesPostpublicationRun(expected, candidate) {
			t.Errorf("collision %#v matched", candidate)
		}
	}
	if !matchesPostpublicationRun(expected, expected) {
		t.Fatal("exact postpublication receipt did not match")
	}
}

type postpublicationRun struct {
	Repository, Workflow, Event, Base, RunID, Attempt, Subject string
}

func matchesPostpublicationRun(expected, candidate postpublicationRun) bool {
	return expected == candidate
}

func replacePostpublicationReceipt(workflow, target string) string {
	start := strings.Index(workflow, "          receipt_name=")
	if start < 0 {
		return workflow
	}
	prefix, receipt := workflow[:start], workflow[start:]
	return prefix + strings.Replace(receipt, target, "removed", 1)
}

func parseReleaseGraph(t *testing.T) (releaseGraph, string) {
	t.Helper()
	raw, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	workflow := releaseGraph{Jobs: map[string]releaseGraphJob{}}
	for _, name := range []string{"docs-attestation", "finalize-release", "deploy-cloud"} {
		workflow.Jobs[name] = parseReleaseJob(t, text, name)
	}
	return workflow, text
}

func parseReleaseJob(t *testing.T, workflow, name string) releaseGraphJob {
	t.Helper()
	start := strings.Index(workflow, "  "+name+":\n")
	if start < 0 {
		t.Fatalf("missing %s", name)
	}
	block := workflow[start:]
	for i := 1; i+3 < len(block); i++ {
		if block[i] == '\n' && block[i+1] == ' ' && block[i+2] == ' ' && block[i+3] != ' ' {
			block = block[:i]
			break
		}
	}
	job := releaseGraphJob{}
	inNeeds := false
	for line := range strings.SplitSeq(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "needs:" {
			inNeeds = true
			continue
		}
		if strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") {
			inNeeds = false
		}
		if inNeeds && strings.HasPrefix(trimmed, "- ") {
			job.Needs = append(job.Needs, strings.TrimPrefix(trimmed, "- "))
		}
		if after, ok := strings.CutPrefix(trimmed, "if: "); ok {
			job.If = after
		}
	}
	return job
}

func releaseAdmission(workflow releaseGraph, outcomes map[string]string, subject docsSubject, checksum string) (bool, bool) {
	docs := outcomes["docs-attestation"] == "success" && admits(workflow.Jobs["docs-attestation"], outcomes) && validDocsSubject(subject)
	outcomes = maps.Clone(outcomes)
	if docs {
		outcomes["docs-attestation"] = "success"
	} else {
		outcomes["docs-attestation"] = "failure"
	}
	finalize := outcomes["finalize-release"] == "success" && admits(workflow.Jobs["finalize-release"], outcomes)
	if finalize {
		outcomes["finalize-release"] = "success"
	} else {
		outcomes["finalize-release"] = "failure"
	}
	return finalize, admits(workflow.Jobs["deploy-cloud"], outcomes) && validCloudChecksum(checksum)
}

func admits(job releaseGraphJob, outcomes map[string]string) bool {
	if len(job.Needs) == 0 {
		return false
	}
	for _, need := range job.Needs {
		if outcomes[need] != "success" {
			return false
		}
		if strings.Contains(job.If, "always()") && !strings.Contains(job.If, "needs."+need+".result == 'success'") {
			return false
		}
	}
	return true
}

func validDocsSubject(subject docsSubject) bool {
	return subject.Schema == "hitkeep.docs-attestation/v2" &&
		regexp.MustCompile("^[a-f0-9]{40}$").MatchString(subject.Base) &&
		subject.Workflow == reviewedDocsV2Pin &&
		regexp.MustCompile("^[1-9][0-9]*$").MatchString(subject.RunID) &&
		regexp.MustCompile("^[1-9][0-9]*$").MatchString(subject.Attempt) &&
		regexp.MustCompile("^[a-f0-9]{64}$").MatchString(subject.Artifact) &&
		subject.Artifact == strings.Repeat("b", 64)
}

func validCloudChecksum(checksum string) bool {
	return checksum == strings.Repeat("c", 64)
}

func TestPostpublicationFailureLeavesValidEarlyReceiptPending(t *testing.T) {
	_, raw := parseReleaseGraph(t)
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
	workflow, raw := parseReleaseGraph(t)
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
	outcomes := map[string]string{
		"release-please": "success", "build-release": "success", "upgrade-from-supported-floor": "success",
		"publish-helm": "success", "verify-tracker-package": "success", "docs-attestation": "failure", "finalize-release": "success",
	}
	subject := docsSubject{Schema: "hitkeep.docs-attestation/v2", Base: docsBase, Workflow: reviewedDocsV2Pin, RunID: "1", Attempt: "1", Artifact: strings.Repeat("b", 64)}
	if finalize, _ := releaseAdmission(workflow, outcomes, subject, strings.Repeat("c", 64)); finalize {
		t.Fatal("failed docs attestation admitted finalization")
	}
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
