package devmcp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"hitkeep/internal/devtool"
)

const workspaceCatalogTTL = 5 * time.Second

type workspaceCatalogSnapshot struct {
	loadedAt time.Time
	byID     map[string]*devtool.App
	byPath   map[string]*devtool.App
	fallback *devtool.App
}

type registryCounters struct {
	hits            atomic.Uint64
	misses          atomic.Uint64
	refreshes       atomic.Uint64
	refreshFailures atomic.Uint64
}

type workspaceRegistry struct {
	fallback     string
	ttl          time.Duration
	now          func() time.Time
	refresh      sync.Mutex
	snapshot     atomic.Pointer[workspaceCatalogSnapshot]
	routingStale atomic.Bool
	counters     registryCounters
}

type registryHealth struct {
	Hits            uint64 `json:"hits"`
	Misses          uint64 `json:"misses"`
	Refreshes       uint64 `json:"refreshes"`
	RefreshFailures uint64 `json:"refresh_failures"`
	CatalogAgeMS    int64  `json:"catalog_age_ms,omitempty"`
	WorkspaceCount  int    `json:"workspace_count"`
	RoutingStale    bool   `json:"routing_stale"`
}

func newWorkspaceRegistry(fallback string) *workspaceRegistry {
	return &workspaceRegistry{fallback: fallback, ttl: workspaceCatalogTTL, now: time.Now}
}

func (registry *workspaceRegistry) resolve(ctx context.Context, selector string, load func(string) (*devtool.App, error)) (*devtool.App, bool, error) {
	now := registry.now()
	if snapshot := registry.snapshot.Load(); snapshot != nil && now.Sub(snapshot.loadedAt) < registry.ttl {
		if app := catalogApp(snapshot, selector); app != nil {
			registry.counters.hits.Add(1)
			return app, false, nil
		}
	}
	registry.counters.misses.Add(1)
	if err := registry.refreshCatalog(ctx, load, false); err != nil {
		registry.counters.refreshFailures.Add(1)
		registry.routingStale.Store(true)
		if snapshot := registry.snapshot.Load(); snapshot != nil {
			if app := catalogApp(snapshot, selector); app != nil {
				return app, true, nil
			}
		}
		return nil, true, fmt.Errorf("refresh workspace catalog: %w", err)
	}
	if app := catalogApp(registry.snapshot.Load(), selector); app != nil {
		return app, false, nil
	}
	return nil, false, fmt.Errorf("workspace %q is not one of the configured fallback clone's HitKeep workspaces", selector)
}

func (registry *workspaceRegistry) resolveFresh(ctx context.Context, selector string, load func(string) (*devtool.App, error)) (*devtool.App, error) {
	registry.counters.misses.Add(1)
	if err := registry.refreshCatalog(ctx, load, true); err != nil {
		registry.counters.refreshFailures.Add(1)
		registry.routingStale.Store(true)
		return nil, fmt.Errorf("refresh workspace catalog before starting work: %w", err)
	}
	if app := catalogApp(registry.snapshot.Load(), selector); app != nil {
		return app, nil
	}
	return nil, fmt.Errorf("workspace %q is not one of the refreshed fallback clone's HitKeep workspaces", selector)
}

