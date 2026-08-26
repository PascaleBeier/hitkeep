package devtool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	json "hitkeep/jsonapi"
)

const (
	maxStructuredPaths = 25
	maxHandoffPaths    = 15
)

func ResolveWorkspace(path string) (Workspace, error) {
	workspace, stateRoot, err := workspaceIdentity(path)
	if err != nil {
		return Workspace{}, err
	}
	if err := os.MkdirAll(filepath.Join(workspace.StateDir, "runs"), 0o700); err != nil {
		return Workspace{}, fmt.Errorf("create workspace state: %w", err)
	}
	lock, err := lockStateRoot(stateRoot, "workspace-ports")
	if err != nil {
		return Workspace{}, err
	}
	defer func() { unlockStateRoot(lock) }()

	leasePath := filepath.Join(workspace.StateDir, "workspace.json")
	changedLease := false
	if saved, loadErr := loadWorkspace(leasePath); loadErr == nil {
		workspace.Ports = saved.Ports
	}
	if workspace.Ports.Backend == 0 {
		workspace.Ports, err = allocatePorts(stateRoot, workspace.ID)
		if err != nil {
			return Workspace{}, err
		}
		changedLease = true
	}
	if workspace.Ports.E2E == 0 {
		workspace.Ports.E2E, err = allocateE2EBase(stateRoot, workspace.ID)
		if err != nil {
			return Workspace{}, err
		}
		changedLease = true
	}
	setWorkspaceURLs(&workspace)
	if changedLease {
		if err := writeJSONAtomic(leasePath, workspace); err != nil {
			return Workspace{}, err
		}
	}
	unlockStateRoot(lock)
	lock = nil

	workspace.Branch, _ = gitOutput(workspace.Root, "branch", "--show-current")
	workspace.Head, _ = gitOutput(workspace.Root, "rev-parse", "HEAD")
	changed, _ := workingTreeChangedPaths(workspace.Root)
	workspace.DirtyCount = len(changed)
	workspace.ChangedPaths, workspace.ChangedPathsTruncated = boundedStrings(changed, maxStructuredPaths)
	workspace.UpdatedAt = time.Now().UTC()
	return workspace, nil
}

