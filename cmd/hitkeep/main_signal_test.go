package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"hitkeep/internal/database"
)

func TestProductionMainSignalCancelsRunningApplication(t *testing.T) {
	if os.Getenv("HITKEEP_PRODUCTION_SIGNAL_SUBPROCESS") == "1" {
		os.Args = []string{"hitkeep"}
		main()
		return
	}

	dbPath, dataPath := prepareSplitCompleteSignalDatabase(t)

	for _, signal := range []struct {
		name string
		sig  os.Signal
	}{
		{name: "SIGINT", sig: os.Interrupt},
		{name: "SIGTERM", sig: syscall.SIGTERM},
	} {
		t.Run(signal.name, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestProductionMainSignalCancelsRunningApplication$")
			command.Env = append(os.Environ(),
				"HITKEEP_PRODUCTION_SIGNAL_SUBPROCESS=1",
				"HITKEEP_HTTP_ADDR=127.0.0.1:0",
				"HITKEEP_BIND_ADDR="+testSignalAddress(t),
				"HITKEEP_NSQ_TCP_ADDRESS="+testSignalAddress(t),
				"HITKEEP_NSQ_HTTP_ADDRESS="+testSignalAddress(t),
				"HITKEEP_DB_PATH="+dbPath,
				"HITKEEP_DATA_PATH="+dataPath,
				"HITKEEP_DB_COMPACT_ON_START=false",
			)
			stdout, err := command.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr lockedBuffer
			command.Stderr = &stderr
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}

			drained := make(chan error, 1)
			drainComplete := false
			waited := false
			defer func() {
				if waited {
					return
				}
				_ = command.Process.Kill()
				if !drainComplete {
					_ = stdout.Close()
					<-drained
				}
				_ = command.Wait()
			}()

			ready := make(chan error, 1)
			go func() {
				scanner := bufio.NewScanner(stdout)
				var logs []string
				readySent := false
				for scanner.Scan() {
					line := scanner.Text()
					logs = append(logs, line)
					if !readySent && strings.Contains(line, `"msg":"Application is running. Press Ctrl+C to exit."`) {
						ready <- nil
						readySent = true
					}
				}
				if !readySent {
					ready <- fmt.Errorf("application readiness log not found: %v; logs: %s", scanner.Err(), strings.Join(logs, "\n"))
				}
				drained <- scanner.Err()
			}()

			readyTimer := time.NewTimer(120 * time.Second)
			defer readyTimer.Stop()
			select {
			case err := <-ready:
				if err != nil {
					t.Fatalf("production application did not become ready: %v; stderr: %s", err, stderr.String())
				}
			case <-readyTimer.C:
				t.Fatalf("timed out waiting for production application readiness; stderr: %s", stderr.String())
			}

			if err := command.Process.Signal(signal.sig); err != nil {
				t.Fatal(err)
			}
			drainTimer := time.NewTimer(15 * time.Second)
			defer drainTimer.Stop()
			select {
			case err := <-drained:
				drainComplete = true
				if err != nil {
					t.Fatalf("production application stdout drain = %v; stderr: %s", err, stderr.String())
				}
			case <-drainTimer.C:
				_ = command.Process.Kill()
				_ = stdout.Close()
				<-drained
				drainComplete = true
				t.Fatalf("production application stdout did not drain after %s; stderr: %s", signal.name, stderr.String())
			}
			if err := command.Wait(); err != nil {
				waited = true
				t.Fatalf("production application exit = %v, want graceful exit; stderr: %s", err, stderr.String())
			}
			waited = true
		})
	}
}

// prepareSplitCompleteSignalDatabase keeps this test focused on full application readiness and signal shutdown; default-tenant migration has dedicated acceptance coverage.
func prepareSplitCompleteSignalDatabase(t *testing.T) (string, string) {
	t.Helper()

	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(root, "hitkeep.db")
	dataPath := filepath.Join(root, "data")
	store, err := database.OpenDefaultSplitControlStore(ctx, dbPath)
	if err != nil {
		t.Fatalf("open default split control store: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close default split control store: %v", err)
	}
	if err := database.RunDefaultTenantSplit(ctx, dbPath, dataPath); err != nil {
		t.Fatalf("prepare split-complete signal database: %v", err)
	}
	return dbPath, dataPath
}

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (buffer *lockedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.Write(data)
}

func (buffer *lockedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}

func testSignalAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}
