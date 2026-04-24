package ingest

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/cors"

	"hitkeep/internal/api"
	"hitkeep/internal/server/shared"
)

type handler struct {
	ctx *shared.Context
}

var (
	forwardedHostPattern   = regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*)$`)
	leaderForwardTransport = newProxyTransport(5 * time.Second)
)

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}
	ingestRoutes := http.NewServeMux()
	ingestRoutes.HandleFunc("POST /ingest", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.IngestLimiter,
	}, h.handleIngest()))
	ingestRoutes.HandleFunc("POST /ingest/event", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.IngestLimiter,
	}, h.handleIngestEvent()))

	corsHandler := newIngestCORS().Handler(ingestRoutes)
	mux.Handle("/ingest", corsHandler)
	mux.Handle("/ingest/event", corsHandler)
}

func newIngestCORS() *cors.Cors {
	return cors.New(cors.Options{
		AllowOriginVaryRequestFunc: func(_ *http.Request, origin string) (bool, []string) {
			return strings.TrimSpace(origin) != "", nil
		},
		AllowedMethods:   []string{http.MethodPost, http.MethodOptions},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
		MaxAge:           86400,
	})
}

func (h *handler) handleIngest() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Cluster.IsLeader() {
			h.handleIngestLeader(w, r)
		} else {
			h.handleIngestFollower(w, r)
		}
	}
}

func (h *handler) handleIngestLeader(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		http.Error(w, "Origin header is required", http.StatusBadRequest)
		return
	}

	parsedURL, err := url.Parse(origin)
	if err != nil {
		http.Error(w, "Invalid Origin header", http.StatusBadRequest)
		return
	}
	domain := normalizeOriginHostname(parsedURL.Hostname())

	site, err := h.ctx.Store.FindSiteByDomain(r.Context(), domain)
	if err != nil {
		slog.Error("Failed to find site", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if site == nil {
		slog.Warn("Dropped hit for unknown site")
		w.WriteHeader(http.StatusAccepted)
		return
	}

	userIP := shared.GetRealIP(r, h.ctx.Config.GetTrustedProxyNetworks())
	if h.ctx.IPFilter != nil && h.ctx.IPFilter.IsBlocked(site.ID, userIP) {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	type ingestPayload struct {
		Path      string    `json:"path"`
		Referrer  *string   `json:"referrer"`
		UserAgent *string   `json:"ua"`
		VPWidth   *int      `json:"vp_w"`
		VPHeight  *int      `json:"vp_h"`
		SCWidth   *int      `json:"sc_w"`
		SCHeight  *int      `json:"sc_h"`
		Language  *string   `json:"lang"`
		UTMSource *string   `json:"u_src"`
		UTMMedium *string   `json:"u_med"`
		UTMCamp   *string   `json:"u_cmp"`
		UTMTerm   *string   `json:"u_trm"`
		UTMCont   *string   `json:"u_cnt"`
		IsUnique  bool      `json:"unique"`
		SessionID uuid.UUID `json:"session_id"`
		PageID    uuid.UUID `json:"page_id"`
	}

	var payload ingestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Bad request body", http.StatusBadRequest)
		return
	}

	if h.ctx.SpamFilter != nil {
		decision := h.ctx.SpamFilter.Evaluate(site.Domain, userIP, payload.Referrer)
		if decision.Blocked {
			slog.Info("Dropped spam hit", "site_id", site.ID, "reason", decision.Reason)
			w.WriteHeader(http.StatusAccepted)
			return
		}
	}

	extractor := NewCountryCodeExtractor(h.ctx.Config.GetTrustedProxyNetworks())
	countryCode := extractor.ExtractFromRequest(r, payload.Language)

	var countryCodePtr *string
	if countryCode != "" {
		countryCodePtr = &countryCode
	}

	hit := api.Hit{
		SiteID:         site.ID,
		SessionID:      payload.SessionID,
		PageID:         payload.PageID,
		Timestamp:      time.Now().UTC(),
		Path:           payload.Path,
		Hostname:       &domain,
		Referrer:       payload.Referrer,
		UserAgent:      payload.UserAgent,
		ViewportWidth:  payload.VPWidth,
		ViewportHeight: payload.VPHeight,
		ScreenWidth:    payload.SCWidth,
		ScreenHeight:   payload.SCHeight,
		Language:       payload.Language,
		CountryCode:    countryCodePtr,
		UTMSource:      payload.UTMSource,
		UTMMedium:      payload.UTMMedium,
		UTMCampaign:    payload.UTMCamp,
		UTMTerm:        payload.UTMTerm,
		UTMContent:     payload.UTMCont,
		IsUnique:       &payload.IsUnique,
	}

	body, err := json.Marshal(hit)
	if err != nil {
		slog.Error("Failed to encode hit for NSQ", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := h.ctx.Producer.Publish("hits", body); err != nil {
		slog.Error("Failed to publish hit to NSQ", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *handler) forwardToLeader(w http.ResponseWriter, r *http.Request, targetPath string) {
	forwardURL, err := buildForwardURL(h.ctx.Cluster.GetLeaderAddr(), h.ctx.Config.HTTPAddr, targetPath)
	if err != nil {
		http.Error(w, "No leader available", http.StatusServiceUnavailable)
		return
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(proxyReq *httputil.ProxyRequest) {
			proxyReq.SetURL(forwardURL)
			proxyReq.Out.Header.Set("Content-Type", "application/json")
			if forwardedFor, ok := proxyReq.In.Header["X-Forwarded-For"]; ok {
				proxyReq.Out.Header["X-Forwarded-For"] = append([]string(nil), forwardedFor...)
			}
			proxyReq.SetXForwarded()
		},
		Transport: leaderForwardTransport,
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
			slog.Error("Follower failed to forward request", "error", proxyErr, "target_path", targetPath)
			http.Error(rw, "Failed to forward request", http.StatusBadGateway)
		},
	}

	proxy.ServeHTTP(w, r)
}

func (h *handler) handleIngestFollower(w http.ResponseWriter, r *http.Request) {
	h.forwardToLeader(w, r, "/ingest")
}

func (h *handler) handleIngestEvent() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Cluster.IsLeader() {
			h.handleIngestEventLeader(w, r)
		} else {
			h.handleIngestEventFollower(w, r)
		}
	}
}

func (h *handler) handleIngestEventLeader(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		http.Error(w, "Origin header is required", http.StatusBadRequest)
		return
	}

	parsedURL, err := url.Parse(origin)
	if err != nil {
		http.Error(w, "Invalid Origin header", http.StatusBadRequest)
		return
	}
	domain := normalizeOriginHostname(parsedURL.Hostname())

	site, err := h.ctx.Store.FindSiteByDomain(r.Context(), domain)
	if err != nil {
		slog.Error("Failed to find site", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if site == nil {
		slog.Warn("Dropped event for unknown site")
		w.WriteHeader(http.StatusAccepted)
		return
	}

	userIP := shared.GetRealIP(r, h.ctx.Config.GetTrustedProxyNetworks())
	if h.ctx.IPFilter != nil && h.ctx.IPFilter.IsBlocked(site.ID, userIP) {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	type eventPayload struct {
		Name       string         `json:"n"`
		Properties map[string]any `json:"p"`
		Referrer   *string        `json:"r"`
		SessionID  uuid.UUID      `json:"sid"`
	}

	var payload eventPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Bad request body", http.StatusBadRequest)
		return
	}

	if h.ctx.SpamFilter != nil {
		decision := h.ctx.SpamFilter.Evaluate(site.Domain, userIP, payload.Referrer)
		if decision.Blocked {
			slog.Info("Dropped spam event", "site_id", site.ID, "reason", decision.Reason)
			w.WriteHeader(http.StatusAccepted)
			return
		}
	}

	event := api.Event{
		SiteID:     site.ID,
		SessionID:  payload.SessionID,
		Name:       payload.Name,
		Properties: payload.Properties,
		Timestamp:  time.Now().UTC(),
	}

	body, err := json.Marshal(event)
	if err != nil {
		slog.Error("Failed to encode event for NSQ", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := h.ctx.Producer.Publish("events", body); err != nil {
		slog.Error("Failed to publish event to NSQ", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *handler) handleIngestEventFollower(w http.ResponseWriter, r *http.Request) {
	h.forwardToLeader(w, r, "/ingest/event")
}

func normalizeLeaderHost(addr string) string {
	if addr == "" {
		return ""
	}

	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return host
	}

	return addr
}

func buildForwardURL(leaderAddr, httpAddr, targetPath string) (*url.URL, error) {
	if targetPath != "/ingest" && targetPath != "/ingest/event" {
		return nil, fmt.Errorf("invalid forward target")
	}

	leaderHost := normalizeLeaderHost(leaderAddr)
	if !isValidForwardHost(leaderHost) {
		return nil, fmt.Errorf("invalid leader address")
	}

	_, port, err := net.SplitHostPort(httpAddr)
	if err != nil || port == "" {
		port = "8080"
	}

	return &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(leaderHost, port),
		Path:   targetPath,
	}, nil
}

func isValidForwardHost(host string) bool {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" {
		return false
	}
	if strings.ContainsAny(trimmed, `/\?#`) {
		return false
	}
	if _, err := netip.ParseAddr(trimmed); err == nil {
		return true
	}
	return forwardedHostPattern.MatchString(trimmed)
}

func newProxyTransport(timeout time.Duration) http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = timeout
	return transport
}

func normalizeOriginHostname(host string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), "www.")
}