func workspaceIdentity(path string) (Workspace, string, error) {
	root, err := gitOutput(path, "rev-parse", "--show-toplevel")
	if err != nil {
		return Workspace{}, "", fmt.Errorf("resolve git worktree: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Workspace{}, "", fmt.Errorf("resolve worktree path: %w", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return Workspace{}, "", fmt.Errorf("resolve absolute worktree path: %w", err)
	}

	commonDir, err := gitOutput(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return Workspace{}, "", fmt.Errorf("resolve git common directory: %w", err)
	}
	commonDir, err = filepath.EvalSymlinks(commonDir)
	if err != nil {
		return Workspace{}, "", fmt.Errorf("resolve git common directory path: %w", err)
	}

	digest := sha256.Sum256([]byte(commonDir + "\x00" + root))
	id := hex.EncodeToString(digest[:8])
	stateRoot, err := stateRootDir()
	if err != nil {
		return Workspace{}, "", err
	}
	stateDir := filepath.Join(stateRoot, "workspaces", id)
	workspace := Workspace{
		ID:             id,
		Root:           root,
		GitCommonDir:   commonDir,
		ComposeProject: "hitkeep-" + id[:8],
		StateDir:       stateDir,
		UpdatedAt:      time.Now().UTC(),
	}
	return workspace, stateRoot, nil
}

func ListWorkspaces(path string) ([]Workspace, error) {
	current, _, err := workspaceIdentity(path)
	if err != nil {
		return nil, err
	}
	lines, err := gitLines(current.Root, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var roots []string
	for _, line := range lines {
		if after, ok := strings.CutPrefix(line, "worktree "); ok {
			roots = append(roots, strings.TrimSpace(after))
		}
	}
	workspaces := make([]Workspace, 0, len(roots))
	for _, root := range roots {
		workspace, _, resolveErr := workspaceIdentity(root)
		if resolveErr != nil {
			continue
		}
		if saved, loadErr := loadWorkspace(filepath.Join(workspace.StateDir, "workspace.json")); loadErr == nil {
			workspace.Ports = saved.Ports
			setWorkspaceURLs(&workspace)
		}
		workspace.Branch, _ = gitOutput(root, "branch", "--show-current")
		workspace.Head, _ = gitOutput(root, "rev-parse", "HEAD")
		changedPaths, _ := workingTreeChangedPaths(root)
		workspace.DirtyCount = len(changedPaths)
		workspace.ChangedPaths, workspace.ChangedPathsTruncated = boundedStrings(changedPaths, maxStructuredPaths)
		workspace.ActiveRuns = runSummariesFromState(workspace.StateDir, true, 10)
		var devRecord devSessionRecord
		if raw, readErr := os.ReadFile(filepath.Join(workspace.StateDir, "dev", "session.json")); readErr == nil && json.Unmarshal(raw, &devRecord) == nil {
			status := devRecord.DevStatus
			workspace.Dev = &status
		}
		workspaces = append(workspaces, workspace)
	}
	slices.SortFunc(workspaces, func(a, b Workspace) int { return strings.Compare(a.Root, b.Root) })
	return workspaces, nil
}

func ValidateWorkspacePath(workspace Workspace, candidate string) (string, error) {
	if candidate == "" {
		return workspace.Root, nil
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve path symlinks: %w", err)
	}
	relative, err := filepath.Rel(workspace.Root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("path is outside the selected worktree")
	}
	return resolved, nil
}

func stateRootDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("HK_STATE_DIR")); override != "" {
		absolute, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("resolve HK_STATE_DIR: %w", err)
		}
		return absolute, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache: %w", err)
	}
	return filepath.Join(cache, "hitkeep", "hk"), nil
}

func allocatePorts(stateRoot, workspaceID string) (Ports, error) {
	reserved := map[int]bool{}
	paths, _ := filepath.Glob(filepath.Join(stateRoot, "workspaces", "*", "workspace.json"))
	for _, path := range paths {
		workspace, err := loadWorkspace(path)
		if err != nil || workspace.ID == workspaceID || !workspaceRootAlive(workspace.Root) {
			continue
		}
		for _, port := range []int{workspace.Ports.Backend, workspace.Ports.Frontend, workspace.Ports.SMTP, workspace.Ports.MailUI} {
			reserved[port] = true
		}
	}
	defaults := []int{8080, 4200, 1025, 8025}
	selected := make([]int, 0, len(defaults))
	for _, port := range defaults {
		if !reserved[port] && portAvailable(port) {
			selected = append(selected, port)
			reserved[port] = true
			continue
		}
		candidate, err := nextAvailablePort(workspaceID, port, reserved)
		if err != nil {
			return Ports{}, err
		}
		selected = append(selected, candidate)
		reserved[candidate] = true
	}
	return Ports{Backend: selected[0], Frontend: selected[1], SMTP: selected[2], MailUI: selected[3]}, nil
}

func allocateE2EBase(stateRoot, workspaceID string) (int, error) {
	reserved := map[int]bool{}
	paths, _ := filepath.Glob(filepath.Join(stateRoot, "workspaces", "*", "workspace.json"))
	for _, path := range paths {
		workspace, err := loadWorkspace(path)
		if err != nil || workspace.ID == workspaceID || workspace.Ports.E2E == 0 || !workspaceRootAlive(workspace.Root) {
			continue
		}
		for _, offset := range []int{0, 1, 50, 51, 52, 53} {
			reserved[workspace.Ports.E2E+offset] = true
		}
	}
	digest := sha256.Sum256([]byte(workspaceID + "e2e"))
	start := 30000 + (int(digest[0])<<4+int(digest[1]))%10000
	for attempt := range 10000 {
		candidate := 30000 + (start-30000+attempt)%10000
		available := true
		for _, offset := range []int{0, 1, 50, 51, 52, 53} {
			if reserved[candidate+offset] || !portAvailable(candidate+offset) {
				available = false
				break
			}
		}
		if available {
			return candidate, nil
		}
	}
	return 0, errors.New("no isolated e2e port block is available")
}

