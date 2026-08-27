package devtool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	json "hitkeep/jsonapi"
)

const doctorCommandTimeout = 15 * time.Second

type App struct {
	workspace          Workspace
	executable         string
	devProbe           func(context.Context) []DevService
	devDetachedStart   func(context.Context, devSessionRecord, DevRequest) error
	devReadyTimeout    time.Duration
	devProbeInterval   time.Duration
	devGracePeriod     time.Duration
	doctorProbeTimeout time.Duration
}

func NewApp(workspacePath string) (*App, error) {
	workspace, err := ResolveWorkspace(workspacePath)
	if err != nil {
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve hk executable: %w", err)
	}
	return &App{
		workspace: workspace, executable: executable,
		devReadyTimeout: devReadyTimeout, devProbeInterval: devProbeInterval, devGracePeriod: devGracePeriod,
		doctorProbeTimeout: doctorCommandTimeout,
	}, nil
}

func (a *App) Workspace(ctx context.Context) (Workspace, error) {
	workspace, err := ResolveWorkspace(a.workspace.Root)
	if err == nil {
		workspace.Services = probeWorkspaceServices(ctx, workspace)
		runs, _ := a.ListRuns(100)
		for _, run := range runs {
			if !isTerminal(run.Status) {
				workspace.ActiveRuns = append(workspace.ActiveRuns, summarizeRun(run))
			}
		}
		if status, statusErr := a.DevStatus(ctx); statusErr == nil {
			workspace.Dev = &status
		}
	}
	return workspace, err
}

func (a *App) RecentRuns(limit int) ([]RunSummary, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	runs, err := a.ListRuns(100)
	if err != nil {
		return nil, err
	}
	summaries := make([]RunSummary, 0, min(limit, len(runs)))
	for _, run := range runs {
		summaries = append(summaries, summarizeRun(run))
		if len(summaries) >= limit {
			break
		}
	}
	return summaries, nil
}

func (a *App) Workspaces(_ context.Context) ([]Workspace, error) {
	return ListWorkspaces(a.workspace.Root)
}

func (a *App) Handoff(ctx context.Context) (Handoff, error) {
	workspace, err := a.Workspace(ctx)
	if err != nil {
		return Handoff{}, err
	}
	boundedPaths, truncated := boundedStrings(workspace.ChangedPaths, maxHandoffPaths)
	workspace.ChangedPaths = boundedPaths
	truncated = truncated || workspace.ChangedPathsTruncated
	workspace.ChangedPathsTruncated = truncated
	runs, _ := a.ListRuns(5)
	recent := make([]RunSummary, 0, len(runs))
	for _, run := range runs {
		recent = append(recent, summarizeRun(run))
	}
	next := []string{"./hk qa plan changed --output json"}
	if workspace.DirtyCount == 0 {
		next = []string{"./hk workspace status --output json"}
	}
	return Handoff{Workspace: workspace, RecentRuns: recent, NextActions: next, Truncated: truncated, GeneratedAt: time.Now().UTC()}, nil
}

