package cluster

import (
	"io"
	"log/slog"
	"testing"

	"github.com/hashicorp/memberlist"
)

func TestNewManagerRequiresLogger(t *testing.T) {
	if _, err := NewManager(nil, nil); err == nil {
		t.Fatal("expected missing logger error")
	}
}

func TestManagerShutdownNilSafe(t *testing.T) {
	var manager *Manager
	if err := manager.Shutdown(); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestEventDelegateMaintainsDeterministicLeaderState(t *testing.T) {
	manager := &Manager{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), self: "node-b", peers: map[string]string{"node-b": "127.0.0.2:7946"}}
	delegate := &eventDelegate{m: manager}

	delegate.NotifyJoin(&memberlist.Node{Name: "node-a", Addr: []byte{127, 0, 0, 1}, Port: 7946})
	if manager.IsLeader() {
		t.Fatal("manager is leader after lexically earlier peer joined")
	}
	if got, want := manager.GetLeaderAddr(), "127.0.0.1:7946"; got != want {
		t.Fatalf("GetLeaderAddr() after join = %q, want %q", got, want)
	}

	delegate.NotifyLeave(&memberlist.Node{Name: "node-a"})
	if !manager.IsLeader() {
		t.Fatal("manager is not leader after earlier peer left")
	}
	if got, want := manager.GetLeaderAddr(), "127.0.0.2:7946"; got != want {
		t.Fatalf("GetLeaderAddr() after leave = %q, want %q", got, want)
	}
}

func TestEventDelegateConcurrentUpdates(t *testing.T) {
	manager := &Manager{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), self: "node-0", peers: map[string]string{"node-0": "127.0.0.1"}}
	delegate := &eventDelegate{m: manager}
	done := make(chan struct{})
	go func() {
		for range 100 {
			delegate.NotifyJoin(&memberlist.Node{Name: "node-1", Addr: []byte{127, 0, 0, 1}})
			delegate.NotifyLeave(&memberlist.Node{Name: "node-1"})
		}
		close(done)
	}()
	for range 100 {
		_ = manager.IsLeader()
		_ = manager.GetLeaderAddr()
	}
	<-done
}