func nextAvailablePort(workspaceID string, salt int, reserved map[int]bool) (int, error) {
	digest := sha256.Sum256([]byte(workspaceID + strconv.Itoa(salt)))
	start := 18000 + int(digest[0])<<5 + int(digest[1])%32
	for offset := range 10000 {
		port := 18000 + (start-18000+offset)%10000
		if !reserved[port] && portAvailable(port) {
			return port, nil
		}
	}
	return 0, errors.New("no development port is available")
}

func portAvailable(port int) bool {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

func setWorkspaceURLs(workspace *Workspace) {
	if workspace.Ports.Backend == 0 {
		return
	}
	workspace.URLs = URLs{
		API:     "http://127.0.0.1:" + strconv.Itoa(workspace.Ports.Backend),
		Web:     "http://127.0.0.1:" + strconv.Itoa(workspace.Ports.Frontend),
		Mailpit: "http://127.0.0.1:" + strconv.Itoa(workspace.Ports.MailUI),
	}
}

func probeWorkspaceServices(ctx context.Context, workspace Workspace) []Service {
	if workspace.Ports.Backend == 0 {
		return nil
	}
	definitions := []struct {
		name string
		port int
	}{
		{name: "backend", port: workspace.Ports.Backend},
		{name: "frontend", port: workspace.Ports.Frontend},
		{name: "smtp", port: workspace.Ports.SMTP},
		{name: "mailpit", port: workspace.Ports.MailUI},
	}
	services := make([]Service, 0, len(definitions))
	for _, definition := range definitions {
		address := net.JoinHostPort("127.0.0.1", strconv.Itoa(definition.port))
		dialer := net.Dialer{Timeout: 100 * time.Millisecond}
		connection, err := dialer.DialContext(ctx, "tcp", address)
		if err == nil {
			_ = connection.Close()
		}
		services = append(services, Service{Name: definition.name, Address: address, Reachable: err == nil})
	}
	return services
}

func workingTreeChangedPaths(root string) ([]string, error) {
	lines, err := gitLines(root, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	for index, line := range lines {
		if len(line) >= 4 {
			lines[index] = strings.TrimSpace(line[3:])
		}
	}
	return compactSorted(lines), nil
}

func boundedStrings(values []string, limit int) ([]string, bool) {
	if len(values) <= limit {
		return slices.Clone(values), false
	}
	return slices.Clone(values[:limit]), true
}

func workspaceRootAlive(root string) bool {
	if root == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(root, ".git"))
	return err == nil
}

func lockStateRoot(stateRoot, name string) (*os.File, error) {
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(stateRoot, name+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func unlockStateRoot(file *os.File) {
	if file == nil {
		return
	}
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	_ = file.Close()
}

func loadWorkspace(path string) (Workspace, error) {
	var workspace Workspace
	raw, err := os.ReadFile(path)
	if err != nil {
		return workspace, err
	}
	if err := json.Unmarshal(raw, &workspace); err != nil {
		return workspace, err
	}
	return workspace, nil
}

func gitOutput(dir string, args ...string) (string, error) {
	command := exec.CommandContext(context.Background(), "git", args...)
	command.Dir = dir
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func gitLines(dir string, args ...string) ([]string, error) {
	command := exec.CommandContext(context.Background(), "git", args...)
	command.Dir = dir
	raw, err := command.Output()
	output := strings.TrimRight(string(raw), "\r\n")
	if err != nil || output == "" {
		return nil, err
	}
	return strings.Split(output, "\n"), nil
}

func compactSorted(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}