func (a *App) Doctor(ctx context.Context) DoctorReport {
	timeout := a.doctorProbeTimeout
	if timeout <= 0 {
		timeout = doctorCommandTimeout
	}
	probeContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	goVersion := requiredVersion(a.workspace.Root, "go.mod", "go ")
	nodeVersion := requiredVersion(a.workspace.Root, filepath.Join("frontend", "dashboard", ".node-version"), "")
	npmVersion := requiredPackageManagerVersion(a.workspace.Root)
	npmExecutable, npmArgs := a.preferredNPMProbe()
	probes := []func(context.Context) Check{
		func(ctx context.Context) Check { return checkCommand(ctx, "git", "git", "--version") },
		func(ctx context.Context) Check {
			return managedToolchainCheck(ctx, "go", goVersion, "go"+goVersion, a.preferredDeveloperExecutable("go"), "version")
		},
		func(ctx context.Context) Check {
			return managedToolchainCheck(ctx, "node", nodeVersion, "v"+nodeVersion, a.preferredDeveloperExecutable("node"), "--version")
		},
		func(ctx context.Context) Check {
			return managedToolchainCheck(ctx, "npm", npmVersion, npmVersion, npmExecutable, npmArgs...)
		},
		func(ctx context.Context) Check { return checkCommand(ctx, "c-compiler", "cc", "--version") },
		func(ctx context.Context) Check {
			return checkCommand(ctx, "docker", "docker", "version", "--format", "{{.Server.Version}}")
		},
		func(ctx context.Context) Check {
			return checkCommand(ctx, "compose", "docker", "compose", "version", "--short")
		},
		func(ctx context.Context) Check { return checkCommand(ctx, "buildx", "docker", "buildx", "version") },
		func(ctx context.Context) Check {
			return checkExactCommand(ctx, "zizmor", ToolVersion("zizmor"), ToolVersion("zizmor"), "zizmor", "--version")
		},
	}
	checks := make([]Check, len(probes))
	var wait sync.WaitGroup
	for index, probe := range probes {
		wait.Go(func() { checks[index] = probe(probeContext) })
	}
	wait.Wait()
	statuses := map[string]bool{}
	for _, check := range checks {
		statuses[check.Name] = check.Status == "ok"
	}
	toolchainReady := statuses["go"] && statuses["node"] && statuses["npm"] && statuses["c-compiler"]
	containerReady := statuses["docker"] && statuses["compose"]
	prQA := toolchainReady && statuses["zizmor"]
	fullQA := prQA && statuses["docker"] && statuses["buildx"]
	ready := statuses["git"] && containerReady
	return DoctorReport{Ready: ready, Capabilities: DoctorCapabilities{ContainerDevelopment: containerReady, PRQA: prQA, FullQA: fullQA}, Checks: checks}
}

func (a *App) QAPlan(ctx context.Context, profile, baseRef string) (QAPlan, error) {
	if err := VerifyDeveloperSource(a.workspace.Root); err != nil {
		return QAPlan{}, err
	}
	return a.buildQAPlan(ctx, profile, baseRef)
}

func (a *App) Catalog() Catalog {
	catalog := CatalogSnapshot()
	for index := range catalog.Variants {
		catalog.Variants[index].LocalImage = a.localImageRef(catalog.Variants[index])
	}
	return catalog
}

func (a *App) Root() string { return a.workspace.Root }

func (a *App) WorkspaceID() string { return a.workspace.ID }

func (a *App) localImageRef(variant Variant) string {
	return variant.LocalImage + "-" + a.workspace.ID[:8]
}

func (a *App) SharedCacheEnvironment() []string {
	environment := []string{"GOTOOLCHAIN=local"}
	paths, err := a.managedToolchainPaths()
	if err != nil {
		return environment
	}
	pathEntries := make([]string, 0, 3)
	if isExecutableFile(paths.GoExecutable) {
		pathEntries = append(pathEntries, filepath.Dir(paths.GoExecutable))
		environment = append(environment, "GOCACHE="+paths.GoBuildCache, "GOMODCACHE="+paths.GoModuleCache)
	}
	if isExecutableFile(paths.NodeExecutable) {
		pathEntries = append(pathEntries, filepath.Dir(paths.NodeExecutable))
		environment = append(environment, "NPM_CONFIG_CACHE="+paths.NPMCache, "PLAYWRIGHT_BROWSERS_PATH="+paths.PlaywrightCache)
	}
	if len(pathEntries) > 0 {
		if currentPath := os.Getenv("PATH"); currentPath != "" {
			pathEntries = append(pathEntries, currentPath)
		}
		environment = append(environment, "PATH="+strings.Join(pathEntries, string(os.PathListSeparator)))
	}
	return environment
}

func (a *App) commandEnvironment(overrides []string) []string {
	environment := a.SharedCacheEnvironment()
	if root := os.Getenv("HK_RUN_TEMP_ROOT"); a.isRunTempRoot(root) {
		environment = append(environment, runTempEnvironment(root)...)
	}
	return mergedCommandEnvironment(append(environment, overrides...))
}

