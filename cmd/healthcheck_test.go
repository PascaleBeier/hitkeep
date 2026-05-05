package hitkeepcmd

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"hitkeep/internal/config"
)

func TestRunHealthcheckUsesHeadRequest(t *testing.T) {
	requests := make(chan struct {
		method string
		path   string
	}, 1)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct {
			method string
			path   string
		}{method: r.Method, path: r.URL.Path}
		w.WriteHeader(http.StatusOK)
	}))
	server.Listener = listener
	server.Start()
	defer server.Close()

	if err := runHealthcheck(&config.Config{HTTPAddr: listener.Addr().String()}); err != nil {
		t.Fatalf("runHealthcheck: %v", err)
	}

	got := <-requests
	if got.method != http.MethodHead {
		t.Fatalf("expected HEAD request, got %s", got.method)
	}
	if got.path != "/healthz" {
		t.Fatalf("expected /healthz path, got %q", got.path)
	}
}