func (registry *workspaceRegistry) refreshCatalog(ctx context.Context, load func(string) (*devtool.App, error), force bool) error {
	registry.refresh.Lock()
	defer registry.refresh.Unlock()

	now := registry.now()
	if snapshot := registry.snapshot.Load(); !force && snapshot != nil && now.Sub(snapshot.loadedAt) < registry.ttl {
		return nil
	}
	if registry.fallback == "" {
		return errors.New("no configured fallback HitKeep workspace is available")
	}
	fallback, err := load(registry.fallback)
	if err != nil {
		return fmt.Errorf("resolve fallback workspace: %w", err)
	}
	base, err := fallback.Workspace(ctx)
	if err != nil {
		return fmt.Errorf("inspect fallback workspace: %w", err)
	}
	workspaces, err := fallback.Workspaces(ctx)
	if err != nil {
		return fmt.Errorf("list HitKeep workspaces: %w", err)
	}
	workspaces = append(workspaces, base)
	snapshot := &workspaceCatalogSnapshot{
		loadedAt: now,
		byID:     make(map[string]*devtool.App, len(workspaces)),
		byPath:   make(map[string]*devtool.App, len(workspaces)),
	}
	for _, workspace := range workspaces {
		if workspace.ID == "" || workspace.GitCommonDir != base.GitCommonDir {
			continue
		}
		cleanRoot, cleanErr := filepath.Abs(filepath.Clean(workspace.Root))
		if cleanErr != nil {
			continue
		}
		app := fallback
		if !samePath(cleanRoot, fallback.Root()) {
			app, cleanErr = load(cleanRoot)
			if cleanErr != nil {
				continue
			}
		}
		snapshot.byID[app.WorkspaceID()] = app
		snapshot.byPath[cleanRoot] = app
		if samePath(cleanRoot, fallback.Root()) {
			snapshot.fallback = app
		}
	}
	if snapshot.fallback == nil {
		return errors.New("configured fallback disappeared during workspace discovery")
	}
	registry.snapshot.Store(snapshot)
	registry.routingStale.Store(false)
	registry.counters.refreshes.Add(1)
	return nil
}

func catalogApp(snapshot *workspaceCatalogSnapshot, selector string) *devtool.App {
	if snapshot == nil {
		return nil
	}
	if selector == "" {
		return snapshot.fallback
	}
	if app := snapshot.byID[selector]; app != nil {
		return app
	}
	clean, err := filepath.Abs(filepath.Clean(selector))
	if err != nil {
		return nil
	}
	return snapshot.byPath[clean]
}

func (registry *workspaceRegistry) health() registryHealth {
	health := registryHealth{
		Hits:            registry.counters.hits.Load(),
		Misses:          registry.counters.misses.Load(),
		Refreshes:       registry.counters.refreshes.Load(),
		RefreshFailures: registry.counters.refreshFailures.Load(),
	}
	if snapshot := registry.snapshot.Load(); snapshot != nil {
		health.CatalogAgeMS = registry.now().Sub(snapshot.loadedAt).Milliseconds()
		health.WorkspaceCount = len(snapshot.byID)
		health.RoutingStale = registry.routingStale.Load() || health.CatalogAgeMS > registry.ttl.Milliseconds()
	}
	return health
}

type contextInput struct {
	workspaceInput
	View string `json:"view,omitempty" jsonschema:"Bounded view: current, workspaces, catalog, configuration, handoff, or runtime"`
}

type runStartInput struct {
	workspaceInput
	Kind    string   `json:"kind" jsonschema:"Finite run kind: setup, qa, build, or smoke"`
	PlanID  string   `json:"plan_id,omitempty" jsonschema:"Required immutable plan identifier for QA"`
	Profile string   `json:"profile,omitempty" jsonschema:"QA profile: changed, complete, pr, or full"`
	GateIDs []string `json:"gate_ids,omitempty" jsonschema:"Optional explicit canonical gate identifiers"`
	Variant string   `json:"variant,omitempty" jsonschema:"Build variant"`
	Target  string   `json:"target,omitempty" jsonschema:"Build target"`
}

type devStatusInput struct {
	workspaceInput
	Cursor int64 `json:"cursor,omitempty" jsonschema:"Next event cursor"`
	Limit  int   `json:"limit,omitempty" jsonschema:"Maximum events, default 50 and maximum 200"`
}

type devStopInput struct {
	workspaceInput
	GenerationID string `json:"generation_id" jsonschema:"Exact observed development generation identifier"`
}

type runStatusInput struct {
	workspaceInput
	RunID  string `json:"run_id" jsonschema:"Validated hk run identifier"`
	GateID string `json:"gate_id,omitempty" jsonschema:"Optional gate identifier for bounded logs"`
	Cursor int    `json:"cursor,omitempty" jsonschema:"Next log cursor"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum log entries, default 50 and maximum 200"`
}
