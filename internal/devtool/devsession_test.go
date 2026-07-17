package devtool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDevSessionEventsAreCursorAddressedBoundedAndRedacted(t *testing.T) {
	app := newDevTestApp(t)
	now := time.Now().UTC()
	record := devSessionRecord{DevStatus: DevStatus{
		State: DevStateStarting, GenerationID: "generation-events", Variant: "self-hosted",
		Owner: DevOwnerForeground, StartedAt: &now, UpdatedAt: now, URLs: app.workspace.URLs,
	}}
	if err := app.writeDevRecord(record); err != nil {
		t.Fatal(err)
	}
	sink, err := app.openDevEventSink(record, nil)
	if err != nil {
		t.Fatal(err)
	}
	for index := range 250 {
		message := "line"
		if index == 249 {
			message = "TOKEN=secret"
		}
		sink.Event("log", "backend", "info", "", message)
	}
	sink.Close()

	batch, err := app.DevLogs(0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Events) != maxDevEvents || !batch.Truncated || batch.NextCursor != 250 {
		t.Fatalf("unexpected bounded events: %+v", batch)
	}
	last := batch.Events[len(batch.Events)-1]
	if last.Cursor != 249 || strings.Contains(last.Message, "secret") || !strings.Contains(last.Message, "[redacted]") {
		t.Fatalf("event redaction/cursor mismatch: %+v", last)
	}
}

