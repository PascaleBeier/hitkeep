package devtool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoReleaserTaggedSelfHostedArchiveManifest(t *testing.T) {
	manifest := readGoReleaserManifest(t)
	archive := manifest[strings.Index(manifest, "archives:\n"):strings.Index(manifest, "\nchecksum:")]

	for _, want := range []string{
		"id: self-hosted",
		"ids:\n      - self-hosted",
		"formats:\n      - tar.gz",
		`name_template: "hitkeep_{{ .Version }}_Linux_{{ .Arch }}"`,
		"- LICENSE",
		"- README.md",
		"- hitkeep-configuration.json",
		"- hitkeep.example.yaml",
		"- hitkeep-configuration-manifest.json",
	} {
		if !strings.Contains(archive, want) {
			t.Errorf("self-hosted archive manifest missing %q", want)
		}
	}
	if strings.Contains(archive, "cloud") {
		t.Error("self-hosted archive must not include cloud artifacts")
	}
}

func TestGoReleaserCrossCompilerTemplates(t *testing.T) {
	manifest := readGoReleaserManifest(t)
	for _, want := range []string{
		`CC={{ if eq .Arch "arm64" }}aarch64-linux-gnu-gcc{{ else }}gcc{{ end }}`,
		`CXX={{ if eq .Arch "arm64" }}aarch64-linux-gnu-g++{{ else }}g++{{ end }}`,
	} {
		if strings.Count(manifest, want) != 2 {
			t.Errorf("expected both Linux builds to define %q", want)
		}
	}
}

func TestGoReleaserSnapshotVersionTemplate(t *testing.T) {
	manifest := readGoReleaserManifest(t)
	if !strings.Contains(manifest, "snapshot:\n  version_template: \"{{ .Env.HITKEEP_ARCHIVE_VERSION }}\"") {
		t.Fatal("GoReleaser snapshot version is not mapped from HITKEEP_ARCHIVE_VERSION")
	}
}

func TestGoReleaserTaggedArchiveChecksums(t *testing.T) {
	manifest := readGoReleaserManifest(t)
	checksum := manifest[strings.Index(manifest, "checksum:\n"):]
	for _, want := range []string{
		"name_template: SHA256SUMS",
		"algorithm: sha256",
		"ids:\n    - self-hosted",
	} {
		if !strings.Contains(checksum, want) {
			t.Errorf("checksum manifest missing %q", want)
		}
	}
}

func TestGoReleaserReleaseWorkflowContract(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "pipeline.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	for _, want := range []string{
		"build-release-archives:",
		"runs-on: ubuntu-22.04",
		"gcc-aarch64-linux-gnu g++-aarch64-linux-gnu",
		"goreleaser/v2@v2.18.0 release --clean --skip=publish --config .goreleaser.yaml",
		"--clean",
		"--skip=publish",
		"sha256sum --check goreleaser-SHA256SUMS",
		"./hk ci release-checksums",
		"goreleaser-SHA256SUMS",
		"hitkeep-cloud-linux-amd64",
		"hitkeep-linux-amd64",
		"./hk catalog configuration-manifest",
		"hitkeep-configuration-manifest.json",
		"release-archives-${{ inputs.version }}",
		"hitkeep_${release_version}_Linux_amd64.tar.gz",
		"hitkeep_${release_version}_Linux_arm64.tar.gz",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release workflow missing %q", want)
		}
	}
}

func TestGoReleaserBranchArchiveWorkflowContract(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "pipeline.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	for _, want := range []string{
		"if: ${{ !cancelled() }}",
		"fetch-depth: 0",
		"release_source_tag",
		"release_source_sha",
		"ref: ${{ inputs.release_source_tag || inputs.checkout_ref || github.sha }}",
		"git rev-parse --verify \"refs/tags/${RELEASE_SOURCE_TAG}^{commit}\"",
		"test \"$tag_commit\" = \"$RELEASE_SOURCE_SHA\"",
		"HITKEEP_ARCHIVE_VERSION=\"${RELEASE_TAG_NAME#v}\" env -u GOROOT go run github.com/goreleaser/goreleaser/v2@v2.18.0 release --snapshot --clean --skip=publish --config .goreleaser.yaml",
		"HITKEEP_ARCHIVE_VERSION=\"${RELEASE_TAG_NAME#v}\"",
		"runner.temp",
		"runner.temp }}/public-assets",
		"build_metadata=\"$(go version -m \"$binary\")\"",
		"GOOS=linux",
		"GOARCH=${arch}",
		"amd64)\n                version_output=\"$(\"$binary\" --version)\"",
		"arm64)\n                if ! strings \"$binary\" | grep -Fxq \"$HITKEEP_VERSION\"; then",
		"binary version mismatch for %s: got %q, want %q",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("branch archive workflow missing %q", want)
		}
	}
	if strings.Contains(workflow, "qemu-aarch64") {
		t.Error("release archive verification must not rely on QEMU runtime execution")
	}
}

