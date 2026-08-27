package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestProductionMainSignalCancelsRunningApplication(t *testing.T) {
	if os.Getenv("HITKEEP_PRODUCTION_SIGNAL_SUBPROCESS") == "1" {
		os.Args = []string{"hitkeep"}
		main()
		return
	}

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
				"HITKEEP_BIND_ADDR=127.0.0.1",
				"HITKEEP_NSQ_TCP_ADDRESS="+testSignalAddress(t),
				"HITKEEP_NSQ_HTTP_ADDRESS="+testSignalAddress(t),
				"HITKEEP_DB_PATH="+t.TempDir()+"/hitkeep.db",
				"HITKEEP_DATA_PATH="+t.TempDir(),
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

			readyTimer := time.NewTimer(30 * time.Second)
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
