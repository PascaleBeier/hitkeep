package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
	"hitkeep/internal/realtime"
	"hitkeep/internal/server/shared"
)

func TestShutdownClosesActiveRealtimeStreamBeforeDeadline(t *testing.T) {
	broker := realtime.NewBroker()
	limiters := [4]*shared.IPRateLimiter{
		shared.NewIPRateLimiter(rate.Inf, 1),
		shared.NewIPRateLimiter(rate.Inf, 1),
		shared.NewIPRateLimiter(rate.Inf, 1),
		shared.NewIPRateLimiter(rate.Inf, 1),
	}
	server := &Server{
		logger:         slog.Default(),
		ingestLimiter:  limiters[0],
		apiLimiter:     limiters[1],
		authLimiter:    limiters[2],
		webhookLimiter: limiters[3],
		ctx:            &shared.Context{Realtime: broker},
	}
	server.httpServer = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		shared.ServeRealtimeStream(w, r, broker, uuid.New())
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		if err := server.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("Serve: %v", err)
		}
	}()

	response, err := http.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}

	deadline, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started := time.Now()
	if err := server.Shutdown(deadline); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed >= time.Second/2 {
		t.Fatalf("shutdown took %s, expected well before deadline", elapsed)
	}
}

func TestShutdownClosesRealtimeBeforeBlockingImportStop(t *testing.T) {
	broker := realtime.NewBroker()
	limiters := [4]*shared.IPRateLimiter{
		shared.NewIPRateLimiter(rate.Inf, 1),
		shared.NewIPRateLimiter(rate.Inf, 1),
		shared.NewIPRateLimiter(rate.Inf, 1),
		shared.NewIPRateLimiter(rate.Inf, 1),
	}
	releaseImportStop := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseImportStop) })
	server := &Server{
		logger:         slog.Default(),
		ingestLimiter:  limiters[0],
		apiLimiter:     limiters[1],
		authLimiter:    limiters[2],
		webhookLimiter: limiters[3],
		ctx:            &shared.Context{Realtime: broker},
		importRunnerStop: func(context.Context) error {
			<-releaseImportStop
			return nil
		},
	}
	server.httpServer = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		shared.ServeRealtimeStream(w, r, broker, uuid.New())
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		if err := server.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("Serve: %v", err)
		}
	}()

	response, err := http.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	bodyDone := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(response.Body)
		bodyDone <- err
	}()
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- server.Shutdown(context.Background()) }()

	select {
	case err := <-bodyDone:
		if err != nil {
			t.Fatalf("realtime stream read: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("realtime stream stayed open while import shutdown blocked")
	}
	releaseOnce.Do(func() { close(releaseImportStop) })
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not complete after import stop released")
	}
}
