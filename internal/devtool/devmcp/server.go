package devmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/internal/devtool"
)

const (
	maxRequestedLogLines           = 200
	centralAppCacheTTL             = 5 * time.Second
	centralAppCacheLimit           = 128
	centralMCPConnectTTL           = 30 * time.Second
	centralMCPIdleTTL              = 30 * time.Second
	centralMCPNotificationDrainTTL = centralMCPIdleTTL
	centralMCPPoolLimit            = 32
)

type workspaceInput struct {
	Workspace string `json:"workspace,omitempty" jsonschema:"Optional client-root name, workspace ID, or workspace path; required only when more than one HitKeep workspace root is active"`
}
type emptyInput struct{ workspaceInput }
type qaPlanInput struct {
	workspaceInput
	Profile string `json:"profile,omitempty" jsonschema:"Profile to plan: changed, pr, or full"`
	BaseRef string `json:"base_ref,omitempty" jsonschema:"Git base ref used only for the changed profile"`
}
type runInput struct {
	workspaceInput
	RunID string `json:"run_id" jsonschema:"Validated hk run identifier"`
}
type logsInput struct {
	workspaceInput
	RunID  string `json:"run_id" jsonschema:"Validated hk run identifier"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum lines to return, from 1 through 200"`
	Cursor int    `json:"cursor,omitempty" jsonschema:"Previous next_cursor for incremental reads"`
	GateID string `json:"gate_id,omitempty" jsonschema:"Optional canonical gate identifier"`
}
type runListInput struct {
	workspaceInput
	Limit int `json:"limit,omitempty" jsonschema:"Maximum recent runs to return, from 1 through 100"`
}
type variantInput struct {
	workspaceInput
	Variant string `json:"variant,omitempty" jsonschema:"Build variant: self-hosted or cloud"`
}
type devInput struct{ workspaceInput }
type devStartInput struct {
	workspaceInput
	Variant string `json:"variant,omitempty" jsonschema:"Build variant: self-hosted or cloud"`
	Seed    bool   `json:"seed,omitempty" jsonschema:"Seed isolated demo data before starting"`
}
type devLogsInput struct {
	workspaceInput
	Cursor int64 `json:"cursor,omitempty" jsonschema:"Next event cursor for incremental reads"`
	Limit  int   `json:"limit,omitempty" jsonschema:"Maximum events to return, from 1 through 200"`
	Follow bool  `json:"follow,omitempty" jsonschema:"Continue streaming events until cancellation or session termination"`
}
type qaStartInput struct {
	workspaceInput
	Profile string   `json:"profile,omitempty" jsonschema:"QA profile: changed, pr, or full"`
	GateIDs []string `json:"gate_ids,omitempty" jsonschema:"Optional canonical gate identifiers"`
}
type buildInput struct {
	workspaceInput
	Variant string `json:"variant,omitempty" jsonschema:"Build variant: self-hosted or cloud"`
	Target  string `json:"target,omitempty" jsonschema:"Build target: binary or image"`
}
type screenshotInput struct {
	workspaceInput
	Routes    []string `json:"routes,omitempty" jsonschema:"Local dashboard routes captured in one browser session"`
	Viewport  string   `json:"viewport,omitempty" jsonschema:"Viewport preset: desktop or mobile"`
	Theme     string   `json:"theme,omitempty" jsonschema:"Color scheme: light or dark"`
	Scale     int      `json:"scale,omitempty" jsonschema:"Device pixel ratio: 1 or 2"`
	WaitMS    int      `json:"wait_ms,omitempty" jsonschema:"Bounded visual settle time after route readiness"`
	FullPage  bool     `json:"full_page,omitempty" jsonschema:"Capture the complete document instead of the viewport"`
	Selector  string   `json:"selector,omitempty" jsonschema:"Optional visible CSS selector for a single-route component capture"`
	Anonymous bool     `json:"anonymous,omitempty" jsonschema:"Capture without seeded development authentication"`
}

type envelopeOutput struct {
	devtool.Envelope
}

type workspaceScoped interface {
	workspaceSelector() string
}

func (input workspaceInput) workspaceSelector() string { return strings.TrimSpace(input.Workspace) }

type appResolver interface {
	Resolve(context.Context, *mcp.ServerSession, string) (*devtool.App, error)
}

type toolDelegatingResolver interface {
	DelegateTool(context.Context, *mcp.CallToolRequest, string, workspaceScoped) (*mcp.CallToolResult, envelopeOutput, error)
}

type resourceDelegatingResolver interface {
	DelegateResource(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error)
}

type staticAppResolver struct{ app *devtool.App }

func (resolver staticAppResolver) Resolve(_ context.Context, _ *mcp.ServerSession, selector string) (*devtool.App, error) {
	if selector == "" || selector == resolver.app.WorkspaceID() || filepath.Clean(selector) == filepath.Clean(resolver.app.Root()) {
		return resolver.app, nil
	}
	return nil, fmt.Errorf("workspace %q is outside the configured worktree", selector)
}

type rootedApp struct {
	app  *devtool.App
	name string
}

type workspaceMCPConnector func(context.Context, context.Context, *devtool.App, *mcp.ClientOptions) (*mcp.ClientSession, error)

type cachedWorkspaceApp struct {
	app       *devtool.App
	expiresAt time.Time
}

type workspaceMCPKey struct {
	outerSession *mcp.ServerSession
	workspaceID  string
}

type workspaceMCPProgressRoute struct {
	outerToken any
	expiresAt  time.Time
}

type pooledWorkspaceMCP struct {
	key            workspaceMCPKey
	app            *devtool.App
	outerSession   *mcp.ServerSession
	ready          chan struct{}
	session        *mcp.ClientSession
	lifetimeCancel context.CancelFunc
	connectErr     error
	activeCalls    int
	lastUsed       time.Time
	idleTimer      *time.Timer
	progressTokens map[string]workspaceMCPProgressRoute
	callCancels    map[uint64]context.CancelFunc
	closed         bool
}

type workspaceMCPLease struct {
	resolver      *centralAppResolver
	child         *pooledWorkspaceMCP
	callContext   context.Context
	callID        uint64
	progressToken string
	releaseOnce   sync.Once
}

