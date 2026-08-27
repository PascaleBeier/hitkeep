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
			t.Parallel()
			command := exec.Command(os.Args[0], "-test.run=^TestProductionMainSignalCancelsRunningApplication$")
			command.Env = append(os.Environ(),
				"HITKEEP_PRODUCTION_SIGNAL_SUBPROCESS=1",
				"HITKEEP_HTTP_ADDR=127.0.0.1:0",
				"HITKEEP_BIND_ADDR="+testSignalAddress(t),
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

			exited := false
			done := make(chan error, 1)
			go func() { done <- command.Wait() }()
			defer func() {
				if exited {
					return
				}
				_ = command.Process.Kill()
				timer := time.NewTimer(5 * time.Second)
				defer timer.Stop()
				select {
				case <-done:
				case <-timer.C:
				}
			}()

			ready := make(chan error, 1)
			go func() {
				scanner := bufio.NewScanner(stdout)
				var logs []string
				for scanner.Scan() {
					line := scanner.Text()
					logs = append(logs, line)
					if strings.Contains(line, `"msg":"Application is running. Press Ctrl+C to exit."`) {
						ready <- nil
						return
					}
				}
				ready <- fmt.Errorf("application readiness log not found: %v; logs: %s", scanner.Err(), strings.Join(logs, "\n"))
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
			exitTimer := time.NewTimer(15 * time.Second)
			defer exitTimer.Stop()
			select {
			case err := <-done:
				exited = true
				if err != nil {
					t.Fatalf("production application exit = %v, want graceful exit; stderr: %s", err, stderr.String())
				}
			case <-exitTimer.C:
				t.Fatalf("production application did not exit after %s; stderr: %s", signal.name, stderr.String())
			}
		})
	}
}

type lockedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (buffer *lockedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.Buffer.Write(data)
}

func (buffer *lockedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.Buffer.String()
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