func TestFilesystemLayoutManifestPinsBuildOwnershipSurfaces(t *testing.T) {
	root := filepath.Join("..", "..")
	contents, err := os.ReadFile(filepath.Join(root, "docs", "config-refactor", "filesystem-layout-manifest.md"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(contents)
	for _, row := range []string{
		"| `internal/devtool/catalog.go::variants` | Canonical developer build owner | Defines variant IDs, build tags, developer-container environment, local image names, and publishability metadata. Its cloud values are developer/build defaults, not runtime settings. |",
		"| `internal/devtool/app.go::App.ComposeEnvironment` | Workspace projection | Projects the selected variant plus workspace-scoped paths and ports into Compose variables; it does not define supported variants or production configuration precedence. |",
		"| `internal/devtool/runs.go::App.executeBuild` | Build orchestrator | Resolves a catalog variant, enforces the production/developer dependency boundary, and invokes the selected binary or image build without redefining tags or defaults. |",
		"| `Dockerfile` | Image-build consumer | Consumes explicit build arguments and produces the selected application image; it is not a configuration catalog or runtime parser. |",
		"| `.goreleaser.yaml` | Release-build projection | Maps the canonical self-hosted and cloud build identities to release tags, CGO targets, archive contents, and names; it must remain aligned with the developer catalog. |",
		"| `.github/workflows/pipeline.yml` | Delivery consumer | Supplies explicit version/ref inputs, restores verified assets, and invokes the canonical build projections. Publication and attestation policy lives here, but variant semantics do not. |",
	} {
		if count := strings.Count(manifest, row); count != 1 {
			t.Errorf("build ownership row appears %d times, want exactly once: %s", count, row)
		}
	}

	for path, anchors := range map[string][]string{
		"internal/devtool/catalog.go":    {"var variants = []Variant{"},
		"internal/devtool/app.go":        {"func (a *App) ComposeEnvironment(variant Variant) []string"},
		"internal/devtool/runs.go":       {"func (a *App) executeBuild(ctx context.Context, request RunRequest, writer io.Writer) error"},
		"Dockerfile":                     nil,
		".goreleaser.yaml":               {"id: self-hosted", "id: cloud", "CGO_ENABLED=1"},
		".github/workflows/pipeline.yml": {"build-release-archives:", "build-and-push-image:"},
	} {
		source, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Errorf("read build ownership surface %s: %v", path, err)
			continue
		}
		for _, anchor := range anchors {
			if !strings.Contains(string(source), anchor) {
				t.Errorf("build ownership surface %s is missing %q", path, anchor)
			}
		}
	}
}

// TestFilesystemLayoutManifestPinsReviewedFamilyRecords is a prose-presence sentinel, not semantic evidence validation.
func TestFilesystemLayoutManifestPinsReviewedFamilyRecords(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "..", "docs", "config-refactor", "filesystem-layout-manifest.md"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(contents)
	records := map[string][]string{
		"### `internal/entitlements`": {
			"11 files: eight production files",
			"`provider_billing.go`, `service_billing.go`, and `service_billing_test.go` require `billing`",
			"`provider_oss.go` and `service_oss.go` require `!billing`",
			"the 27 currently identified direct external importers are `cmd/hitkeep.go`",
			"`internal/server/access/context.go`",
			"`internal/server/access/context_test.go`",
			"`internal/server/admin/activation_plan_handlers_billing.go`",
			"`internal/server/admin/team_site_handlers.go`",
			"`internal/server/auth/account_handlers.go`",
			"`internal/server/auth/handlers_cloud_billing_test.go`",
			"`internal/server/auth/handlers_test.go`",
			"`internal/server/auth/sso_handlers_test.go`",
			"`internal/server/cloud/handlers_billing.go`",
			"`internal/server/cloud/handlers_billing_events.go`",
			"`internal/server/server.go`",
			"`internal/server/server_test.go`",
			"`internal/server/shared/context.go`",
			"`internal/server/sites/handlers_test.go`",
			"`internal/server/user/report_definitions_handlers_test.go`",
			"`internal/server/user/security_handlers_test.go`",
			"`internal/server/user/team_handlers.go`",
			"`internal/server/user/team_handlers_billing_test.go`",
			"`internal/server/user/team_handlers_cloud_billing_test.go`",
			"`internal/server/user/team_membership_handlers.go`",
			"`internal/server/user/tracking_domain_handlers_test.go`",
			"`internal/worker/cloud_retention_sync_billing.go`",
			"`internal/worker/cloud_retention_sync_billing_test.go`",
			"`internal/worker/cloud_retention_sync_oss.go`",
			"`internal/worker/reports.go`",
			"`internal/worker/reports_test.go`",
			"`internal/entitlements/service_test.go` is the package-local external-test importer",
			"The full production import union is stdlib `context`, `errors`, `fmt`, and `strings`; `github.com/google/uuid`; and `hitkeep/config` and `hitkeep/internal/database`",
			"no direct filesystem, network, process, environment, `go:embed`, generator, OS, or CGO ownership",
			"OSS defaults are unlimited",
			"`TeamBypassesCloudLimits` is actor-independent for operator-owned teams",
			"`ErrActiveTenantChanged`",
			"`ErrSiteLimitReached`",
			"`403 Site limit reached` and `409 Active team changed; retry`",
			"`MaxTeams`, `MaxTeamMembers`, and membership check-then-act races remain unresolved",
			"coordinated source revert of this record/sentinel and the site-quota runtime changes in `internal/database/store.go`, `internal/database/site_quota_test.go`, `internal/database/store_site.go`, `internal/database/tenant_store_manager.go`, `internal/entitlements/service.go`, `internal/entitlements/service_test.go`, `internal/server/sites/handlers.go`, and `internal/server/sites/handlers_test.go`",
			"quota-aware `CreateSiteWithQuota`/`TransferSiteWithQuota`, the shared `siteQuotaMu` and stable sentinels, `TeamSiteLimit`, handler wiring and `403`/`409` statuses, and focused quota tests",
			"`internal/database/site_quota_test.go`",
			"reintroduces unenforced/unserialized over-cap site creation and transfer",
			"Disposition: decomposition required / stay internal / blocked",
		},
		"### `internal/devtool`":     {"decomposition required / stay internal", "native PID/lock/process/toolchain/cancellation state"},
		"### `internal/importables`": {"`os.Open(source.Path)`", "`zip.OpenReader(source.Path)`", "untrusted staging/source-path containment boundary", "stay internal / blocked"},
		"### `internal/assetstore`":  {"`internal/assetstore` → `internal/assetstore`; no move is approved", "direct imports are stdlib `errors`, `fmt`, `mime`, `os`, `path/filepath`, `strings`, and `syscall`, plus `github.com/google/uuid`", "seven direct dependent files", "no import cycle; reconfirm that result before any move", "user/site-derived QR paths", "only `PutQRCodeAsset` creates the configured root", "os.OpenRoot", "os.OpenInRoot", "Rooted traversal and ancestor-symlink escapes are rejected", "`Open` rejects a final-file escape symlink", "`Delete` unlinks it and rooted `Rename` replaces it without touching its target", "bind-mount, device-file, fsync/durability, or cross-platform atomic-replacement isolation", "TestStoreMissingRootOperationsHaveNoSideEffects", "TestStorePutOpenDeleteRoundTrip", "TestPutQRCodeAssetCleansTemporaryFileAfterRenameFailure", "TestStoreRejectsSymlinkEscapes", "outside sentinels remain unchanged", "No compatibility shim or forwarding package is justified", "stay internal / blocked"},
		"### `internal/realtime`":    {"exact three-file package inventory is `broker.go`, `broker_test.go`, and `broker_lifecycle_test.go`", "`internal/server/shared/realtime_stream_lifecycle_test.go` and `internal/server/server_realtime_shutdown_test.go`", "exact 14 external importer files", "`cmd/hitkeep.go`, `internal/ingest/consumer.go`, `internal/ingest/consumer_test.go`", "`internal/server/aifetch/handlers.go`, `internal/server/goals/handlers.go`, `internal/server/imports/handlers.go`, `internal/server/opportunities/handlers.go`, `internal/server/server.go`, `internal/server/server_realtime_shutdown_test.go`", "`internal/server/share/realtime_handlers_test.go`, `internal/server/shared/context.go`, `internal/server/shared/realtime_stream.go`, `internal/server/shared/realtime_stream_lifecycle_test.go`, and `internal/server/sites/realtime_handlers_test.go`", "full direct import union, including package tests, is stdlib `strconv`, `sync`, `testing`, `testing/synctest`, and `time`, plus `github.com/google/uuid`", "no build tags, generated inputs, filesystem, network, process, environment, `go:embed`, OS, or CGO ownership", "bounded graph reports no Go import cycle", "wider consumer closure remains a lower bound", "mutex serializes lifecycle transitions and global event IDs", "retained only while that site has an active subscriber", "invalid nonempty and newer-than-retained `Last-Event-ID` values resync", "slow subscribers also receive resync", "`Broker.Close` is idempotent", "`Server.Shutdown` closes the broker before import/filter/limiter shutdown", "fixed one-minute cutoff and response write deadline before subscription, prelude, resync, or replay output", "native `EventSource` reconnects and re-authorizes", "Revocation exposure is bounded by reconnect, not immediate", "mock has no `Last-Event-ID` simulation", "no persistence/cross-leader replay or exact broader-consumer closure is proven", "TestBrokerCloseClosesAllSubscriptionsAndRejectsNewWork", "TestServeRealtimeStreamSetsDeadlineBeforePrelude", "TestShutdownClosesRealtimeBeforeBlockingImportStop", "go test -race ./internal/realtime ./internal/ingest ./internal/server ./internal/server/share ./internal/server/shared ./internal/server/sites -count=1", "coordinated rollback reverts this record, README/sentinel wiring, `internal/realtime/broker.go`", "reintroduces shutdown, replay, privacy/revocation, and memory-retention defects", "future move carries server, ingest, and realtime handlers together", "no compatibility shim is justified", "Disposition: decomposition required / stay internal / blocked"},
		"### `internal/reporting`": {
			"exact three-file package inventory is `schedule.go`, `schedule_test.go`, and `tokens.go`",
			"exactly eight direct external importer files",
			"`internal/database/store_report_confirmations.go`, `internal/database/store_report_definitions.go`, `internal/database/store_report_definitions_test.go`, `internal/database/store_report_delivery.go`",
			"`internal/server/user/report_definitions_handlers.go`, `internal/server/user/report_definitions_handlers_test.go`, `internal/worker/reports_scheduler.go`, and `internal/worker/reports_test.go`",
			"The full direct import union is stdlib `errors`, `fmt`, `strconv`, `strings`, `time`, `crypto/hmac`, `crypto/rand`, `crypto/sha256`, `encoding/base64`, and `encoding/hex`; `hitkeep/internal/api`; and `github.com/google/uuid`",
			"bounded direct graph reports no import cycle; exhaustive transitive closure remains unproven",
			"no package-local filesystem, network, process, environment, persistence, `go:embed`, or build-tag ownership is evidenced",
			"`ValidateSchedule`, `NextOccurrence`, `PeriodBounds`, and `CatchUpWindow`",
			"confirmation-token generation reads exactly 32 random bytes",
			"HMAC-SHA256 values bound to the report and recipient UUIDs",
			"tamper coverage rejects altered tokens",
			"No concrete runtime defect was found in this bounded evidence",
			"go test -race ./internal/reporting ./internal/database ./internal/server/user ./internal/worker -count=1",
			"rollback of this documentation/sentinel decision is removal of this record and its README/sentinel anchors",
			"no compatibility shim or forwarding package is justified",
			"Disposition: decomposition required / stay internal / blocked",
		},
		"### `internal/searchconsole`":                          {"exact three-file package inventory is `client.go`, `client_test.go`, and `client_lifecycle_test.go`", "exact nine direct external importer files", "`cmd/hitkeep.go`, `cmd/seed/google_search_console.go`, `internal/server/server.go`, `internal/server/shared/context.go`, and `internal/server/system/api_docs_schemas.go`", "`internal/server/user/google_search_console_handlers.go`, `internal/server/user/google_search_console_handlers_test.go`, `internal/worker/search_console.go`, and `internal/worker/search_console_test.go`", "No build tags, generated inputs, filesystem, process, environment, `go:embed`, OS-specific, or CGO ownership was observed", "OAuth and Google API network calls remain package-owned", "bounded graph reports no import cycle", "wider consumer closure is a lower bound", "one-minute operation deadline", "one-minute due-run deadline", "preserving earlier caller cancellation", "25,000 rows", "250,000 rows", "100 pages", "rather than silently truncating", "TestGoogleOperationContextPreservesParentCancellation", "TestGoogleClientOperationsHaveDeadline", "TestGoogleClientQuerySearchAnalyticsCeilingsReturnErrors", "TestSearchConsoleSyncWorkerStartStopsBlockedSyncWhenContextExpires", "Live Google OAuth/API behavior, rate-limit/backoff behavior, exact broader-consumer closure, and per-request pagination fixtures remain unproven", "go test -race ./internal/searchconsole ./internal/worker ./internal/server/user ./internal/server/shared ./internal/server ./cmd ./cmd/hitkeep -count=1", "Coordinated rollback reverts this record, README/sentinel wiring, `internal/searchconsole/client.go`, `internal/searchconsole/client_lifecycle_test.go`, `internal/worker/search_console.go`, and `internal/worker/search_console_test.go`", "reintroduces unbounded provider and worker calls plus unbounded pagination", "future move must carry server wiring, worker, and API handlers atomically", "no compatibility shim is justified", "Disposition: decomposition required / stay internal / blocked"},
		"### `internal/analyticstools`":                         {"exactly `tools.go`", "zero local test files", "`internal/mcpserver/tools.go`, `internal/opportunities/tool_bridge.go`, and `internal/server/askai/handlers.go`", "The full direct import union is stdlib `context`, `fmt`, `strings`, and `time`; `github.com/google/uuid`; `github.com/zendev-sh/goai`; and `hitkeep/analyticscatalog`, `hitkeep/internal/api`, `hitkeep/internal/database`, and `hitkeep/jsonapi`", "no direct filesystem, network, subprocess, environment, `go:embed`, generator, or build-tag ownership", "bounded direct graph returns no import cycle", "`Config` carries caller-provided site/user scope, time range, filters, and `BeforeExecute`", "event-breakdown tool maximum is 25, built-in ecommerce and web-vitals limits are 10, and the correlation window maximum is 90 days", "`EventNamesData`/`toolJSON` name-set bounds, exported-helper output bounds", "go test -race ./internal/mcpserver ./internal/opportunities ./internal/server/askai", "direct `internal/analyticstools` package proof", "Rollback for this documentation/sentinel decision is removal of this record", "A future move must restore the old package and imports atomically", "no compatibility shim or forwarding package is justified", "decomposition required / stay internal / blocked"},
		"### `internal/ai`":                                     {"bounded direct importers", "exhaustive transitive closure is not claimed", "Config.Timeout", "RunRecorder", "AppendAIRun", "ReserveAIRun", "validated", "no raw provider", "TestLiveOpenAIOpportunityCandidateProposalSmoke", "no move is approved", "No `internal/ai` import cycle is evidenced", "no compatibility shim", "decomposition required / stay internal / blocked"},
		"### `internal/api`":                                    {"`internal/api` → `internal/api`; no move is approved", "exactly three indexed Go files", "210 importing files", "bounded graph", "lower bound, not a complete closure", "No cycle is returned, but the graph is incomplete", "pure API/DTO contract ownership", "no filesystem, process, network, database, persistence", "no build tags", "google_search_console_test.go", "No compatibility shim or forwarding package is justified", "stay internal / blocked"},
		"### `internal/database`":                               {"`internal/database` → `internal/database`; no move is approved", "`TenantStoreManager`", "Exact direct-importer count and exhaustive transitive closure are not claimed", "exact combined import list and complete cycle result are not established", "direct CGO status", "`recoverCompactionSwap`", "`os.Rename`", "`syncDirectory`", "`.wal` artifact cleanup", "do not prove package-wide fsync, atomic-replacement, no-clobber, or WAL guarantees", "No subprocess ownership is evidenced", "`ReplaceSearchConsoleFacts`", "Exhaustive test closure is not claimed", "No compatibility shim or forwarding package is justified", "decomposition required / stay internal / blocked"},
		"### `internal/server`":                                 {"`internal/server` → `internal/server`; no move is approved", "Required decomposition", "Gortex returned 50 files", "bounded result", "confirmed bounded outward impact reaches `cmd/hitkeep`", "inward dependencies or integration surfaces", "bounded direct-importer inventory is unavailable", "heterogeneous imports", "complete cycle proof is unavailable", "concrete focused subpackage targets", "affected-consumer closure", "direct rollback", "no compatibility shim", "inbound HTTP", "database/store", "network-facing integrations", "billing/OSS variants", "Unix-specific disk-usage", "Complete build-tag, OS, CGO, generated, and embed closure is not established", "exact test closure is incomplete", "No compatibility shim or forwarding package is justified", "decomposition required / stay internal / blocked"},
		"### `internal/worker`":                                 {"exactly 21 indexed files", "no move is approved", "backup", "retention/archive", "rollup/backfill", "reports/Search Console", "import-stage cleanup", "cloud billing/OSS variants", "lifecycle.go::waitForDelay", "bounded direct importer list", "exhaustive transitive closure is not claimed", "native DuckDB export", "local/S3 archive", "DuckDB retention queries/deletes", "cancellation-aware timer", "8 indexed test files", "affected-package/test closure", "billing/OSS build matrix", "no import cycle is proven", "embedded assets", "generated inputs", "build-tag/CGO matrix", "OS-specific", "no compatibility shim", "decomposition required / stay internal / blocked"},
		"### `internal/cluster`":                                {"the two direct importers", "no move is approved", "memberlist", "in-memory", "bounded transitive impact", "No import cycle is evidenced", "no build tags", "OS/CGO split", "generated source", "embedded assets", "test closure", "no compatibility shim", "stay internal / blocked"},
		"### `internal/mcpserver`":                              {"`internal/mcpserver` → `internal/mcpserver`; no move is approved", "the exact direct importer is `internal/server/server.go`", "91 affected symbols lower bound", "No Go import cycle is evidenced", "six production Go files and two test files", "owns no filesystem, subprocess, or package-local persistence boundary", "uses a 10-second timeout", "caps responses at 2 MiB", "46 `Test*` functions", "`user_id` and `owner_email`", "general non-default-tenant routing", "No compatibility shim or forwarding package is justified", "decomposition required / stay internal / blocked"},
		"### `internal/opportunities`":                          {"`internal/opportunities` → `internal/opportunities`; no move is approved", "deterministic detector/evidence core", "`internal/opportunities/smokegate`", "`cmd/opportunities-smoke` is an external", "Exact direct-importer count and exhaustive transitive closure are not claimed", "exact combined import list and complete cycle result are not established", "Exact build tags, OS/CGO files, generated inputs, embedded assets", "Direct filesystem, subprocess, and network ownership are not established", "belong to external `cmd/opportunities-smoke`", "opportunity persistence/schema remains `internal/database`-owned", "`loadOpportunitySignals`", "does not prove caller authorization", "`TestLoadOpportunitySignalsCanProvideSetupEvidenceSnapshot`", "Exhaustive test closure is not claimed", "No compatibility shim or forwarding package is justified", "decomposition required / stay internal / blocked"},
		"### `internal/blocking`":                               {"`internal/blocking` → `internal/blocking`; no move is approved", "the exact nine-file inventory", "`cidr.go`, `cidr_test.go`, `default_spam_filter.json`, `ip_filter.go`, `ip_filter_test.go`, `spam_data.go`, `spam_filter.go`, `spam_filter_test.go`, and `spam_updater.go`", "exactly 11 direct importer files", "cmd/update_spam_lists.go", "internal/server/shared/exclusion_rule.go", "bounded graph establishes no import cycle", "full direct import union", "`bytes`", "`github.com/google/uuid`", "`hitkeep/internal/database`", "go:embed", "scripts/update-default-spam-filter.sh", ".github/workflows/spam-list-refresh.yml", "same-directory temporary file created 0600", "No fsync/durability or cross-platform atomic-replacement claim is made", "final symlink is replaced rather than followed", "exactly 10 MiB is accepted", "larger body is rejected as an invalid response", "original response body closes on status, read, parser, success, and oversize paths", "No speculative SSRF layer is justified", "holds a channel-based context-aware transition gate across fetch, cancellation recheck, cache save, and in-memory apply", "public `RefreshFromDisk()` uses a background context and takes the same transition gate", "TestSpamFilterQueuedTransitionCancellationReleasesGate", "TestSaveSpamFeedDataWritesNewFile0600AndReplacesFinalSymlink", "TestSpamFilterSerializesWholeUpdateGeneration", "leader remote refresh starts asynchronously", "decomposition required / stay internal / blocked"},
		"### `internal/ingest`":                                 {"no direct filesystem operations found", "stay internal / blocked"},
		"### `internal/ipmeta` and `internal/ipmeta/ipmetagen`": {"embedded `io/fs`", "ordinary generated-file candidates", "stay internal / blocked"},
		"### `internal/mailables`":                              {"`billing` build tag", "stay internal / blocked"},
		"### `internal/mailer` and `internal/mailer/drivers`": {
			"exact 31-file inventory",
			"`driver.go`, `errors.go`, `i18n.go`, `manager.go`, `types.go`",
			"`errors_test.go`, `manager_test.go`",
			"`drivers/smtp.go`, `drivers/smtp_test.go`",
			"`locales/de.json`, `en.json`, `es.json`, `fr.json`, `it.json`, `nl.json`, `pt.json`",
			"`templates/analytics_digest.txt`, `cloud_free_limit_reminder.txt`, `cloud_free_retention_pretrim.txt`, `cloud_free_retention_reminder.txt`, `cloud_welcome.txt`, `email_verification.txt`, `layout.txt`, `mfa_magic_link.txt`, `opportunity_digest.txt`, `password_reset.txt`, `report_confirmation.txt`, `site_report.txt`, `social_confirmation.txt`, `team_invite.txt`, `user_invite.txt`",
			"exactly 35 direct external importer files",
			"`cmd/hitkeep.go`, `cmd/preview-emails/main.go`",
			"`internal/mailables/auth.go`, `internal/mailables/cloud_lifecycle.go`, `internal/mailables/cloud_lifecycle_test.go`, `internal/mailables/cloud_signup.go`, `internal/mailables/invite.go`, `internal/mailables/report_confirmation.go`, `internal/mailables/reports.go`, `internal/mailables/reports_test.go`, `internal/mailables/team_invite.go`",
			"`internal/server/admin/handlers_test.go`, `internal/server/admin/system_action_handlers.go`, `internal/server/admin/system_handlers_test.go`, `internal/server/admin/team_site_handlers.go`",
			"`internal/server/auth/account_handlers.go`, `internal/server/auth/handlers.go`, `internal/server/auth/handlers_test.go`, `internal/server/auth/mfa_handlers.go`, `internal/server/auth/social_handlers.go`, `internal/server/auth/social_handlers_test.go`",
			"`internal/server/cloud/handlers_billing.go`, `internal/server/cloud/handlers_billing_test.go`, `internal/server/server.go`, `internal/server/shared/context.go`",
			"`internal/server/user/report_definitions_handlers.go`, `internal/server/user/report_definitions_handlers_test.go`, `internal/server/user/team_handlers.go`, `internal/server/user/team_handlers_test.go`",
			"`internal/worker/cloud_lifecycle_billing.go`, `internal/worker/cloud_lifecycle_billing_test.go`, `internal/worker/cloud_lifecycle_oss.go`, `internal/worker/reports.go`, `internal/worker/reports_scheduler.go`, and `internal/worker/reports_test.go`",
			"`internal/devtool/ci_test.go` is a race-inventory reference, not a package importer",
			"full direct import union is stdlib `bytes`, `context`, `crypto/tls`, `embed`, `errors`, `fmt`, `html/template`, `io/fs`, `math`, `net`, `net/url`, `path`, `regexp`, `slices`, `strings`, `testing`, `text/template`, and `time`",
			"`github.com/Boostport/mjml-go`, `github.com/wneessen/go-mail`, `golang.org/x/text/language`, and `golang.org/x/text/message`",
			"`hitkeep/config`, `hitkeep/internal/mailer/drivers`, and `hitkeep/jsonapi`",
			"`//go:embed locales/*.json`",
			"`//go:embed templates/*.mjml templates/*.txt`",
			"Audited production server handlers and workers use bounded, sanitized `DescribeError` details",
			"`cmd/preview-emails` logs raw `err` and `site.Domain`, an explicit developer-preview privacy gap",
			"Forgot-password unknown-email, send-failure, and nil-mailer cases return the identical canonical HTTP 200 generic body",
			"Admin mail status requires both a nonblank SMTP host and a runtime mailer, and it redacts credentials",
			"`Send` is contextless, so cancellation of in-flight delivery can lag",
			"There is no direct concurrency, live SMTP, TLS/auth, or timeout proof, and `SendWithHeaders` lacks direct boundary proof",
			"TestHandleForgotPasswordMailFailureMatchesUnknownEmail",
			"TestSystemMailRedaction",
			"coordinated revert of this record, the README/sentinel wiring, and `internal/server/auth/account_handlers.go`, `internal/server/auth/handlers_test.go`, `internal/server/admin/system_handlers.go`, and `internal/server/admin/system_handlers_test.go`",
			"it reintroduces password-reset enumeration/status defects",
			"A future move must atomically carry mailer, drivers, embeds, and all imports",
			"Disposition: decomposition required / stay internal / blocked",
		},
		"### `internal/testutil`":    {"Subrecord — `passkeys.go`", "no compatibility shim", "Subrecord — `testdb/testdb.go`", "Rollback: retain the existing native fixture path; no shim.", "stay internal / blocked"},
		"### `internal/socialauth`":  {"`crypto/subtle`", "`Client.Complete` gives provider completion a 10s timeout", "custom Microsoft JWKS and GitHub JSON reads", "not general OIDC discovery or OAuth token exchange", "no compatibility shim", "stay internal / blocked"},
		"### `internal/sso`":         {"the ten direct importers", "validates DNS and dial targets as public", "self-hosted mode permits private identity providers", "both discovery and `Complete` token exchange/verification are bounded by `providerTimeout`", "AES-GCM", "HMAC-SHA256 domain separation", "timeout fix unless a separately approved compatibility/security decision", "stay internal / blocked"},
		"### `internal/aianalytics`": {"Runtime subrecord", "Updater subrecord", "successful body `Close` delegates to the underlying `resp.Body`", "Rollback for this documentation decision is removal of the record", "decomposition required / stay internal / blocked"},
		"### `internal/webhookdispatcher` and `internal/webhooks`": {
			"exact coupled inventory is 15 files",
			"`dispatcher.go`, `dispatcher_test.go`, `emitter.go`, `emitter_test.go`, `sweeper.go`, `sweeper_test.go`, `worker.go`, and `worker_test.go`",
			"`catalog.go`, `catalog_test.go`, `event.go`, `url.go`, `url_test.go`, `version.go`, and `version_test.go`",
			"exactly 22 direct external importer files",
			"`internal/database/store_webhook_delivery.go`",
			"`internal/database/store_webhook_delivery_test.go`",
			"`internal/database/store_webhook_retention_test.go`",
			"`internal/database/store_webhooks.go`",
			"`internal/database/store_webhooks_test.go`",
			"`internal/ingest/consumer.go`",
			"`internal/ingest/consumer_test.go`",
			"`internal/server/admin/handlers.go`",
			"`internal/server/admin/handlers_test.go`",
			"`internal/server/auth/account_handlers.go`",
			"`internal/server/auth/sso_handlers.go`",
			"`internal/server/goals/handlers.go`",
			"`internal/server/goals/handlers_test.go`",
			"`internal/server/imports/handlers.go`",
			"`internal/server/imports/runner.go`",
			"`internal/server/shared/context.go`",
			"`internal/server/shared/webhooks.go`",
			"`internal/server/sites/handlers.go`",
			"`internal/server/user/team_handlers.go`",
			"`internal/server/user/team_handlers_test.go`",
			"`internal/server/webhooks/handlers.go`",
			"`internal/server/webhooks/handlers_test.go`",
			"exactly three direct external importers: `cmd/hitkeep.go`, `internal/server/server.go`, and `internal/server/webhooks/handlers_test.go`",
			"external union is exactly 24 unique paths",
			"webhooks production import union is stdlib `context`, `errors`, `fmt`, `net`, `net/netip`, `net/url`, `slices`, and `strings`",
			"dispatcher production import union is stdlib `context`, `crypto/hmac`, `crypto/sha256`",
			"No build tags, OS-specific files, CGO, generated inputs, `go:embed`, or package-local filesystem ownership were found",
			"bounded graph reports no import cycle; exhaustive consumer and transitive closure remain unproven",
			"lifecycle is explicitly context-cancelable through startup, worker, and sweeper shutdown",
			"Webhook deletion and delivery retention remain database-owned",
			"no concrete runtime defect was found in this bounded evidence",
			"End-to-end remote delivery, durable retry/retention behavior, raw payload/log privacy, and exhaustive test/consumer closure remain unproven",
			"Rollback is removal of this documentation/README/sentinel record; no runtime behavior changed",
			"future move must carry the dispatcher, webhook contracts, database ownership, ingest emission, and server handlers atomically",
			"No compatibility shim or forwarding package is justified",
			"Disposition: **decomposition required / stay internal / blocked**",
		},
		"### `internal/takeout`": {
			"exact three-file package inventory is `takeout.go`, `takeout_test.go`, and `takeout_cleanup_test.go`",
			"exact four external importer files are `internal/server/server.go`, `internal/server/shared/context.go`, `internal/server/takeout/handlers.go`, and `internal/server/takeout/handlers_test.go`",
			"`ExportUserData`, `ExportSiteData`, and `ExportQRCodeData` select their sources, then route through `exportTakeoutFromSources` and `exportTakeoutFromStore` to the DuckDB query/COPY boundary",
			"It produces XLSX, CSV, Parquet, JSON, and NDJSON exports",
			"`OpenExportFile` and `CleanupExportFile` retain the existing `cleanExportPath` containment boundary",
			"the package is unconditional; no package-local network, subprocess, build-tag, or CGO ownership is evidenced",
			"Database-store ownership is external, as are authorization/routing handlers; filesystem containment and cleanup stay here",
			"Bounded graph evidence does not prove a Go import cycle, but this is not a move-proof",
			"focused takeout and server/takeout tests cover the package boundary",
			"They do not prove transitive cycle closure, export size/time/concurrency/durability/platform behavior, caller-owned authorization, or complete privacy/table and retention lifecycle",
			"coordinated rollback reverts this record with its README and sentinel wiring; it adds no compatibility shim",
			"A future move must carry the database, authorization, export, and persistence boundary together",
			"Disposition: decomposition required / stay internal / blocked",
		},
		"### `internal/security`": {
			"`internal/security` → `internal/security` (no move)",
			"The exact indexed inventory is `passkeys.go`, `recovery_codes.go`, `recovery_codes_canonical_test.go`, `recovery_codes_test.go`, `totp.go`, and `totp_test.go`",
			"exactly 12 direct external importers",
			"`internal/database/store_security.go`, `internal/database/store_security_test.go`",
			"`internal/server/auth/handlers_test.go`, `internal/server/auth/login_flow.go`, `internal/server/auth/mfa_handlers.go`, `internal/server/auth/passkey_handlers.go`, `internal/server/auth/social_handlers.go`, `internal/server/auth/sso_handlers.go`",
			"`internal/server/user/security_handlers.go`, `internal/server/user/security_handlers_test.go`, `internal/socialauth/socialauth.go`, and `internal/sso/relying_party.go`",
			"The full direct import union is stdlib `crypto/rand`, `crypto/subtle`, `encoding/base64`, `errors`, `fmt`, `net`, `net/http`, `net/url`, `strconv`, `strings`, `testing`, and `time`",
			"`github.com/go-webauthn/webauthn/protocol`, `github.com/go-webauthn/webauthn/webauthn`, `github.com/google/uuid`, `github.com/pquerna/otp`, `github.com/pquerna/otp/totp`, and `golang.org/x/crypto/argon2`",
			"`hitkeep/internal/api`",
			"bounded direct graph proves no import cycle; wider closure remains unproven",
			"No package-local filesystem, network, process, environment, `go:embed`, generator, OS-specific, or CGO ownership was observed",
			"persisted hashes must use the package's canonical memory, time, and thread parameters before `argon2.IDKey` is reached",
			"Positive TOTP-window and random-challenge-size caller bounds remain unproven",
			"passkeys have no direct package test",
			"forwarded-proto trust depends on upstream trusted-proxy middleware proof",
			"go test -race ./internal/security ./internal/database ./internal/server/auth ./internal/server/user ./internal/socialauth ./internal/sso -count=1",
			"A future move must carry security primitives, credential storage, and handlers atomically",
			"No compatibility shim or forwarding package is justified",
			"Disposition: **decomposition required / stay internal / blocked**",
		},
		"### `internal/auth`": {"`internal/auth` → `internal/auth` (no move)", "Combined package imports are stdlib `fmt`, `net/http`, `slices`, `sort`, `strings`, and `time`", "It imports no HitKeep package, so it introduces no internal dependency cycle.", "jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()})", "RenderTypeScriptCapabilities", "TestGeneratedTypeScriptCapabilitiesAreCurrent", "Retain the HS256 restriction", "separate approved compatibility decision", "stay internal / blocked"},
	}
	for heading, fragments := range records {
		if count := strings.Count(manifest, heading); count != 1 {
			t.Errorf("reviewed family heading %s appears %d times, want exactly once", heading, count)
			continue
		}
		section := manifest[strings.Index(manifest, heading)+len(heading):]
		if next := strings.Index(section, "\n### "); next >= 0 {
			section = section[:next]
		}
		for _, fragment := range fragments {
			if !strings.Contains(section, fragment) {
				t.Errorf("reviewed family %s is missing %q", heading, fragment)
			}
		}
	}
}

func TestGoReleaserReleaseArchiveAssetsStayInWorkspace(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "pipeline.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	start := strings.Index(workflow, "  build-release-archives:\n")
	if start == -1 {
		t.Fatal("release archive job missing")
	}
	archiveJob := workflow[start:]
	if end := strings.Index(archiveJob, "\n  upload-release-binaries:"); end != -1 {
		archiveJob = archiveJob[:end]
	}
	for _, want := range []string{
		"path: ${{ runner.temp }}/public-assets",
		"PUBLIC_ASSETS_DIR: ${{ runner.temp }}/public-assets",
		"PUBLIC_ASSETS_ARCHIVE: public/.public-assets.tar.gz",
		"cp \"$archive\" \"$PUBLIC_ASSETS_ARCHIVE\"",
		"./hk ci restore-dashboard --archive \"$PUBLIC_ASSETS_ARCHIVE\"",
		"rm -f \"$PUBLIC_ASSETS_ARCHIVE\"",
	} {
		if !strings.Contains(archiveJob, want) {
			t.Errorf("release archive assets setup missing %q", want)
		}
	}
	if strings.Contains(archiveJob, "artifacts/public") {
		t.Error("release archive job must not stage public assets in artifacts/public")
	}
}

func TestGoReleaserReleaseConfigUsesProductionCommand(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "pipeline.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	for _, want := range []string{
		"./hk catalog configuration --output json",
		"env -u GOROOT go run ./cmd/hitkeep config init --output \"$release_inputs/hitkeep.example.yaml\"",
		"cmp \"$release_inputs/hitkeep.example.yaml\" hitkeep.example.yaml",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release configuration generation missing %q", want)
		}
	}
	if strings.Contains(workflow, "./hk config init") {
		t.Error("release configuration generation must use the production Cobra config init command")
	}
}