type centralAppResolver struct {
	fallback         string
	connect          workspaceMCPConnector
	loadApp          func(string) (*devtool.App, error)
	now              func() time.Time
	cacheMu          sync.Mutex
	apps             map[string]cachedWorkspaceApp
	childMu          sync.Mutex
	children         map[workspaceMCPKey]*pooledWorkspaceMCP
	childIdleTTL     time.Duration
	childPoolClosed  bool
	callSequence     atomic.Uint64
	progressSequence atomic.Uint64
}

func newCentralAppResolver(fallback string, connect workspaceMCPConnector) *centralAppResolver {
	return &centralAppResolver{
		fallback: fallback, connect: connect, apps: map[string]cachedWorkspaceApp{},
		children: map[workspaceMCPKey]*pooledWorkspaceMCP{}, childIdleTTL: centralMCPIdleTTL,
	}
}

func (resolver *centralAppResolver) connector() workspaceMCPConnector {
	if resolver.connect != nil {
		return resolver.connect
	}
	return connectWorkspaceMCP
}

func (resolver *centralAppResolver) Resolve(ctx context.Context, session *mcp.ServerSession, selector string) (*devtool.App, error) {
	apps, rootsErr := appsFromClientRoots(ctx, session, resolver.appForPath)
	if selector != "" {
		cleanSelector := filepath.Clean(selector)
		for _, candidate := range apps {
			if selector == candidate.app.WorkspaceID() || cleanSelector == filepath.Clean(candidate.app.Root()) {
				return candidate.app, nil
			}
		}
		var namedMatches []*devtool.App
		for _, candidate := range apps {
			if selector == candidate.name {
				namedMatches = append(namedMatches, candidate.app)
			}
		}
		if len(namedMatches) == 1 {
			return namedMatches[0], nil
		}
		if len(namedMatches) > 1 {
			return nil, fmt.Errorf("workspace name %q is ambiguous; pass a workspace ID or path", selector)
		}
		if rootsErr != nil {
			return nil, fmt.Errorf("read MCP client roots: %w", rootsErr)
		}
		return nil, fmt.Errorf("workspace %q is not one of the client's HitKeep roots", selector)
	}
	if len(apps) == 1 {
		return apps[0].app, nil
	}
	if len(apps) > 1 {
		identifiers := make([]string, 0, len(apps))
		for _, candidate := range apps {
			identifiers = append(identifiers, candidate.app.WorkspaceID())
		}
		return nil, fmt.Errorf("multiple HitKeep workspaces are active; pass workspace as one of: %s", strings.Join(identifiers, ", "))
	}
	if resolver.fallback != "" {
		if app, err := resolver.appForPath(resolver.fallback); err == nil {
			return app, nil
		}
	}
	if rootsErr != nil {
		return nil, fmt.Errorf("read MCP client roots: %w", rootsErr)
	}
	return nil, errors.New("no HitKeep workspace root is active; open a HitKeep workspace or pass workspace explicitly")
}

func (resolver *centralAppResolver) appForPath(path string) (*devtool.App, error) {
	key := filepath.Clean(path)
	if absolute, err := filepath.Abs(key); err == nil {
		key = absolute
	}
	now := time.Now()
	if resolver.now != nil {
		now = resolver.now()
	}
	resolver.cacheMu.Lock()
	if cached, ok := resolver.apps[key]; ok && now.Before(cached.expiresAt) {
		resolver.cacheMu.Unlock()
		return cached.app, nil
	}
	delete(resolver.apps, key)

	loader := newHitKeepApp
	if resolver.loadApp != nil {
		loader = resolver.loadApp
	}
	app, err := loader(path)
	if err != nil {
		resolver.cacheMu.Unlock()
		return nil, err
	}
	entry := cachedWorkspaceApp{app: app, expiresAt: now.Add(centralAppCacheTTL)}
	if resolver.apps == nil {
		resolver.apps = map[string]cachedWorkspaceApp{}
	}
	if len(resolver.apps) >= centralAppCacheLimit {
		for cachedKey, cached := range resolver.apps {
			if !now.Before(cached.expiresAt) {
				delete(resolver.apps, cachedKey)
			}
		}
	}
	if len(resolver.apps) >= centralAppCacheLimit {
		var oldestKey string
		var oldestExpiry time.Time
		for cachedKey, cached := range resolver.apps {
			if oldestKey == "" || cached.expiresAt.Before(oldestExpiry) {
				oldestKey, oldestExpiry = cachedKey, cached.expiresAt
			}
		}
		delete(resolver.apps, oldestKey)
	}
	resolver.apps[key] = entry
	resolver.cacheMu.Unlock()
	return app, nil
}

func (resolver *centralAppResolver) acquireWorkspaceMCP(ctx context.Context, app *devtool.App, outer *mcp.ServerSession, outerProgress any) (*workspaceMCPLease, error) {
	if outer == nil {
		return nil, errors.New("MCP session is unavailable")
	}
	key := workspaceMCPKey{outerSession: outer, workspaceID: app.WorkspaceID()}
	for {
		resolver.childMu.Lock()
		if resolver.childPoolClosed {
			resolver.childMu.Unlock()
			return nil, errors.New("workspace MCP pool is closed")
		}
		child := resolver.children[key]
		if child == nil || child.closed {
			child = &pooledWorkspaceMCP{
				key: key, app: app, outerSession: outer, ready: make(chan struct{}),
				progressTokens: map[string]workspaceMCPProgressRoute{}, callCancels: map[uint64]context.CancelFunc{}, lastUsed: time.Now(),
			}
			resolver.children[key] = child
			evicted := resolver.pruneWorkspaceMCPPoolLocked(child)
			resolver.childMu.Unlock()
			for _, stale := range evicted {
				go resolver.closePooledWorkspaceMCP(stale)
			}
			go resolver.connectPooledWorkspaceMCP(child) //nolint:gosec // pooled connection intentionally outlives the initiating request
		} else {
			resolver.childMu.Unlock()
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-child.ready:
		}

		resolver.childMu.Lock()
		if child.connectErr != nil {
			err := child.connectErr
			resolver.childMu.Unlock()
			return nil, err
		}
		if resolver.children[key] != child || child.closed || child.session == nil {
			resolver.childMu.Unlock()
			continue
		}
		now := time.Now()
		child.activeCalls++
		child.lastUsed = now
		pruneWorkspaceMCPProgressLocked(child, now)
		if child.idleTimer != nil {
			child.idleTimer.Stop()
			child.idleTimer = nil
		}
		progressToken := ""
		if outerProgress != nil {
			progressToken = fmt.Sprintf("hk-broker-%d", resolver.progressSequence.Add(1))
			child.progressTokens[progressToken] = workspaceMCPProgressRoute{outerToken: outerProgress}
		}
		callContext, callCancel := context.WithCancel(ctx) //nolint:gosec // the lease owns and releases this cancel function
		callID := resolver.callSequence.Add(1)
		child.callCancels[callID] = callCancel
		resolver.childMu.Unlock()
		return &workspaceMCPLease{
			resolver: resolver, child: child, callContext: callContext, callID: callID, progressToken: progressToken,
		}, nil
	}
}