func (a *App) isRunTempRoot(root string) bool {
	if root == "" {
		return false
	}
	runID := filepath.Base(root)
	if err := validateRunID(runID); err != nil || filepath.Clean(root) != a.runTempRoot(runID) {
		return false
	}
	info, err := os.Stat(root)
	return err == nil && info.IsDir()
}

func (a *App) ComposeEnvironment(variant Variant) []string {
	toolchain, _ := a.ToolchainConfig()
	values := map[string]string{
		"HK_COMPOSE_PROJECT":         a.workspace.ComposeProject,
		"HK_BACKEND_PORT":            fmt.Sprint(a.workspace.Ports.Backend),
		"HK_FRONTEND_PORT":           fmt.Sprint(a.workspace.Ports.Frontend),
		"HK_SMTP_PORT":               fmt.Sprint(a.workspace.Ports.SMTP),
		"HK_MAIL_UI_PORT":            fmt.Sprint(a.workspace.Ports.MailUI),
		"HK_GO_VERSION":              toolchain.Go,
		"HK_NODE_VERSION":            toolchain.Node,
		"HK_NPM_VERSION":             toolchain.NPM,
		"HITKEEP_E2E_PORT":           fmt.Sprint(a.workspace.Ports.E2E),
		"HITKEEP_E2E_MAIL_PORT":      fmt.Sprint(a.workspace.Ports.E2E + 53),
		"HITKEEP_E2E_HTML_REPORT":    filepath.Join(a.workspace.StateDir, "e2e", "report"),
		"HITKEEP_E2E_OUTPUT_DIR":     filepath.Join(a.workspace.StateDir, "e2e", "results"),
		"HITKEEP_FRONTEND_CACHE_DIR": filepath.Join(a.workspace.StateDir, "frontend-cache"),
		"HITKEEP_GO_BUILD_TAGS":      strings.Join(variant.BuildTags, " "),
		"HITKEEP_PUBLIC_URL":         a.workspace.URLs.Web,
		"HITKEEP_MAIL_DRIVER":        "smtp",
		"HITKEEP_MAIL_ENCRYPTION":    "none",
		"HITKEEP_MCP_ENABLED":        "true",
		// Worktree data is disposable local state. Retained recovery bundles
		// still preserve the pre-recovery database and WAL for inspection.
		"HITKEEP_DB_AUTO_RECOVER_WAL": "true",
		// Provider discovery and button rendering require complete client pairs.
		// These values are confined to hk-managed local development processes;
		// real provider credentials are used only in staging and production.
		"HITKEEP_SOCIAL_GOOGLE_CLIENT_ID":        "hitkeep-local-google-client",
		"HITKEEP_SOCIAL_GOOGLE_CLIENT_SECRET":    "hitkeep-local-google-secret",
		"HITKEEP_SOCIAL_GITHUB_CLIENT_ID":        "hitkeep-local-github-client",
		"HITKEEP_SOCIAL_GITHUB_CLIENT_SECRET":    "hitkeep-local-github-secret",
		"HITKEEP_SOCIAL_MICROSOFT_CLIENT_ID":     "hitkeep-local-microsoft-client",
		"HITKEEP_SOCIAL_MICROSOFT_CLIENT_SECRET": "hitkeep-local-microsoft-secret",
	}
	for key, value := range variant.Environment {
		values[key] = replaceDefaultPorts(value, a.workspace.Ports)
	}
	values["HITKEEP_JWT_SECRET"] = "dev-workspace-" + a.workspace.ID
	environment := make([]string, 0, len(values))
	for key, value := range values {
		environment = append(environment, key+"="+value)
	}
	slices.Sort(environment)
	return environment
}

