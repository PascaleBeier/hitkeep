package server

import (
	"context"
	"crypto/subtle"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

var serverCustomTrackingHostnameRegex = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$`)

func (s *Server) customTrackingHostMiddleware(publicFS fs.FS, rootHandler, normalHandler http.Handler) http.Handler {
	fileServer := http.FileServer(http.FS(publicFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		domain, known, err := s.customTrackingDomainForRequest(r)
		if err != nil {
			s.writeDatabaseQueryFailure(w)
			return
		}
		if !known {
			normalHandler.ServeHTTP(w, r)
			return
		}
		if domain == nil || !customTrackingDomainCanRoute(*domain) {
			http.NotFound(w, r)
			return
		}

		switch {
		case isCustomTrackingAssetRoute(r):
			s.serveCustomTrackingAsset(publicFS, fileServer, w, r)
		case isCustomTrackingIngestRoute(r):
			rootHandler.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

func (s *Server) customTrackingDomainForRequest(r *http.Request) (*api.CustomTrackingDomain, bool, error) {
	if s == nil || s.store == nil || r == nil {
		return nil, false, nil
	}
	hostname := requestHostname(r.Host)
	if !isValidServerCustomTrackingHostname(hostname) {
		return nil, false, nil
	}
	domain, err := s.store.FindCustomTrackingDomainByHostname(r.Context(), hostname)
	if err != nil {
		slog.Error("Failed to resolve custom tracking request host", "error", err)
		return nil, true, err
	}
	return domain, domain != nil, nil
}

func requestHostname(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	return database.NormalizeCustomTrackingHostname(host)
}

func isValidServerCustomTrackingHostname(hostname string) bool {
	if len(hostname) > 253 || strings.Contains(hostname, "://") {
		return false
	}
	return serverCustomTrackingHostnameRegex.MatchString(hostname)
}

func customTrackingDomainCanRoute(domain api.CustomTrackingDomain) bool {
	return domain.Enabled &&
		domain.VerificationStatus == api.CustomTrackingDomainStatusVerified &&
		domain.TargetStatus == api.CustomTrackingDomainStatusVerified
}

func isCustomTrackingAssetRoute(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	return r.URL.Path == "/hk.js" || r.URL.Path == "/hk-vitals.js"
}

func isCustomTrackingIngestRoute(r *http.Request) bool {
	if r.Method != http.MethodPost && r.Method != http.MethodOptions {
		return false
	}
	switch r.URL.Path {
	case "/ingest", "/ingest/event", "/ingest/web-vitals":
		return true
	default:
		return false
	}
}

func (s *Server) serveCustomTrackingAsset(publicFS fs.FS, fileServer http.Handler, w http.ResponseWriter, r *http.Request) {
	assetPath := strings.TrimPrefix(r.URL.Path, "/")
	f, err := publicFS.Open(assetPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		http.NotFound(w, r)
		return
	}

	if isImmutableAsset(assetPath) {
		w.Header().Set("Cache-Control", cacheControlImmutable)
	} else {
		w.Header().Set("Cache-Control", cacheControlNoCache)
	}
	fileServer.ServeHTTP(w, r)
}

func isCaddyTLSAskPath(requestPath string) bool {
	return strings.HasPrefix(requestPath, "/internal/caddy/on-demand-tls/")
}

func (s *Server) handleCaddyOnDemandTLSAsk() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.conf == nil || s.store == nil {
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			return
		}
		expectedToken := strings.TrimSpace(s.conf.CaddyTLSAskToken)
		token := strings.TrimSpace(r.PathValue("token"))
		if expectedToken == "" || token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) != 1 {
			http.NotFound(w, r)
			return
		}

		hostname := database.NormalizeCustomTrackingHostname(r.URL.Query().Get("domain"))
		if !isValidServerCustomTrackingHostname(hostname) {
			http.Error(w, "Invalid domain", http.StatusBadRequest)
			return
		}

		domain, err := s.store.FindCustomTrackingDomainByHostname(r.Context(), hostname)
		if err != nil {
			slog.Error("Failed to resolve Caddy on-demand TLS domain", "error", err)
			s.writeDatabaseQueryFailure(w)
			return
		}
		if domain == nil || !customTrackingDomainCanRoute(*domain) {
			http.Error(w, "Domain is not allowed", http.StatusForbidden)
			return
		}

		if err := s.store.RecordCustomTrackingDomainTLSAsk(r.Context(), hostname, time.Now().UTC()); err != nil {
			slog.Warn("Failed to record Caddy on-demand TLS ask", "error", err, "hostname", hostname)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) writeDatabaseQueryFailure(w http.ResponseWriter) {
	if s != nil && s.store != nil {
		if err := s.store.Ping(context.Background()); err != nil {
			writeDatabaseUnavailable(w, database.DatabaseStateFailed)
			return
		}
	}
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}
