package devtool

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	json "hitkeep/jsonapi"
)

const (
	devReadyTimeout    = 10 * time.Minute
	devGracePeriod     = 10 * time.Second
	devProbeInterval   = time.Second
	maxDevEvents       = 200
	maxDevResultEvents = 3
	devEventReadChunk  = 64 * 1024
	maxDevEventBytes   = 1024 * 1024
)

type devSessionRecord struct {
	DevStatus
	SupervisorPID int `json:"supervisor_pid,omitempty"`
}

type devEventSink struct {
	app          *App
	record       devSessionRecord
	file         *os.File
	observer     func(DevEvent)
	segmentBytes int64
	segmentCount int
	mu           sync.Mutex
}

type devProcess struct {
	component string
	command   *exec.Cmd
	done      <-chan error
}

func normalizeDevRequest(request DevRequest) DevRequest {
	if request.Variant == "" {
		request.Variant = "self-hosted"
	}
	return request
}

func ValidateDevRequest(request DevRequest) error {
	request = normalizeDevRequest(request)
	if _, err := VariantByID(request.Variant); err != nil {
		return err
	}
	return nil
}

func (a *App) StartDevDetached(ctx context.Context, request DevRequest) (DevStartResult, error) {
	return a.StartDevDetachedObserved(ctx, request, nil)
}

func (a *App) StartDevDetachedObserved(ctx context.Context, request DevRequest, observer func(DevEvent)) (DevStartResult, error) {
	if err := VerifyDeveloperSource(a.workspace.Root); err != nil {
		return DevStartResult{}, err
	}
	request = normalizeDevRequest(request)
	if err := ValidateDevRequest(request); err != nil {
		return DevStartResult{}, err
	}
	record, reused, err := a.prepareDevSession(request, DevOwnerDetached)
	if err != nil {
		return DevStartResult{}, err
	}
	if reused {
		status := record.DevStatus
		observedDuringWait := false
		if status.State == DevStateStarting {
			observedDuringWait = true
			status, err = a.waitDevStartupObserved(ctx, observer)
			if err != nil {
				result, _ := a.devStartResult(status, true)
				return result, WithErrorData(err, result)
			}
		}
		result, resultErr := a.devStartResult(status, true)
		if observer != nil && !observedDuringWait {
			for _, event := range result.RecentEvents {
				observer(event)
			}
		}
		return result, resultErr
	}
	if a.devDetachedStart != nil {
		record.SupervisorPID = os.Getpid()
		record.UpdatedAt = time.Now().UTC()
		if err := a.writeDevRecord(record); err != nil {
			return DevStartResult{}, err
		}
		detachedContext := context.WithoutCancel(ctx)
		go func() {
			_ = a.devDetachedStart(detachedContext, record, request)
		}()
		status, waitErr := a.waitDevStartupObserved(ctx, observer)
		result, resultErr := a.devStartResult(status, false)
		if waitErr != nil {
			return result, WithErrorData(waitErr, result)
		}
		return result, resultErr
	}

	launcher := filepath.Join(a.workspace.Root, "hk")
	if info, statErr := os.Stat(launcher); statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		launcher = a.executable // compatibility for legacy/test workers; real worktrees always use ./hk
	}
	command := exec.CommandContext( //nolint:gosec
		context.WithoutCancel(ctx),
		launcher,
		"__dev",
		"--workspace", a.workspace.Root,
		"--generation-id", record.GenerationID,
		"--variant", request.Variant,
	)
	if request.Seed {
		command.Args = append(command.Args, "--seed")
	}
	command.Dir = a.workspace.Root
	stateRoot := filepath.Dir(filepath.Dir(a.workspace.StateDir))
	fingerprint, err := DeveloperSourceFingerprint(a.workspace.Root)
	if err != nil {
		if launcher != a.executable {
			return DevStartResult{}, a.failPreparedDev(record, fmt.Errorf("fingerprint development worker source: %w", err))
		}
		fingerprint = "legacy"
	}
	childEnvironment := []string{"HK_CHILD_DEV=1", "HK_STATE_DIR=" + stateRoot, "HK_EXPECTED_SCHEMA=" + SchemaVersion, "HK_WORKER_PROTOCOL=1", "HK_WORKSPACE_ID=" + a.workspace.ID, "HK_SOURCE_FINGERPRINT=" + fingerprint}
	if agentOutputEnabled(ctx) {
		childEnvironment = append(childEnvironment, "HK_CHILD_OUTPUT=json")
	}
	command.Env = a.commandEnvironment(childEnvironment)
	command.Stdin = nil
	command.Stdout = nil
	command.Stderr = nil
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return DevStartResult{}, a.failPreparedDev(record, fmt.Errorf("start development supervisor: %w", err))
	}
	record.SupervisorPID = command.Process.Pid
	record.UpdatedAt = time.Now().UTC()
	if err := a.writeDevRecord(record); err != nil {
		_ = command.Process.Kill()
		return DevStartResult{}, err
	}
	if err := os.WriteFile(a.devLaunchPath(), []byte("launch\n"), 0o600); err != nil {
		_ = command.Process.Kill()
		return DevStartResult{}, a.failPreparedDev(record, fmt.Errorf("release development supervisor: %w", err))
	}
	if err := command.Process.Release(); err != nil {
		return DevStartResult{}, fmt.Errorf("release development supervisor: %w", err)
	}
	status, waitErr := a.waitDevStartupObserved(ctx, observer)
	result, resultErr := a.devStartResult(status, false)
	if waitErr != nil {
		return result, WithErrorData(waitErr, result)
	}
	return result, resultErr
}