func TestDevLogsReadsBoundedTailAndResetsStaleCursor(t *testing.T) {
	app := newDevTestApp(t)
	now := time.Now().UTC()
	record := devSessionRecord{DevStatus: DevStatus{
		State: DevStateStopped, GenerationID: "generation-large-tail", Variant: "self-hosted",
		Owner: DevOwnerDetached, StartedAt: &now, StoppedAt: &now, UpdatedAt: now, URLs: app.workspace.URLs,
	}}
	if err := app.writeDevRecord(record); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(app.devEventsDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	largePrefix := append(make([]byte, maxDevEventBytes+1), '\n')
	if err := os.WriteFile(app.devEventsPath(record.GenerationID), largePrefix, 0o600); err != nil {
		t.Fatal(err)
	}
	sink, err := app.openDevEventSink(record, nil)
	if err != nil {
		t.Fatal(err)
	}
	for index := range 250 {
		sink.Event("log", "backend", "info", "", fmt.Sprintf("event %d", index))
	}
	sink.Close()

	assertTail := func(cursor int64) {
		t.Helper()
		batch, readErr := app.DevLogs(cursor, 20)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(batch.Events) != 20 || !batch.Truncated || batch.Events[0].Cursor != 230 || batch.Events[19].Cursor != 249 || batch.NextCursor != 250 {
			t.Fatalf("unexpected bounded tail for cursor %d: %+v", cursor, batch)
		}
	}
	assertTail(0)
	assertTail(999)
}

func TestDevSessionStartIsIdempotentAndRejectsConflicts(t *testing.T) {
	app := newDevTestApp(t)
	now := time.Now().UTC()
	record := devSessionRecord{
		DevStatus: DevStatus{
			State: DevStateReady, GenerationID: "generation-active", Variant: "self-hosted",
			Owner: DevOwnerDetached, StartedAt: &now, ReadyAt: &now, UpdatedAt: now, URLs: app.workspace.URLs,
		},
		SupervisorPID: os.Getpid(),
	}
	if err := app.writeDevRecord(record); err != nil {
		t.Fatal(err)
	}

	reused, err := app.StartDevDetached(context.Background(), DevRequest{Variant: "self-hosted"})
	if err != nil {
		t.Fatal(err)
	}
	if !reused.Reused || reused.Status.GenerationID != record.GenerationID {
		t.Fatalf("matching session was not reused: %+v", reused)
	}
	if _, err := app.StartDevDetached(context.Background(), DevRequest{Variant: "self-hosted", Seed: true}); err == nil || !strings.Contains(err.Error(), "reset --seed") {
		t.Fatalf("active session accepted seed mutation: %v", err)
	}
	if _, err := app.StartDevDetached(context.Background(), DevRequest{Variant: "cloud"}); err == nil || !strings.Contains(err.Error(), "dev stop") {
		t.Fatalf("active session accepted conflicting configuration: %v", err)
	}
}

func TestDevStatusReconcilesExitedSupervisor(t *testing.T) {
	app := newDevTestApp(t)
	started := time.Now().Add(-time.Minute).UTC()
	record := devSessionRecord{
		DevStatus: DevStatus{
			State: DevStateReady, GenerationID: "generation-exited", Variant: "self-hosted",
			Owner: DevOwnerDetached, StartedAt: &started, UpdatedAt: started, URLs: app.workspace.URLs,
		},
		SupervisorPID: 1 << 30,
	}
	if err := app.writeDevRecord(record); err != nil {
		t.Fatal(err)
	}
	status, err := app.DevStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.State != DevStateFailed || status.Error == "" || status.StoppedAt == nil {
		t.Fatalf("exited supervisor was not reconciled: %+v", status)
	}
}

func TestForegroundDevStopsEverythingOnCancellation(t *testing.T) {
	app := newDevTestApp(t)
	installFakeDevCommands(t, false)
	app.devProbe = func(context.Context) []DevService { return devTestServices(true) }
	app.devProbeInterval = 10 * time.Millisecond
	app.devGracePeriod = 500 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	var phases []string
	var mu sync.Mutex
	result, err := app.StartDevForeground(ctx, DevRequest{Variant: "self-hosted"}, func(event DevEvent) {
		if event.Phase == "ready" {
			cancel()
		}
		if event.Phase != "" {
			mu.Lock()
			phases = append(phases, event.Phase)
			mu.Unlock()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("foreground cancellation error = %v", err)
	}
	if result.Status.State != DevStateStopped {
		t.Fatalf("foreground session did not stop: %+v", result.Status)
	}
	if strings.Join(phases, ",") != "starting,ready,stopping,stopped" {
		t.Fatalf("unexpected lifecycle phases: %v", phases)
	}
}

func TestDetachedDevWaitsUntilReadyAndStopsCompletely(t *testing.T) {
	app := newDevTestApp(t)
	installFakeDevCommands(t, false)
	var probes atomic.Int64
	app.devProbe = func(context.Context) []DevService {
		return devTestServices(probes.Add(1) > 1)
	}
	app.devProbeInterval = 10 * time.Millisecond
	app.devGracePeriod = 500 * time.Millisecond
	app.devDetachedStart = func(ctx context.Context, record devSessionRecord, request DevRequest) error {
		return app.superviseDev(ctx, record, request, nil)
	}
	startContext, cancelStart := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelStart()
	result, err := app.StartDevDetached(startContext, DevRequest{Variant: "self-hosted"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status.State != DevStateReady || result.Status.Owner != DevOwnerDetached || result.Status.ReadyAt == nil {
		t.Fatalf("detached start returned before readiness: %+v", result)
	}
	if len(result.RecentEvents) == 0 || result.NextCursor == 0 {
		t.Fatalf("detached start returned no fallback events: %+v", result)
	}
	stopContext, cancelStop := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelStop()
	stopped, err := app.StopDev(stopContext)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.State != DevStateStopped || stopped.StoppedAt == nil {
		t.Fatalf("detached stop was incomplete: %+v", stopped)
	}
}

func TestDevStartupTimeoutFailsAndCleansUp(t *testing.T) {
	app := newDevTestApp(t)
	installFakeDevCommands(t, false)
	app.devProbe = func(context.Context) []DevService { return devTestServices(false) }
	app.devReadyTimeout = 50 * time.Millisecond
	app.devProbeInterval = 10 * time.Millisecond
	app.devGracePeriod = 100 * time.Millisecond
	result, err := app.StartDevForeground(context.Background(), DevRequest{Variant: "self-hosted"}, nil)
	if err == nil || !strings.Contains(err.Error(), "did not become ready") {
		t.Fatalf("startup timeout error = %v", err)
	}
	if result.Status.State != DevStateFailed {
		t.Fatalf("startup timeout did not fail the session: %+v", result.Status)
	}
}

func TestDevChildCrashFailsTheSession(t *testing.T) {
	app := newDevTestApp(t)
	installFakeDevCommands(t, true)
	app.devProbe = func(context.Context) []DevService { return devTestServices(false) }
	app.devProbeInterval = 10 * time.Millisecond
	app.devGracePeriod = 100 * time.Millisecond
	result, err := app.StartDevForeground(context.Background(), DevRequest{Variant: "self-hosted"}, nil)
	if err == nil || !strings.Contains(err.Error(), "compose exited with code") {
		t.Fatalf("child crash error = %v", err)
	}
	if result.Status.State != DevStateFailed {
		t.Fatalf("child crash did not fail the session: %+v", result.Status)
	}
}

func TestDevDegradedSessionRecovers(t *testing.T) {
	app := newDevTestApp(t)
	installFakeDevCommands(t, false)
	var probes atomic.Int64
	app.devProbe = func(context.Context) []DevService {
		switch probes.Add(1) {
		case 1:
			return devTestServices(false)
		case 2:
			return devTestServices(true)
		case 3:
			return devTestServices(false)
		default:
			return devTestServices(true)
		}
	}
	app.devProbeInterval = 10 * time.Millisecond
	app.devGracePeriod = 500 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	var phases []string
	result, err := app.StartDevForeground(ctx, DevRequest{Variant: "self-hosted"}, func(event DevEvent) {
		if event.Phase != "" {
			phases = append(phases, event.Phase)
		}
		if event.Phase == "ready" && slicesCount(phases, "ready") == 2 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) || result.Status.State != DevStateStopped {
		t.Fatalf("recovery result=%+v err=%v", result, err)
	}
	if !strings.Contains(strings.Join(phases, ","), "ready,degraded,ready") {
		t.Fatalf("degraded recovery phases missing: %v", phases)
	}
}

func TestDevLogFollowCancellationDoesNotStopSession(t *testing.T) {
	app := newDevTestApp(t)
	now := time.Now().UTC()
	record := devSessionRecord{
		DevStatus: DevStatus{
			State: DevStateReady, GenerationID: "generation-follow", Variant: "self-hosted",
			Owner: DevOwnerDetached, StartedAt: &now, ReadyAt: &now, UpdatedAt: now, URLs: app.workspace.URLs,
		},
		SupervisorPID: os.Getpid(),
	}
	if err := app.writeDevRecord(record); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := app.FollowDevEvents(ctx, 0, 20, func(DevEvent) {}); !errors.Is(err, context.Canceled) {
		t.Fatalf("follow cancellation error = %v", err)
	}
	status, err := app.DevStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.State != DevStateReady {
		t.Fatalf("observer cancellation stopped development: %+v", status)
	}
}

func TestWorkspaceReportsDevSessionAndFiniteRun(t *testing.T) {
	app := newDevTestApp(t)
	now := time.Now().UTC()
	record := devSessionRecord{
		DevStatus: DevStatus{
			State: DevStateReady, GenerationID: "generation-workspace", Variant: "self-hosted",
			Owner: DevOwnerDetached, StartedAt: &now, ReadyAt: &now, UpdatedAt: now, URLs: app.workspace.URLs,
		},
		SupervisorPID: os.Getpid(),
	}
	if err := app.writeDevRecord(record); err != nil {
		t.Fatal(err)
	}
	run := Run{
		ID: "20260716T000000-finite", WorkspaceID: app.workspace.ID, Request: RunRequest{Kind: "setup"},
		Status: "running", PID: os.Getpid(), LogPath: filepath.Join(app.workspace.StateDir, "runs", "finite.log"), StartedAt: now,
	}
	if err := app.writeRun(run); err != nil {
		t.Fatal(err)
	}
	workspace, err := app.Workspace(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Dev == nil || workspace.Dev.GenerationID != record.GenerationID {
		t.Fatalf("workspace lost development status: %+v", workspace.Dev)
	}
	if len(workspace.ActiveRuns) != 1 || workspace.ActiveRuns[0].Request.Kind != "setup" {
		t.Fatalf("workspace mixed development into finite active runs: %+v", workspace.ActiveRuns)
	}
	recent, err := app.RecentRuns(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].ID != run.ID {
		t.Fatalf("run history lost the finite operation: %+v", recent)
	}
}

func newDevTestApp(t *testing.T) *App {
	t.Helper()
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func installFakeDevCommands(t *testing.T, crashLogs bool) {
	t.Helper()
	bin := t.TempDir()
	docker := "#!/bin/sh\n"
	if crashLogs {
		docker += "case \"$*\" in *\"logs --follow\"*) exit 7;; *) exit 0;; esac\n"
	} else {
		docker += "case \"$*\" in *\"logs --follow\"*) trap 'exit 0' TERM INT; while :; do sleep 1; done;; *) exit 0;; esac\n"
	}
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(docker), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func devTestServices(reachable bool) []DevService {
	now := time.Now().UTC()
	return []DevService{
		{Name: "backend", Address: "127.0.0.1:1", Reachable: reachable, CheckedAt: now},
		{Name: "frontend", Address: "127.0.0.1:2", Reachable: reachable, CheckedAt: now},
		{Name: "smtp", Address: "127.0.0.1:3", Reachable: reachable, CheckedAt: now},
		{Name: "mailpit", Address: "127.0.0.1:4", Reachable: reachable, CheckedAt: now},
	}
}

func slicesCount(values []string, value string) int {
	count := 0
	for _, candidate := range values {
		if candidate == value {
			count++
		}
	}
	return count
}