func TestGoReleaserSnapshotBuildCallsSetArchiveVersion(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "pipeline.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	const snapshotBuild = "goreleaser/v2@v2.18.0 build \\\n            --snapshot"
	const versionedSnapshotBuild = "HITKEEP_ARCHIVE_VERSION=\"${HITKEEP_VERSION#v}\" env -u GOROOT go run github.com/goreleaser/goreleaser/v2@v2.18.0 build"
	const cleanSnapshotBuild = "--snapshot \\\n            --clean \\\n            --single-target"
	if got := strings.Count(workflow, snapshotBuild); got != 2 {
		t.Errorf("snapshot GoReleaser build calls = %d, want 2", got)
	}
	if got := strings.Count(workflow, versionedSnapshotBuild); got != 2 {
		t.Errorf("snapshot GoReleaser build calls with an archive version = %d, want 2", got)
	}
	if got := strings.Count(workflow, cleanSnapshotBuild); got != 2 {
		t.Errorf("snapshot GoReleaser build calls that clean dist first = %d, want 2", got)
	}
}

func TestGoReleaserNativeBuildsVerifyRawBinaryVersions(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "pipeline.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	start := strings.Index(workflow, "  build-binaries:\n")
	if start == -1 {
		t.Fatal("native binary build job missing")
	}
	nativeJob := workflow[start:]
	if end := strings.Index(nativeJob, "\n  build-release-archives:"); end != -1 {
		nativeJob = nativeJob[:end]
	}
	verificationStart := strings.Index(nativeJob, "      - name: Verify native binary versions\n")
	if verificationStart == -1 {
		t.Fatal("native binary version verification step missing")
	}
	verificationStep := nativeJob[verificationStart:]
	if end := strings.Index(verificationStep, "\n      - uses: actions/upload-artifact"); end != -1 {
		verificationStep = verificationStep[:end]
	}
	for _, want := range []string{
		"HITKEEP_VERSION: ${{ inputs.version }}",
		"for binary in \"hitkeep-${{ matrix.artifact_suffix }}\" \"hitkeep-cloud-${{ matrix.artifact_suffix }}\"; do",
		"version_output=\"$(\"./$binary\" --version)\"",
		"test \"$version_output\" = \"$HITKEEP_VERSION\"",
	} {
		if !strings.Contains(verificationStep, want) {
			t.Errorf("native binary version verification missing %q", want)
		}
	}
	if strings.Contains(verificationStep, "qemu-") || strings.Contains(verificationStep, "strings \"$binary\"") {
		t.Error("native binary version verification must execute the matching runner binaries")
	}
}

func TestGoReleaserReleaseCallSitePinsImmutableSource(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "release.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	for _, want := range []string{
		"checkout_ref: ${{ github.sha }}",
		"release_source_tag: ${{ needs.release-please.outputs.tag_name }}",
		"release_source_sha: ${{ github.sha }}",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release call site missing %q", want)
		}
	}
}

func readGoReleaserManifest(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", ".goreleaser.yaml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}