func (a *App) waitDevStartupObserved(ctx context.Context, observer func(DevEvent)) (DevStatus, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var cursor int64
	for {
		batch, err := a.DevLogs(cursor, maxDevEvents)
		if err != nil {
			return DevStatus{}, err
		}
		for _, event := range batch.Events {
			if observer != nil {
				observer(event)
			}
		}
		cursor = batch.NextCursor
		switch batch.Status.State {
		case DevStateReady, DevStateDegraded:
			return batch.Status, nil
		case DevStateFailed:
			return batch.Status, errors.New(batch.Status.Error)
		case DevStateStopped:
			return batch.Status, errors.New("development stopped before becoming ready")
		case DevStateStarting, DevStateStopping:
		}
		select {
		case <-ctx.Done():
			return batch.Status, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (a *App) StartDevForeground(ctx context.Context, request DevRequest, observer func(DevEvent)) (DevStartResult, error) {
	request = normalizeDevRequest(request)
	if err := ValidateDevRequest(request); err != nil {
		return DevStartResult{}, err
	}
	record, reused, err := a.prepareDevSession(request, DevOwnerForeground)
	if err != nil {
		return DevStartResult{}, err
	}
	if reused {
		return a.devStartResult(record.DevStatus, true)
	}
	record.SupervisorPID = os.Getpid()
	if err := a.writeDevRecord(record); err != nil {
		return DevStartResult{}, err
	}
	runErr := a.superviseDev(ctx, record, request, observer)
	status, statusErr := a.DevStatus(context.Background())
	if statusErr != nil {
		return DevStartResult{}, statusErr
	}
	result, resultErr := a.devStartResult(status, false)
	if runErr != nil {
		return result, WithErrorData(runErr, result)
	}
	return result, resultErr
}

func (a *App) ExecuteDevSession(ctx context.Context, generationID string, request DevRequest) error {
	if err := a.validateDevWorkerProtocol(); err != nil {
		return err
	}
	if os.Getenv("HK_CHILD_OUTPUT") == "json" {
		ctx = WithAgentOutput(ctx)
	}
	if err := a.waitUntilDevLaunch(ctx); err != nil {
		return err
	}
	defer os.Remove(a.devLaunchPath())
	record, err := a.readDevRecord()
	if err != nil {
		return err
	}
	if record.GenerationID != generationID || record.Owner != DevOwnerDetached {
		return errors.New("development worker generation no longer owns the workspace")
	}
	record.SupervisorPID = os.Getpid()
	record.UpdatedAt = time.Now().UTC()
	if err := a.writeDevRecord(record); err != nil {
		return err
	}
	return a.superviseDev(ctx, record, normalizeDevRequest(request), nil)
}

func (a *App) prepareDevSession(request DevRequest, owner DevOwner) (devSessionRecord, bool, error) {
	lock, err := lockStateRoot(a.workspace.StateDir, "dev")
	if err != nil {
		return devSessionRecord{}, false, fmt.Errorf("lock development session: %w", err)
	}
	defer unlockStateRoot(lock)

	status, statusErr := a.devStatusUnlocked()
	if statusErr == nil && devStateActive(status.State) {
		if status.Variant == request.Variant {
			if request.Seed {
				return devSessionRecord{}, false, errors.New("development is already active; use ./hk dev reset --seed to replace its data")
			}
			record, err := a.readDevRecord()
			return record, true, err
		}
		return devSessionRecord{}, false, fmt.Errorf("development is already %s with variant %s; run ./hk dev stop or ./hk dev reset", status.State, status.Variant)
	}
	now := time.Now().UTC()
	record := devSessionRecord{
		State:        DevStateStarting,
		GenerationID: time.Now().UTC().Format("20060102T150405") + "-" + uuid.NewString()[:8],
		Variant:      request.Variant,
		Owner:        owner,
		StartedAt:    &now,
		UpdatedAt:    now,
		URLs:         a.workspace.URLs,
		Services:     a.probeDevServices(context.Background()),
	}
	if err := os.MkdirAll(a.devEventsDir(), 0o700); err != nil {
		return devSessionRecord{}, false, err
	}
	if err := os.Remove(a.devCancelPath()); err != nil && !os.IsNotExist(err) {
		return devSessionRecord{}, false, err
	}
	if err := a.writeDevRecord(record); err != nil {
		return devSessionRecord{}, false, err
	}
	return record, false, nil
}

func (a *App) superviseDev(ctx context.Context, record devSessionRecord, request DevRequest, observer func(DevEvent)) error {
	sink, err := a.openDevEventSink(record, observer)
	if err != nil {
		return err
	}
	defer sink.Close()
	sink.Event("phase", "supervisor", "info", "starting", "starting development session")

	variant, err := VariantByID(request.Variant)
	if err != nil {
		return sink.Fail(err)
	}
	processes := make([]*devProcess, 0, 1)
	cleanup := func() error {
		stopContext, cancel := context.WithTimeout(context.Background(), a.devGracePeriod+5*time.Second)
		defer cancel()
		for _, process := range processes {
			if process.command.Process != nil {
				_ = syscall.Kill(-process.command.Process.Pid, syscall.SIGTERM)
			}
		}
		deadline := time.Now().Add(a.devGracePeriod)
		for _, process := range processes {
			if process.command.Process == nil || !processAlive(process.command.Process.Pid) {
				continue
			}
			select {
			case <-process.done:
			case <-time.After(max(time.Until(deadline), 0)):
				_ = syscall.Kill(-process.command.Process.Pid, syscall.SIGKILL)
			}
		}
		return a.runDevCommand(stopContext, sink, "compose", commandSpec{
			Args: a.composeArgs("down", "--remove-orphans"),
			Env:  a.ComposeEnvironment(variant),
		})
	}
	stopSession := func(message string, result error) error {
		sink.Transition(DevStateStopping, "stopping", message)
		if cleanupErr := cleanup(); cleanupErr != nil {
			return sink.Fail(fmt.Errorf("stop development: %w", cleanupErr))
		}
		sink.Services(a.probeDevServices(context.Background()))
		sink.Transition(DevStateStopped, "stopped", "development stopped")
		return result
	}

	if request.Seed {
		if err := a.runDevCommand(ctx, sink, "seed", commandSpec{
			Args: a.composeArgs("run", "--rm", "seed"),
			Env:  a.ComposeEnvironment(variant),
		}); err != nil {
			_ = cleanup()
			return sink.Fail(err)
		}
	}
	if err := a.runDevCommand(ctx, sink, "compose", commandSpec{
		Args: a.composeArgs("up", "--build", "-d", "backend", "frontend", "mailpit"),
		Env:  a.ComposeEnvironment(variant),
	}); err != nil {
		_ = cleanup()
		return sink.Fail(err)
	}
	logs, startErr := a.startDevProcess(ctx, sink, "compose", commandSpec{
		Args: a.composeArgs("logs", "--follow", "--no-color"),
		Env:  a.ComposeEnvironment(variant),
	})
	if startErr != nil {
		_ = cleanup()
		return sink.Fail(startErr)
	}
	processes = append(processes, logs)

	readyTimer := time.NewTimer(a.devReadyTimeout)
	defer readyTimer.Stop()
	probeTicker := time.NewTicker(a.devProbeInterval)
	defer probeTicker.Stop()
	cancelTicker := time.NewTicker(100 * time.Millisecond)
	defer cancelTicker.Stop()
	ready := false
	for {
		for _, process := range processes {
			select {
			case processErr := <-process.done:
				if ctxErr := ctx.Err(); ctxErr != nil {
					return stopSession("foreground development interrupted", ctxErr)
				}
				if processErr == nil {
					processErr = fmt.Errorf("%s process exited", process.component)
				}
				_ = cleanup()
				return sink.Fail(processErr)
			default:
			}
		}
		select {
		case <-ctx.Done():
			return stopSession("foreground development interrupted", ctx.Err())
		case <-cancelTicker.C:
			if _, statErr := os.Stat(a.devCancelPath()); statErr == nil {
				_ = os.Remove(a.devCancelPath())
				return stopSession("development stop requested", nil)
			}
		case <-readyTimer.C:
			if !ready {
				_ = cleanup()
				return sink.Fail(errors.New("development did not become ready within 10 minutes"))
			}
		case <-probeTicker.C:
			services := a.probeDevServices(ctx)
			allReady := allDevServicesReady(services)
			sink.Services(services)
			switch {
			case !ready && allReady:
				ready = true
				sink.Transition(DevStateReady, "ready", "development is ready at "+a.workspace.URLs.Web)
			case ready && !allReady && sink.record.State == DevStateReady:
				sink.Transition(DevStateDegraded, "degraded", "one or more development services are unavailable")
			case ready && allReady && sink.record.State == DevStateDegraded:
				sink.Transition(DevStateReady, "ready", "development services recovered")
			}
		}
	}
}

func (a *App) startDevProcess(ctx context.Context, sink *devEventSink, component string, spec commandSpec) (*devProcess, error) {
	if len(spec.Args) == 0 {
		return nil, errors.New("empty development command")
	}
	args := spec.Args
	environment := spec.Env
	if agentOutputEnabled(ctx) {
		args = agentOptimizedCommand(args)
		environment = append(slices.Clone(environment), "HK_AGENT_OUTPUT=json", "NO_COLOR=1", "TERM=dumb")
	}
	sink.Event("log", component, "info", "", "$ "+strings.Join(args, " "))
	command := exec.CommandContext(ctx, a.commandExecutable(args[0]), args[1:]...) //nolint:gosec
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		err := syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	command.WaitDelay = a.devGracePeriod
	command.Dir = a.workspace.Root
	if spec.Dir != "" {
		resolved, err := ValidateWorkspacePath(a.workspace, filepath.Join(a.workspace.Root, spec.Dir))
		if err != nil {
			return nil, err
		}
		command.Dir = resolved
	}
	command.Env = a.commandEnvironment(environment)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", component, err)
	}
	var readers sync.WaitGroup
	readers.Add(2)
	go scanDevOutput(stdout, component, "info", sink, &readers)
	go scanDevOutput(stderr, component, "error", sink, &readers)
	done := make(chan error, 1)
	go func() {
		waitErr := command.Wait()
		readers.Wait()
		if waitErr != nil && ctx.Err() == nil {
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				waitErr = fmt.Errorf("%s exited with code %d", component, exitErr.ExitCode())
			} else {
				waitErr = fmt.Errorf("%s exited: %w", component, waitErr)
			}
		}
		done <- waitErr
	}()
	return &devProcess{component: component, command: command, done: done}, nil
}

func scanDevOutput(reader io.Reader, component, level string, sink *devEventSink, wait *sync.WaitGroup) {
	defer wait.Done()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		lineComponent, message := parseComposeLog(component, scanner.Text())
		sink.Event("log", lineComponent, level, "", message)
	}
}