func (resolver *centralAppResolver) connectPooledWorkspaceMCP(child *pooledWorkspaceMCP) {
	lifetimeContext, lifetimeCancel := context.WithCancel(context.Background())
	resolver.childMu.Lock()
	if child.closed || resolver.children[child.key] != child {
		child.connectErr = errors.New("workspace MCP connection closed before startup")
		close(child.ready)
		resolver.childMu.Unlock()
		lifetimeCancel()
		return
	}
	child.lifetimeCancel = lifetimeCancel
	resolver.childMu.Unlock()

	connectContext, connectCancel := context.WithTimeout(context.Background(), centralMCPConnectTTL)
	defer connectCancel()
	options := &mcp.ClientOptions{
		KeepAlive: centralMCPIdleTTL,
		ProgressNotificationHandler: func(forwardContext context.Context, request *mcp.ProgressNotificationClientRequest) {
			resolver.forwardWorkspaceMCPProgress(forwardContext, child, request)
		},
		LoggingMessageHandler: func(forwardContext context.Context, request *mcp.LoggingMessageRequest) {
			resolver.forwardWorkspaceMCPLog(forwardContext, child, request)
		},
	}
	session, err := resolver.connector()(connectContext, lifetimeContext, child.app, options)

	resolver.childMu.Lock()
	if err != nil {
		child.connectErr = err
		child.closed = true
		if resolver.children[child.key] == child {
			delete(resolver.children, child.key)
		}
		close(child.ready)
		resolver.childMu.Unlock()
		lifetimeCancel()
		if session != nil {
			_ = session.Close()
		}
		return
	}
	if child.closed || resolver.children[child.key] != child {
		child.connectErr = errors.New("workspace MCP connection closed during startup")
		close(child.ready)
		resolver.childMu.Unlock()
		_ = session.Close()
		lifetimeCancel()
		return
	}
	child.session = session
	child.lastUsed = time.Now()
	close(child.ready)
	resolver.scheduleWorkspaceMCPIdleLocked(child)
	resolver.childMu.Unlock()

	go func() {
		_ = session.Wait()
		resolver.workspaceMCPExited(child)
	}()
}

func (resolver *centralAppResolver) forwardWorkspaceMCPProgress(ctx context.Context, child *pooledWorkspaceMCP, request *mcp.ProgressNotificationClientRequest) {
	if request == nil || request.Params == nil {
		return
	}
	childToken, ok := request.Params.ProgressToken.(string)
	if !ok {
		return
	}
	resolver.childMu.Lock()
	route, ok := child.progressTokens[childToken]
	if ok && !route.expiresAt.IsZero() && !time.Now().Before(route.expiresAt) {
		delete(child.progressTokens, childToken)
		ok = false
	}
	outer := child.outerSession
	closed := child.closed
	resolver.childMu.Unlock()
	if !ok || closed || outer == nil {
		return
	}
	params := *request.Params
	params.ProgressToken = route.outerToken
	_ = outer.NotifyProgress(ctx, &params)
}

func pruneWorkspaceMCPProgressLocked(child *pooledWorkspaceMCP, now time.Time) {
	for token, route := range child.progressTokens {
		if !route.expiresAt.IsZero() && !now.Before(route.expiresAt) {
			delete(child.progressTokens, token)
		}
	}
}

func (resolver *centralAppResolver) forwardWorkspaceMCPLog(ctx context.Context, child *pooledWorkspaceMCP, request *mcp.LoggingMessageRequest) {
	if request == nil || request.Params == nil {
		return
	}
	resolver.childMu.Lock()
	outer := child.outerSession
	closed := child.closed
	resolver.childMu.Unlock()
	if closed || outer == nil {
		return
	}
	_ = outer.Log(ctx, request.Params)
}

func (resolver *centralAppResolver) scheduleWorkspaceMCPIdleLocked(child *pooledWorkspaceMCP) {
	if child.closed || child.activeCalls != 0 || resolver.children[child.key] != child {
		return
	}
	if child.idleTimer != nil {
		child.idleTimer.Stop()
	}
	ttl := resolver.childIdleTTL
	if ttl <= 0 {
		ttl = centralMCPIdleTTL
	}
	child.idleTimer = time.AfterFunc(ttl, func() { resolver.expireWorkspaceMCP(child) })
}

func (resolver *centralAppResolver) expireWorkspaceMCP(child *pooledWorkspaceMCP) {
	resolver.childMu.Lock()
	if child.closed || child.activeCalls != 0 || resolver.children[child.key] != child {
		resolver.childMu.Unlock()
		return
	}
	resolver.detachWorkspaceMCPLocked(child)
	resolver.childMu.Unlock()
	resolver.closePooledWorkspaceMCP(child)
}

func (resolver *centralAppResolver) invalidateWorkspaceMCP(child *pooledWorkspaceMCP) {
	resolver.childMu.Lock()
	if resolver.children[child.key] != child || child.closed {
		resolver.childMu.Unlock()
		return
	}
	resolver.detachWorkspaceMCPLocked(child)
	resolver.childMu.Unlock()
	go resolver.closePooledWorkspaceMCP(child)
}