func checkCommand(ctx context.Context, name string, executable string, args ...string) Check {
	path, err := exec.LookPath(executable)
	if err != nil {
		return Check{Name: name, Status: "missing", Remediation: "install " + executable + " or use the container runtime"}
	}
	probeContext, cancel := context.WithTimeout(ctx, doctorCommandTimeout)
	defer cancel()
	command := exec.CommandContext(probeContext, path, args...)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	command.WaitDelay = 250 * time.Millisecond
	output, runErr := command.CombinedOutput()
	if runErr != nil {
		if errors.Is(probeContext.Err(), context.DeadlineExceeded) {
			return Check{Name: name, Status: "unavailable", Detected: "timed out after " + doctorCommandTimeout.String(), Remediation: "check " + executable + " configuration"}
		}
		return Check{Name: name, Status: "unavailable", Detected: strings.TrimSpace(string(output)), Remediation: "check " + executable + " configuration"}
	}
	return Check{Name: name, Status: "ok", Detected: strings.TrimSpace(string(output))}
}

func checkExactCommand(ctx context.Context, name, required, expected, executable string, args ...string) Check {
	check := checkCommand(ctx, name, executable, args...)
	check.Required = required
	if check.Status == "ok" && required != "" && !strings.Contains(check.Detected, expected) {
		check.Status = "mismatch"
		check.Remediation = "install the exact repository version or use the container runtime"
	}
	return check
}

func managedToolchainCheck(ctx context.Context, name, required, expected, executable string, args ...string) Check {
	check := checkExactCommand(ctx, name, required, expected, executable, args...)
	if check.Status != "ok" {
		check.Remediation = "run ./hk setup to provision the pinned managed toolchain"
	}
	return check
}

func requiredVersion(root, relativePath, prefix string) string {
	raw, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if prefix == "" && line != "" {
			return line
		}
		if after, ok := strings.CutPrefix(line, prefix); ok {
			return strings.TrimSpace(after)
		}
	}
	return ""
}

func requiredPackageManagerVersion(root string) string {
	raw, err := os.ReadFile(filepath.Join(root, "frontend", "dashboard", "package.json"))
	if err != nil {
		return ""
	}
	manifest := struct {
		PackageManager string `json:"packageManager"`
	}{}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return ""
	}
	version, ok := strings.CutPrefix(manifest.PackageManager, "npm@")
	if !ok || !exactToolVersionPattern.MatchString(version) {
		return ""
	}
	return version
}

func changedPaths(root, baseRef string) ([]string, error) {
	mergeBase, err := gitOutput(root, "merge-base", baseRef, "HEAD")
	if err != nil {
		return nil, err
	}
	tracked, err := gitLines(root, "diff", "--name-only", mergeBase)
	if err != nil {
		return nil, err
	}
	untracked, _ := gitLines(root, "ls-files", "--others", "--exclude-standard")
	return compactSorted(append(tracked, untracked...)), nil
}

func replaceDefaultPorts(value string, ports Ports) string {
	replacements := map[string]string{
		"localhost:8080": "localhost:" + fmt.Sprint(ports.Backend),
		"localhost:4200": "localhost:" + fmt.Sprint(ports.Frontend),
		"127.0.0.1:8080": "127.0.0.1:" + fmt.Sprint(ports.Backend),
		"127.0.0.1:4200": "127.0.0.1:" + fmt.Sprint(ports.Frontend),
	}
	for from, to := range replacements {
		value = strings.ReplaceAll(value, from, to)
	}
	return value
}

func maxParallelism() int {
	if override := validatedQASlotOverride(); override != "" {
		if slots, err := strconv.Atoi(override); err == nil {
			return slots
		}
	}
	if runtime.NumCPU() <= 2 {
		return 1
	}
	return max(2, min(8, runtime.NumCPU()/2))
}

func validatedQASlotOverride() string {
	value := strings.TrimSpace(os.Getenv("HK_QA_SLOTS"))
	slots, err := strconv.Atoi(value)
	if err != nil || slots < 1 || slots > 32 {
		return ""
	}
	return strconv.Itoa(slots)
}