func parseComposeLog(fallback, line string) (string, string) {
	before, after, ok := strings.Cut(line, "|")
	if !ok {
		return fallback, line
	}
	component := strings.TrimSpace(before)
	for _, known := range []string{"backend", "frontend", "mailpit", "seed"} {
		if component == known || strings.HasPrefix(component, known+"-") || strings.Contains(component, "-"+known+"-") || strings.HasSuffix(component, "-"+known) {
			component = known
			break
		}
	}
	if component == "" {
		component = fallback
	}
	return component, strings.TrimSpace(after)
}

func (a *App) runDevCommand(ctx context.Context, sink *devEventSink, component string, spec commandSpec) error {
	process, err := a.startDevProcess(ctx, sink, component, spec)
	if err != nil {
		return err
	}
	err = <-process.done
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if err != nil {
		return err
	}
	return nil
}

func (a *App) openDevEventSink(record devSessionRecord, observer func(DevEvent)) (*devEventSink, error) {
	if err := os.MkdirAll(a.devEventsDir(), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(a.devEventsPath(record.GenerationID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	segmentBytes, segmentCount, err := devEventSegmentStats(file)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return &devEventSink{app: a, record: record, file: file, observer: observer, segmentBytes: segmentBytes, segmentCount: segmentCount}, nil
}

func (sink *devEventSink) Close() {
	_ = sink.file.Close()
}

func (sink *devEventSink) Event(eventType, component, level, phase, message string) {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	event := DevEvent{
		Cursor:    sink.record.NextEventCursor,
		Timestamp: time.Now().UTC(),
		Type:      eventType,
		Component: component,
		Level:     level,
		Phase:     phase,
		Message:   redactError(strings.TrimSpace(message)),
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return
	}
	if err := sink.rotateIfNeeded(len(raw) + 1); err != nil {
		return
	}
	written, err := sink.file.Write(append(raw, '\n'))
	if err != nil {
		return
	}
	sink.segmentBytes += int64(written)
	sink.segmentCount++
	sink.record.NextEventCursor++
	sink.record.UpdatedAt = event.Timestamp
	if eventType == "phase" {
		_ = sink.app.writeDevRecord(sink.record)
	}
	if sink.observer != nil {
		sink.observer(event)
	}
}

func (sink *devEventSink) Services(services []DevService) {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sink.record.Services = services
	sink.record.UpdatedAt = time.Now().UTC()
	_ = sink.app.writeDevRecord(sink.record)
}

func (sink *devEventSink) Transition(state DevState, phase, message string) {
	sink.mu.Lock()
	now := time.Now().UTC()
	sink.record.State = state
	sink.record.UpdatedAt = now
	sink.record.Error = ""
	switch state {
	case DevStateReady:
		if sink.record.ReadyAt == nil {
			sink.record.ReadyAt = &now
		}
	case DevStateStopping:
		sink.record.StoppingAt = &now
	case DevStateStopped:
		sink.record.StoppedAt = &now
		sink.record.SupervisorPID = 0
	case DevStateStarting, DevStateDegraded, DevStateFailed:
	}
	_ = sink.app.writeDevRecord(sink.record)
	sink.mu.Unlock()
	sink.Event("phase", "supervisor", "info", phase, message)
}

func (sink *devEventSink) Fail(err error) error {
	sink.mu.Lock()
	now := time.Now().UTC()
	sink.record.State = DevStateFailed
	sink.record.UpdatedAt = now
	sink.record.StoppedAt = &now
	sink.record.SupervisorPID = 0
	sink.record.Error = redactError(err.Error())
	_ = sink.app.writeDevRecord(sink.record)
	sink.mu.Unlock()
	sink.Event("phase", "supervisor", "error", "failed", err.Error())
	return err
}

func (a *App) DevStatus(ctx context.Context) (DevStatus, error) {
	status, err := a.devStatusUnlocked()
	if err == nil {
		return status, nil
	}
	if !os.IsNotExist(err) {
		return DevStatus{}, err
	}
	now := time.Now().UTC()
	return DevStatus{State: DevStateStopped, UpdatedAt: now, URLs: a.workspace.URLs, Services: a.probeDevServices(ctx)}, nil
}

func (a *App) devStatusUnlocked() (DevStatus, error) {
	record, err := a.readDevRecord()
	if err != nil {
		return DevStatus{}, err
	}
	if devStateActive(record.State) && record.SupervisorPID > 0 && time.Since(record.UpdatedAt) > 2*time.Second && !processAlive(record.SupervisorPID) {
		now := time.Now().UTC()
		record.State = DevStateFailed
		record.Error = "development supervisor exited before recording completion"
		record.StoppedAt = &now
		record.UpdatedAt = now
		record.SupervisorPID = 0
		_ = a.writeDevRecord(record)
	}
	return record.DevStatus, nil
}

func (a *App) WaitDevStartup(ctx context.Context) (DevStatus, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, err := a.DevStatus(ctx)
		if err != nil {
			return DevStatus{}, err
		}
		switch status.State {
		case DevStateReady, DevStateDegraded:
			return status, nil
		case DevStateFailed:
			return status, errors.New(status.Error)
		case DevStateStopped:
			return status, errors.New("development stopped before becoming ready")
		case DevStateStarting, DevStateStopping:
		}
		select {
		case <-ctx.Done():
			return status, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (a *App) StopDev(ctx context.Context) (DevStatus, error) {
	status, err := a.DevStatus(ctx)
	if err != nil {
		return DevStatus{}, err
	}
	if !devStateActive(status.State) {
		return status, nil
	}
	if err := os.WriteFile(a.devCancelPath(), []byte("stop requested\n"), 0o600); err != nil {
		return status, fmt.Errorf("request development stop: %w", err)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		current, statusErr := a.DevStatus(ctx)
		if statusErr != nil {
			return status, statusErr
		}
		status = current
		if status.State == DevStateStopped || status.State == DevStateFailed {
			return status, nil
		}
		select {
		case <-ctx.Done():
			return status, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (a *App) ResetDevData(ctx context.Context) error {
	return a.resetDevData(ctx, io.Discard)
}

func (a *App) DevLogs(cursor int64, limit int) (DevLogBatch, error) {
	if cursor < 0 {
		return DevLogBatch{}, errors.New("cursor must be non-negative")
	}
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, maxDevEvents)
	status, err := a.DevStatus(context.Background())
	if err != nil {
		return DevLogBatch{}, err
	}
	if status.GenerationID == "" {
		return DevLogBatch{Status: status, Events: []DevEvent{}, NextCursor: cursor, Complete: true}, nil
	}
	paths := devEventSegmentPaths(a.devEventsPath(status.GenerationID))
	if len(paths) == 0 {
		return DevLogBatch{Status: status, Events: []DevEvent{}, NextCursor: cursor, Complete: devStateTerminal(status.State)}, nil
	}
	events, observedNext, truncated, earliest, err := readDevEventSegments(paths, cursor, limit, status.NextEventCursor)
	if err != nil {
		return DevLogBatch{}, err
	}
	if cursor > observedNext {
		cursor = earliest
		events, observedNext, truncated, earliest, err = readDevEventSegments(paths, cursor, limit, observedNext)
		if err != nil {
			return DevLogBatch{}, err
		}
	}
	status.NextEventCursor = observedNext
	nextCursor := cursor
	if len(events) > 0 {
		nextCursor = events[len(events)-1].Cursor + 1
	} else if status.NextEventCursor > cursor {
		nextCursor = status.NextEventCursor
	}
	return DevLogBatch{
		Status: status, Events: events, NextCursor: nextCursor, EarliestCursor: earliest, DroppedEventCount: earliest, Truncated: truncated,
		Complete: devStateTerminal(status.State) && nextCursor >= status.NextEventCursor,
	}, nil
}

func (a *App) FollowDevEvents(ctx context.Context, cursor int64, limit int, observer func(DevEvent)) (DevLogBatch, error) {
	batch, err := a.DevLogs(cursor, limit)
	if err != nil {
		return batch, err
	}
	for _, event := range batch.Events {
		observer(event)
	}
	cursor = batch.NextCursor
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for !batch.Complete {
		select {
		case <-ctx.Done():
			return batch, ctx.Err()
		case <-ticker.C:
		}
		next, nextErr := a.DevLogs(cursor, maxDevEvents)
		if nextErr != nil {
			return batch, nextErr
		}
		for _, event := range next.Events {
			observer(event)
		}
		batch = next
		cursor = batch.NextCursor
	}
	return batch, nil
}

func (a *App) devStartResult(status DevStatus, reused bool) (DevStartResult, error) {
	cursor := max(status.NextEventCursor-maxDevResultEvents, 0)
	batch, err := a.DevLogs(cursor, maxDevResultEvents)
	if err != nil {
		return DevStartResult{Status: status, Reused: reused, NextCursor: status.NextEventCursor}, err
	}
	return DevStartResult{Status: status, Reused: reused, RecentEvents: batch.Events, NextCursor: batch.NextCursor}, nil
}

func (a *App) readDevRecord() (devSessionRecord, error) {
	var record devSessionRecord
	raw, err := os.ReadFile(a.devSessionPath())
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		return record, fmt.Errorf("decode development session: %w", err)
	}
	return record, nil
}

func (a *App) writeDevRecord(record devSessionRecord) error {
	return writeJSONAtomic(a.devSessionPath(), record)
}

func (a *App) failPreparedDev(record devSessionRecord, err error) error {
	now := time.Now().UTC()
	record.State = DevStateFailed
	record.Error = redactError(err.Error())
	record.UpdatedAt = now
	record.StoppedAt = &now
	record.SupervisorPID = 0
	_ = a.writeDevRecord(record)
	return err
}

func (a *App) devSessionPath() string {
	return filepath.Join(a.workspace.StateDir, "dev", "session.json")
}

func (a *App) devEventsDir() string {
	return filepath.Join(a.workspace.StateDir, "dev", "events")
}

func (a *App) devEventsPath(generationID string) string {
	return filepath.Join(a.devEventsDir(), generationID+".ndjson")
}

func (a *App) devCancelPath() string {
	return filepath.Join(a.workspace.StateDir, "dev", "cancel")
}

func (a *App) devLaunchPath() string {
	return filepath.Join(a.workspace.StateDir, "dev", "launch")
}

func (a *App) waitUntilDevLaunch(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		if _, err := os.Stat(a.devLaunchPath()); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return errors.New("timed out waiting for development launcher state")
		case <-ticker.C:
		}
	}
}

func devStateActive(state DevState) bool {
	return state == DevStateStarting || state == DevStateReady || state == DevStateDegraded || state == DevStateStopping
}

func devStateTerminal(state DevState) bool {
	return state == DevStateStopped || state == DevStateFailed
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || !errors.Is(err, syscall.ESRCH)
}

func (a *App) probeDevServices(ctx context.Context) []DevService {
	if a.devProbe != nil {
		return a.devProbe(ctx)
	}
	now := time.Now().UTC()
	definitions := []struct {
		name string
		port int
		http bool
	}{
		{name: "backend", port: a.workspace.Ports.Backend, http: true},
		{name: "frontend", port: a.workspace.Ports.Frontend},
		{name: "smtp", port: a.workspace.Ports.SMTP},
		{name: "mailpit", port: a.workspace.Ports.MailUI},
	}
	services := make([]DevService, 0, len(definitions))
	for _, definition := range definitions {
		address := net.JoinHostPort("127.0.0.1", strconv.Itoa(definition.port))
		reachable := false
		if definition.http {
			probeContext, cancel := context.WithTimeout(ctx, time.Second)
			request, _ := http.NewRequestWithContext(probeContext, http.MethodGet, "http://"+address+"/healthz", nil)
			response, err := http.DefaultClient.Do(request)
			if err == nil {
				reachable = response.StatusCode >= 200 && response.StatusCode < 300
				_ = response.Body.Close()
			}
			cancel()
		} else {
			connection, err := (&net.Dialer{Timeout: 500 * time.Millisecond}).DialContext(ctx, "tcp", address)
			if err == nil {
				reachable = true
				_ = connection.Close()
			}
		}
		services = append(services, DevService{Name: definition.name, Address: address, Reachable: reachable, CheckedAt: now})
	}
	return services
}

func allDevServicesReady(services []DevService) bool {
	return len(services) > 0 && !slices.ContainsFunc(services, func(service DevService) bool { return !service.Reachable })
}