func (resolver *centralAppResolver) workspaceMCPExited(child *pooledWorkspaceMCP) {
	resolver.childMu.Lock()
	if resolver.children[child.key] != child || child.closed {
		resolver.childMu.Unlock()
		return
	}
	resolver.detachWorkspaceMCPLocked(child)
	cancel := child.lifetimeCancel
	resolver.childMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (resolver *centralAppResolver) detachWorkspaceMCPLocked(child *pooledWorkspaceMCP) {
	if resolver.children[child.key] == child {
		delete(resolver.children, child.key)
	}
	child.closed = true
	if child.idleTimer != nil {
		child.idleTimer.Stop()
		child.idleTimer = nil
	}
	clear(child.progressTokens)
	for _, cancel := range child.callCancels {
		cancel()
	}
	clear(child.callCancels)
}

func (resolver *centralAppResolver) pruneWorkspaceMCPPoolLocked(keep *pooledWorkspaceMCP) []*pooledWorkspaceMCP {
	var evicted []*pooledWorkspaceMCP
	for len(resolver.children) > centralMCPPoolLimit {
		var oldest *pooledWorkspaceMCP
		for _, candidate := range resolver.children {
			if candidate == keep || candidate.closed || candidate.session == nil || candidate.activeCalls != 0 {
				continue
			}
			if oldest == nil || candidate.lastUsed.Before(oldest.lastUsed) {
				oldest = candidate
			}
		}
		if oldest == nil {
			break
		}
		resolver.detachWorkspaceMCPLocked(oldest)
		evicted = append(evicted, oldest)
	}
	return evicted
}

func (resolver *centralAppResolver) closePooledWorkspaceMCP(child *pooledWorkspaceMCP) {
	if child.lifetimeCancel != nil {
		child.lifetimeCancel()
	}
	if child.session != nil {
		_ = child.session.Close()
	}
}

func (resolver *centralAppResolver) Close() {
	resolver.childMu.Lock()
	resolver.childPoolClosed = true
	children := make([]*pooledWorkspaceMCP, 0, len(resolver.children))
	for _, child := range resolver.children {
		resolver.detachWorkspaceMCPLocked(child)
		children = append(children, child)
	}
	resolver.childMu.Unlock()
	for _, child := range children {
		if child.lifetimeCancel != nil {
			child.lifetimeCancel()
		}
		if child.session != nil {
			_ = child.session.Close()
		}
	}
}

func (lease *workspaceMCPLease) Release() {
	if lease == nil {
		return
	}
	lease.releaseOnce.Do(func() {
		resolver := lease.resolver
		resolver.childMu.Lock()
		if route, ok := lease.child.progressTokens[lease.progressToken]; ok {
			route.expiresAt = time.Now().Add(centralMCPNotificationDrainTTL)
			lease.child.progressTokens[lease.progressToken] = route
		}
		callCancel := lease.child.callCancels[lease.callID]
		delete(lease.child.callCancels, lease.callID)
		if lease.child.activeCalls > 0 {
			lease.child.activeCalls--
		}
		lease.child.lastUsed = time.Now()
		resolver.scheduleWorkspaceMCPIdleLocked(lease.child)
		resolver.childMu.Unlock()
		if callCancel != nil {
			callCancel()
		}
	})
}

func (resolver *centralAppResolver) DelegateTool(ctx context.Context, request *mcp.CallToolRequest, command string, input workspaceScoped) (*mcp.CallToolResult, envelopeOutput, error) {
	app, err := resolver.Resolve(ctx, request.Session, input.workspaceSelector())
	if err != nil {
		return output(nil, command, nil, err)
	}
	arguments := map[string]any{}
	if len(request.Params.Arguments) > 0 {
		if err := json.Unmarshal(request.Params.Arguments, &arguments); err != nil {
			return output(app, command, nil, fmt.Errorf("decode delegated tool input: %w", err))
		}
	}
	delete(arguments, "workspace")
	progressToken := request.Params.GetProgressToken()
	lease, err := resolver.acquireWorkspaceMCP(ctx, app, request.Session, progressToken)
	if err != nil {
		return output(app, command, nil, err)
	}
	defer lease.Release()
	callParams := &mcp.CallToolParams{Name: request.Params.Name, Arguments: arguments}
	if lease.progressToken != "" {
		callParams.SetProgressToken(lease.progressToken)
	}
	result, err := lease.child.session.CallTool(lease.callContext, callParams)
	if err != nil {
		if ctx.Err() == nil {
			resolver.invalidateWorkspaceMCP(lease.child)
		}
		return output(app, command, nil, fmt.Errorf("workspace MCP tool %s: %w", request.Params.Name, err))
	}
	envelope, err := delegatedEnvelope(result.StructuredContent, command, app.WorkspaceID(), result.IsError)
	if err != nil {
		resolver.invalidateWorkspaceMCP(lease.child)
		return output(app, command, nil, err)
	}
	return result, envelopeOutput{Envelope: envelope}, nil
}

func (resolver *centralAppResolver) DelegateResource(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	app, err := resolver.Resolve(ctx, request.Session, "")
	if err != nil {
		return nil, err
	}
	lease, err := resolver.acquireWorkspaceMCP(ctx, app, request.Session, nil)
	if err != nil {
		return nil, err
	}
	defer lease.Release()
	result, err := lease.child.session.ReadResource(lease.callContext, &mcp.ReadResourceParams{URI: request.Params.URI})
	if err != nil && ctx.Err() == nil {
		resolver.invalidateWorkspaceMCP(lease.child)
	}
	return result, err
}

func connectWorkspaceMCP(connectContext, lifetimeContext context.Context, app *devtool.App, options *mcp.ClientOptions) (*mcp.ClientSession, error) {
	launcher := filepath.Join(app.Root(), "hk")
	info, err := os.Lstat(launcher)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace hk launcher: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return nil, fmt.Errorf("workspace hk launcher must be a regular executable file: %s", launcher)
	}
	command := exec.CommandContext(lifetimeContext, launcher, "--workspace", app.Root(), "mcp", "serve") //nolint:gosec // launcher is confined to the selected HitKeep root
	client := mcp.NewClient(&mcp.Implementation{Name: "hitkeep-developer-broker", Version: "1"}, options)
	session, err := client.Connect(connectContext, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		return nil, fmt.Errorf("connect workspace MCP: %w", err)
	}
	if options != nil && options.LoggingMessageHandler != nil {
		if err := session.SetLoggingLevel(connectContext, &mcp.SetLoggingLevelParams{Level: mcp.LoggingLevel("debug")}); err != nil {
			_ = session.Close()
			return nil, fmt.Errorf("enable workspace MCP logging: %w", err)
		}
	}
	return session, nil
}

func delegatedEnvelope(value any, command, workspaceID string, isError bool) (devtool.Envelope, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return devtool.Envelope{}, fmt.Errorf("encode workspace MCP result: %w", err)
	}
	var envelope devtool.Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return devtool.Envelope{}, fmt.Errorf("decode workspace MCP result: %w", err)
	}
	if envelope.SchemaVersion != devtool.SchemaVersion {
		return devtool.Envelope{}, fmt.Errorf("workspace MCP returned schema %q, want %q", envelope.SchemaVersion, devtool.SchemaVersion)
	}
	if envelope.Command != command {
		return devtool.Envelope{}, fmt.Errorf("workspace MCP returned command %q, want %q", envelope.Command, command)
	}
	if envelope.WorkspaceID != workspaceID {
		return devtool.Envelope{}, fmt.Errorf("workspace MCP returned workspace %q, want %q", envelope.WorkspaceID, workspaceID)
	}
	wantStatus := "ok"
	if isError {
		wantStatus = "error"
	}
	if envelope.Status != wantStatus || isError && envelope.Error == "" {
		return devtool.Envelope{}, errors.New("workspace MCP returned an inconsistent structured envelope")
	}
	return envelope, nil
}

func appsFromClientRoots(ctx context.Context, session *mcp.ServerSession, loadApp func(string) (*devtool.App, error)) ([]rootedApp, error) {
	if session == nil {
		return nil, errors.New("MCP session is unavailable")
	}
	result, err := session.ListRoots(ctx, nil)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	apps := make([]rootedApp, 0, len(result.Roots))
	for _, root := range result.Roots {
		path, pathErr := clientRootPath(root.URI)
		if pathErr != nil {
			continue
		}
		app, appErr := loadApp(path)
		if appErr != nil || seen[app.WorkspaceID()] {
			continue
		}
		seen[app.WorkspaceID()] = true
		apps = append(apps, rootedApp{app: app, name: strings.TrimSpace(root.Name)})
	}
	return apps, nil
}

func newHitKeepApp(path string) (*devtool.App, error) {
	app, err := devtool.NewApp(path)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(app.Root(), "go.mod"))
	if err != nil {
		return nil, errors.New("workspace has no HitKeep module")
	}
	for line := range strings.Lines(string(raw)) {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) == 2 && fields[0] == "module" && fields[1] == "hitkeep" {
			return app, nil
		}
		if fields[0] == "module" {
			break
		}
	}
	return nil, errors.New("workspace is not the HitKeep module")
}

func clientRootPath(rawURI string) (string, error) {
	parsed, err := url.Parse(rawURI)
	if err != nil || parsed.Scheme != "file" || parsed.Path == "" || parsed.Host != "" && parsed.Host != "localhost" {
		return "", errors.New("MCP root is not a local file URI")
	}
	return filepath.FromSlash(parsed.Path), nil
}

func NewServer(app *devtool.App, version string) *mcp.Server {
	return newServer(staticAppResolver{app: app}, version)
}

// NewCentralServer returns one root-aware MCP server that delegates every
// operation to the worktree selected by the connected client's MCP roots.
func NewCentralServer(fallbackWorkspace, version string) *mcp.Server {
	return newServer(newCentralAppResolver(fallbackWorkspace, nil), version)
}

func newServer(resolver appResolver, version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "hitkeep-developer", Version: version}, &mcp.ServerOptions{
		Instructions: "Local, root-routed and worktree-confined HitKeep development operations. Development is one container-only session per workspace and uses status plus event cursors, never run IDs. Dev start, stop, and follow tools stream progress and structured component logs. Screenshot capture is a bounded synchronous visual-QA operation; finite setup, QA, build, and smoke operations use run IDs. Pass workspace only when the client exposes multiple HitKeep roots. Mutable workflow facts come from hk catalogs; no arbitrary command execution is available.",
		Capabilities: &mcp.ServerCapabilities{Logging: &mcp.LoggingCapabilities{}},
	})
	registerTools(server, resolver)
	registerResources(server, resolver)
	return server
}

func registerTools(server *mcp.Server, resolver appResolver) {
	readOnly := annotations(true, true, false, false)
	action := annotations(false, false, false, false)
	idempotentAction := annotations(false, true, false, false)
	destructiveAction := annotations(false, true, false, true)

	mcp.AddTool(server, tool("hk_workspace_status", "Inspect the selected Git worktree, allocated ports, URLs, and change count.", readOnly), routedHandler(resolver, "workspace status", func(ctx context.Context, app *devtool.App, _ emptyInput) (any, error) {
		return app.Workspace(ctx)
	}))
	mcp.AddTool(server, tool("hk_workspace_list", "List HitKeep worktrees with their isolated workspace identifiers, ports, and state.", readOnly), routedHandler(resolver, "workspace list", func(ctx context.Context, app *devtool.App, _ emptyInput) (any, error) {
		return app.Workspaces(ctx)
	}))
	mcp.AddTool(server, tool("hk_workspace_handoff", "Return compact, secret-free handoff context for the selected worktree.", readOnly), routedHandler(resolver, "workspace handoff", func(ctx context.Context, app *devtool.App, _ emptyInput) (any, error) {
		return app.Handoff(ctx)
	}))
	mcp.AddTool(server, tool("hk_doctor", "Check the local toolchain and container runtime prerequisites.", readOnly), routedHandler(resolver, "doctor", func(ctx context.Context, app *devtool.App, _ emptyInput) (any, error) {
		return app.Doctor(ctx), nil
	}))
	mcp.AddTool(server, tool("hk_qa_plan", "Select canonical QA gates without running them.", readOnly), routedHandler(resolver, "qa plan", func(ctx context.Context, app *devtool.App, input qaPlanInput) (any, error) {
		if input.Profile == "" {
			input.Profile = "changed"
		}
		return app.QAPlan(ctx, input.Profile, input.BaseRef)
	}))
	mcp.AddTool(server, tool("hk_run_status", "Inspect one asynchronous hk run.", readOnly), routedHandler(resolver, "run status", func(_ context.Context, app *devtool.App, input runInput) (any, error) {
		return app.GetRun(input.RunID)
	}))
	mcp.AddTool(server, tool("hk_run_list", "List bounded recent runs for the selected worktree.", readOnly), routedHandler(resolver, "run list", func(_ context.Context, app *devtool.App, input runListInput) (any, error) {
		if input.Limit == 0 {
			input.Limit = 20
		}
		if input.Limit < 1 || input.Limit > 100 {
			return nil, errors.New("limit must be between 1 and 100")
		}
		return app.RecentRuns(input.Limit)
	}))
	mcp.AddTool(server, tool("hk_logs_tail", "Read a bounded, redacted tail of one hk run log.", readOnly), routedHandler(resolver, "logs tail", func(_ context.Context, app *devtool.App, input logsInput) (any, error) {
		if input.Limit < 0 || input.Limit > maxRequestedLogLines || input.Cursor < 0 {
			return nil, errors.New("limit must be between 1 and 200")
		}
		if input.GateID != "" {
			return app.TailGateLogAfter(input.RunID, input.GateID, input.Limit, input.Cursor)
		}
		return app.TailLogAfter(input.RunID, input.Limit, input.Cursor)
	}))

	mcp.AddTool(server, tool("hk_setup_start", "Start reproducible dependency setup and return a run ID immediately.", action), startHandler(resolver, "setup start", func(emptyInput) devtool.RunRequest {
		return devtool.RunRequest{Kind: "setup"}
	}))
	mcp.AddTool(server, tool("hk_dev_start", "Start or reuse the selected workspace's development session and stream progress until ready.", idempotentAction), routedRequestHandler(resolver, "dev start", func(ctx context.Context, request *mcp.CallToolRequest, app *devtool.App, input devStartInput) (any, error) {
		return app.StartDevDetachedObserved(devtool.WithAgentOutput(ctx), devtool.DevRequest{Variant: input.Variant, Seed: input.Seed}, devEventNotifier(ctx, request))
	}))
	mcp.AddTool(server, tool("hk_dev_status", "Inspect the selected workspace's development session.", readOnly), routedHandler(resolver, "dev status", func(ctx context.Context, app *devtool.App, _ devInput) (any, error) {
		return app.DevStatus(ctx)
	}))
	mcp.AddTool(server, tool("hk_dev_logs", "Read or follow cursor-addressed development events without changing the session.", readOnly), routedRequestHandler(resolver, "dev logs", func(ctx context.Context, request *mcp.CallToolRequest, app *devtool.App, input devLogsInput) (any, error) {
		if input.Cursor < 0 || input.Limit < 0 || input.Limit > maxRequestedLogLines {
			return nil, errors.New("cursor must be non-negative and limit must be between 1 and 200")
		}
		notify := devEventNotifier(ctx, request)
		if input.Follow {
			return app.FollowDevEvents(ctx, input.Cursor, input.Limit, notify)
		}
		batch, err := app.DevLogs(input.Cursor, input.Limit)
		if err != nil {
			return batch, err
		}
		for _, event := range batch.Events {
			notify(event)
		}
		return batch, nil
	}))
	mcp.AddTool(server, tool("hk_dev_stop", "Stop only the selected workspace's development session and stream shutdown progress.", destructiveAction), routedRequestHandler(resolver, "dev stop", func(ctx context.Context, request *mcp.CallToolRequest, app *devtool.App, _ devInput) (any, error) {
		before, _ := app.DevStatus(context.Background())
		cursor := before.NextEventCursor
		type stopResult struct {
			status devtool.DevStatus
			err    error
		}
		done := make(chan stopResult, 1)
		stopContext := context.WithoutCancel(ctx)
		go func() {
			status, err := app.StopDev(stopContext)
			done <- stopResult{status: status, err: err}
		}()
		notify := devEventNotifier(ctx, request)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case result := <-done:
				batch, _ := app.DevLogs(cursor, maxRequestedLogLines)
				for _, event := range batch.Events {
					notify(event)
				}
				return result.status, result.err
			case <-ctx.Done():
				return app.DevStatus(context.Background())
			case <-ticker.C:
				batch, batchErr := app.DevLogs(cursor, maxRequestedLogLines)
				if batchErr != nil {
					continue
				}
				for _, event := range batch.Events {
					notify(event)
				}
				cursor = batch.NextCursor
			}
		}
	}))
	mcp.AddTool(server, tool("hk_screenshot", "Capture up to eight local dashboard routes in one browser session and return managed PNG resource links.", action), screenshotHandler(resolver))
	mcp.AddTool(server, tool("hk_qa_start", "Start canonical QA gates asynchronously and return a run ID.", action), startHandler(resolver, "qa start", func(input qaStartInput) devtool.RunRequest {
		return devtool.RunRequest{Kind: "qa", Profile: input.Profile, GateIDs: input.GateIDs}
	}))
	mcp.AddTool(server, tool("hk_build_start", "Start a deterministic binary or local image build and return a run ID.", action), startHandler(resolver, "build start", func(input buildInput) devtool.RunRequest {
		return devtool.RunRequest{Kind: "build", Variant: input.Variant, Target: input.Target}
	}))
	mcp.AddTool(server, tool("hk_smoke_start", "Start a local production-image smoke test and return a run ID.", action), startHandler(resolver, "smoke start", func(input variantInput) devtool.RunRequest {
		return devtool.RunRequest{Kind: "smoke", Variant: input.Variant}
	}))
	mcp.AddTool(server, tool("hk_run_cancel", "Request cancellation of one active hk run.", destructiveAction), routedHandler(resolver, "run cancel", func(_ context.Context, app *devtool.App, input runInput) (any, error) {
		return app.CancelRun(input.RunID)
	}))
}

func routedHandler[In workspaceScoped](resolver appResolver, command string, handler func(context.Context, *devtool.App, In) (any, error)) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, envelopeOutput, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, envelopeOutput, error) {
		if delegate, ok := resolver.(toolDelegatingResolver); ok {
			return delegate.DelegateTool(ctx, request, command, input)
		}
		app, err := resolver.Resolve(ctx, request.Session, input.workspaceSelector())
		if err != nil {
			return output(nil, command, nil, err)
		}
		value, err := handler(ctx, app, input)
		return output(app, command, value, err)
	}
}

func routedRequestHandler[In workspaceScoped](resolver appResolver, command string, handler func(context.Context, *mcp.CallToolRequest, *devtool.App, In) (any, error)) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, envelopeOutput, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, envelopeOutput, error) {
		if delegate, ok := resolver.(toolDelegatingResolver); ok {
			return delegate.DelegateTool(ctx, request, command, input)
		}
		app, err := resolver.Resolve(ctx, request.Session, input.workspaceSelector())
		if err != nil {
			return output(nil, command, nil, err)
		}
		value, err := handler(ctx, request, app, input)
		return output(app, command, value, err)
	}
}

func screenshotHandler(resolver appResolver) func(context.Context, *mcp.CallToolRequest, screenshotInput) (*mcp.CallToolResult, envelopeOutput, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, input screenshotInput) (*mcp.CallToolResult, envelopeOutput, error) {
		if delegate, ok := resolver.(toolDelegatingResolver); ok {
			return delegate.DelegateTool(ctx, request, "screenshot", input)
		}
		app, err := resolver.Resolve(ctx, request.Session, input.workspaceSelector())
		if err != nil {
			return output(nil, "screenshot", nil, err)
		}
		result, captureErr := app.CaptureScreenshots(ctx, devtool.ScreenshotRequest{
			Routes: input.Routes, Viewport: input.Viewport, Theme: input.Theme, Scale: input.Scale,
			WaitMS: input.WaitMS, FullPage: input.FullPage, Selector: input.Selector, Anonymous: input.Anonymous,
		})
		toolResult, envelope, _ := output(app, "screenshot", result, captureErr)
		if captureErr != nil {
			return toolResult, envelope, nil
		}
		appendScreenshotResourceLinks(toolResult, result)
		return toolResult, envelope, nil
	}
}

func appendScreenshotResourceLinks(toolResult *mcp.CallToolResult, result devtool.ScreenshotResult) {
	for _, artifact := range result.Artifacts {
		size := artifact.Bytes
		toolResult.Content = append(toolResult.Content, &mcp.ResourceLink{
			URI:         localFileURI(artifact.Path),
			Name:        filepath.Base(artifact.Path),
			Title:       "HitKeep " + artifact.Route,
			Description: fmt.Sprintf("%dx%d local visual-QA screenshot", artifact.Width, artifact.Height),
			MIMEType:    artifact.MIMEType,
			Size:        &size,
		})
	}
}

func localFileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func devEventNotifier(ctx context.Context, request *mcp.CallToolRequest) func(devtool.DevEvent) {
	return func(event devtool.DevEvent) {
		if request.Session == nil {
			return
		}
		if progressToken := request.Params.GetProgressToken(); progressToken != nil {
			_ = request.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
				ProgressToken: progressToken,
				Progress:      float64(event.Cursor),
				Message:       event.Message,
			})
		}
		if event.Type != "log" {
			return
		}
		level := mcp.LoggingLevel("info")
		switch event.Level {
		case "debug", "error", "critical", "alert", "emergency", "notice":
			level = mcp.LoggingLevel(event.Level)
		case "warn", "warning":
			level = "warning"
		}
		logger := event.Component
		if logger == "" {
			logger = "supervisor"
		}
		_ = request.Session.Log(ctx, &mcp.LoggingMessageParams{
			Level: level, Logger: "hitkeep.dev." + logger, Data: event,
		})
	}
}

func startHandler[In workspaceScoped](resolver appResolver, command string, request func(In) devtool.RunRequest) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, envelopeOutput, error) {
	return routedHandler(resolver, command, func(ctx context.Context, app *devtool.App, input In) (any, error) {
		return app.StartRun(devtool.WithAgentOutput(ctx), request(input))
	})
}

func output(app *devtool.App, command string, data any, err error) (*mcp.CallToolResult, envelopeOutput, error) {
	workspaceID := ""
	if app != nil {
		workspaceID = app.WorkspaceID()
	}
	var envelope devtool.Envelope
	if err != nil {
		envelope = devtool.ErrorEnvelope(command, workspaceID, err)
	} else {
		envelope = devtool.SuccessEnvelope(command, workspaceID, data)
	}
	return &mcp.CallToolResult{IsError: err != nil}, envelopeOutput{Envelope: envelope}, nil
}

func tool(name, description string, annotation *mcp.ToolAnnotations) *mcp.Tool {
	return &mcp.Tool{Name: name, Description: description, Annotations: annotation, InputSchema: inputSchema(name), OutputSchema: envelopeOutputSchema()}
}

// envelopeOutputSchema returns the inferred envelope schema with the
// `any`-typed data field forced into object form. jsonschema-go marshals the
// empty schema it infers for `any` as the boolean form `true`, which some MCP
// clients (Claude Code) reject when validating outputSchema properties. A
// description-only schema stays permissive but serializes as an object.
var envelopeOutputSchema = sync.OnceValue(func() *jsonschema.Schema {
	schema, err := jsonschema.For[envelopeOutput](nil)
	if err != nil {
		panic(fmt.Errorf("devmcp: infer envelope output schema: %w", err))
	}
	schema.Properties["data"] = &jsonschema.Schema{Description: "Command-specific payload."}
	return schema
})

func inputSchema(name string) *jsonschema.Schema {
	properties := map[string]*jsonschema.Schema{
		"workspace": {Type: "string", MaxLength: new(4096), Description: "Optional client-root name, workspace ID, or workspace path; required only for ambiguous multi-root sessions"},
	}
	required := []string{}
	switch name {
	case "hk_qa_plan":
		properties["profile"] = enum("changed", "pr", "full")
		properties["base_ref"] = &jsonschema.Schema{Type: "string", MaxLength: new(200)}
	case "hk_run_status", "hk_run_cancel":
		properties["run_id"] = runIDSchema()
		required = []string{"run_id"}
	case "hk_logs_tail":
		properties["run_id"] = runIDSchema()
		properties["limit"] = &jsonschema.Schema{Type: "integer", Minimum: new(1.0), Maximum: new(float64(maxRequestedLogLines))}
		properties["cursor"] = &jsonschema.Schema{Type: "integer", Minimum: new(0.0)}
		properties["gate_id"] = gateEnum()
		required = []string{"run_id"}
	case "hk_run_list":
		properties["limit"] = &jsonschema.Schema{Type: "integer", Minimum: new(1.0), Maximum: new(100.0)}
	case "hk_dev_start":
		properties["variant"] = enum("self-hosted", "cloud")
		properties["seed"] = &jsonschema.Schema{Type: "boolean"}
	case "hk_dev_logs":
		properties["cursor"] = &jsonschema.Schema{Type: "integer", Minimum: new(0.0)}
		properties["limit"] = &jsonschema.Schema{Type: "integer", Minimum: new(1.0), Maximum: new(float64(maxRequestedLogLines))}
		properties["follow"] = &jsonschema.Schema{Type: "boolean"}
	case "hk_screenshot":
		properties["routes"] = &jsonschema.Schema{
			Type: "array", MinItems: new(1), MaxItems: new(devtool.MaxScreenshotRoutes), UniqueItems: true,
			Items: &jsonschema.Schema{Type: "string", MinLength: new(1), MaxLength: new(2048), Pattern: `^/(?:[^/]|$)`},
		}
		properties["viewport"] = enum("desktop", "mobile")
		properties["theme"] = enum("light", "dark")
		properties["scale"] = &jsonschema.Schema{Type: "integer", Enum: []any{1, 2}}
		properties["wait_ms"] = &jsonschema.Schema{Type: "integer", Minimum: new(0.0), Maximum: new(5000.0)}
		properties["full_page"] = &jsonschema.Schema{Type: "boolean"}
		properties["selector"] = &jsonschema.Schema{Type: "string", MaxLength: new(500)}
		properties["anonymous"] = &jsonschema.Schema{Type: "boolean"}
	case "hk_smoke_start":
		properties["variant"] = enum("self-hosted", "cloud")
	case "hk_qa_start":
		properties["profile"] = enum("changed", "pr", "full")
		properties["gate_ids"] = &jsonschema.Schema{Type: "array", Items: gateEnum(), UniqueItems: true, MaxItems: new(len(devtool.CatalogSnapshot().Gates))}
	case "hk_build_start":
		properties["variant"] = enum("self-hosted", "cloud")
		properties["target"] = enum("binary", "image")
	}
	return &jsonschema.Schema{Type: "object", Properties: properties, Required: required, AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}}}
}

func enum(values ...string) *jsonschema.Schema {
	items := make([]any, len(values))
	for index, value := range values {
		items[index] = value
	}
	return &jsonschema.Schema{Type: "string", Enum: items}
}

func gateEnum() *jsonschema.Schema {
	gates := devtool.CatalogSnapshot().Gates
	values := make([]string, len(gates))
	for index, gate := range gates {
		values[index] = gate.ID
	}
	return enum(values...)
}

func runIDSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Pattern: `^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`, MaxLength: new(100)}
}

func annotations(readOnly, idempotent, openWorld, destructive bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: readOnly, IdempotentHint: idempotent, OpenWorldHint: &openWorld, DestructiveHint: &destructive}
}

func registerResources(server *mcp.Server, resolver appResolver) {
	server.AddResource(resource("Current workspace", "hitkeep-dev://workspace/current", "Current secret-free worktree state."), resourceHandler(resolver))
	server.AddResource(resource("Build variants", "hitkeep-dev://catalog/variants", "Canonical self-hosted and cloud variant catalog."), resourceHandler(resolver))
	server.AddResource(resource("QA catalog", "hitkeep-dev://catalog/qa", "Canonical QA profiles and gate catalog."), resourceHandler(resolver))
	server.AddResource(resource("Contributing guide", "hitkeep-dev://docs/contributing", "Repository contributor onboarding guidance."), resourceHandler(resolver))
	server.AddResourceTemplate(&mcp.ResourceTemplate{Name: "Run summary", Description: "One asynchronous hk run summary.", MIMEType: "application/json", URITemplate: "hitkeep-dev://runs/{run_id}/summary"}, resourceHandler(resolver))
	server.AddResourceTemplate(&mcp.ResourceTemplate{Name: "Run logs", Description: "Bounded and redacted run log tail.", MIMEType: "application/json", URITemplate: "hitkeep-dev://runs/{run_id}/logs/{gate}"}, resourceHandler(resolver))
}

func resource(name, uri, description string) *mcp.Resource {
	return &mcp.Resource{Name: name, URI: uri, Description: description, MIMEType: "application/json"}
}

func resourceHandler(resolver appResolver) mcp.ResourceHandler {
	return func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		if delegate, ok := resolver.(resourceDelegatingResolver); ok {
			return delegate.DelegateResource(ctx, request)
		}
		app, err := resolver.Resolve(ctx, request.Session, "")
		if err != nil {
			return nil, err
		}
		uri := request.Params.URI
		var value any
		var readErr error
		switch uri {
		case "hitkeep-dev://workspace/current":
			value, readErr = app.Workspace(ctx)
		case "hitkeep-dev://catalog/variants":
			value = map[string]any{"schema_version": devtool.SchemaVersion, "variants": app.Catalog().Variants}
		case "hitkeep-dev://catalog/qa":
			catalog := app.Catalog()
			value = map[string]any{"schema_version": devtool.SchemaVersion, "profiles": catalog.Profiles, "gates": catalog.Gates}
		case "hitkeep-dev://docs/contributing":
			value, readErr = contributingResource(app.Root())
		default:
			value, readErr = runResource(app, uri)
		}
		if readErr != nil {
			return nil, readErr
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "application/json", Text: string(raw)}}}, nil
	}
}

func contributingResource(root string) (map[string]any, error) {
	raw, err := os.ReadFile(filepath.Join(root, "CONTRIBUTING.md"))
	if err != nil {
		return nil, err
	}
	const maxBytes = 64 * 1024
	truncated := len(raw) > maxBytes
	if truncated {
		raw = raw[:maxBytes]
	}
	return map[string]any{"schema_version": devtool.SchemaVersion, "markdown": string(raw), "truncated": truncated}, nil
}

func runResource(app *devtool.App, rawURI string) (any, error) {
	parsed, err := url.Parse(rawURI)
	if err != nil || parsed.Scheme != "hitkeep-dev" || parsed.Host != "runs" {
		return nil, errors.New("unknown developer resource")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) == 2 && parts[1] == "summary" {
		return app.GetRun(parts[0])
	}
	if len(parts) == 3 && parts[1] == "logs" {
		if parts[2] != "all" {
			return app.TailGateLog(parts[0], parts[2], 80)
		}
		return app.TailLog(parts[0], 80)
	}
	return nil, fmt.Errorf("unknown run resource %q", rawURI)
}

func RunStdio(ctx context.Context, app *devtool.App, version string) error {
	return NewServer(app, version).Run(ctx, &mcp.StdioTransport{})
}

func RunCentralStdio(ctx context.Context, fallbackWorkspace, version string) error {
	resolver := newCentralAppResolver(fallbackWorkspace, nil)
	defer resolver.Close()
	return newServer(resolver, version).Run(ctx, &mcp.StdioTransport{})
}

func ParseLimit(value string) (int, error) {
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > maxRequestedLogLines {
		return 0, errors.New("limit must be between 1 and 200")
	}
	return limit, nil
}
