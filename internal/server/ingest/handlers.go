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
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
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
	mux.HandleFunc("POST /api/ingest/server/pageview", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.IngestLimiter,
	}, h.handleServerPageviewIngest()))
	mux.HandleFunc("POST /api/ingest/server/event", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.IngestLimiter,
	}, h.handleServerEventIngest()))

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

type serverIngestContext struct {
	site      *api.Site
	domain    string
	path      string
	timestamp time.Time
	visitorIP string
	userAgent string
	utm       url.Values
}

func (h *handler) handleServerPageviewIngest() http.HandlerFunc {
	leaderHandler := h.ctx.RequireAPIClientAuth(h.handleServerPageviewIngestLeader())
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.isLeader() {
			h.forwardToLeader(w, r, "/api/ingest/server/pageview")
			return
		}
		leaderHandler(w, r)
	}
}

func (h *handler) handleServerPageviewIngestLeader() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		var payload api.ServerPageviewIngestRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			h.recordRejection()
			http.Error(w, "Bad request body", http.StatusBadRequest)
			return
		}

		ingestCtx, ok := h.resolveServerIngestContext(w, r, payload.URL, payload.Timestamp, payload.VisitorIP, payload.UserAgent)
		if !ok {
			return
		}

		if payload.DNT {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		if h.ctx.IPFilter != nil && h.ctx.IPFilter.IsBlocked(ingestCtx.site.ID, ingestCtx.visitorIP) {
			h.recordRejection()
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if h.ctx.SpamFilter != nil {
			decision := h.ctx.SpamFilter.Evaluate(ingestCtx.site.Domain, ingestCtx.visitorIP, payload.Referrer)
			if decision.Blocked {
				slog.Info("Dropped spam server-side hit", "site_id", ingestCtx.site.ID, "reason", decision.Reason)
				h.recordSpamDrop()
				w.WriteHeader(http.StatusAccepted)
				return
			}
		}

		sessionID := payload.SessionID
		if sessionID == uuid.Nil {
			sessionID = uuid.New()
		}
		pageID := payload.PageID
		if pageID == uuid.Nil {
			pageID = uuid.New()
		}

		countryCode := countryCodeFromVisitorIP(ingestCtx.visitorIP, h.ctx.Config.GetTrustedProxyNetworks())
		var countryCodePtr *string
		if countryCode != "" {
			countryCodePtr = &countryCode
		}

		userAgent := ingestCtx.userAgent
		hit := api.Hit{
			SiteID:         ingestCtx.site.ID,
			SessionID:      sessionID,
			PageID:         pageID,
			Timestamp:      ingestCtx.timestamp.UTC(),
			Path:           ingestCtx.path,
			Hostname:       &ingestCtx.domain,
			Referrer:       payload.Referrer,
			UserAgent:      &userAgent,
			ViewportWidth:  payload.VPWidth,
			ViewportHeight: payload.VPHeight,
			ScreenWidth:    payload.SCWidth,
			ScreenHeight:   payload.SCHeight,
			Language:       payload.Language,
			CountryCode:    countryCodePtr,
			UTMSource:      queryValuePtr(ingestCtx.utm, "utm_source"),
			UTMMedium:      queryValuePtr(ingestCtx.utm, "utm_medium"),
			UTMCampaign:    queryValuePtr(ingestCtx.utm, "utm_campaign"),
			UTMTerm:        queryValuePtr(ingestCtx.utm, "utm_term"),
			UTMContent:     queryValuePtr(ingestCtx.utm, "utm_content"),
		}
		if err := h.publishJSON("hits", hit); err != nil {
			slog.Error("Failed to publish server-side hit to NSQ", "error", err, "site_id", ingestCtx.site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

func (h *handler) handleServerEventIngest() http.HandlerFunc {
	leaderHandler := h.ctx.RequireAPIClientAuth(h.handleServerEventIngestLeader())
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.isLeader() {
			h.forwardToLeader(w, r, "/api/ingest/server/event")
			return
		}
		leaderHandler(w, r)
	}
}

func (h *handler) handleServerEventIngestLeader() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		var payload api.ServerEventIngestRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			h.recordRejection()
			http.Error(w, "Bad request body", http.StatusBadRequest)
			return
		}

		ingestCtx, ok := h.resolveServerIngestContext(w, r, payload.URL, payload.Timestamp, payload.VisitorIP, payload.UserAgent)
		if !ok {
			return
		}

		name := strings.TrimSpace(payload.Name)
		if name == "" {
			h.recordRejection()
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}

		if payload.DNT {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		if h.ctx.IPFilter != nil && h.ctx.IPFilter.IsBlocked(ingestCtx.site.ID, ingestCtx.visitorIP) {
			h.recordRejection()
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if h.ctx.SpamFilter != nil {
			decision := h.ctx.SpamFilter.Evaluate(ingestCtx.site.Domain, ingestCtx.visitorIP, payload.Referrer)
			if decision.Blocked {
				slog.Info("Dropped spam server-side event", "site_id", ingestCtx.site.ID, "reason", decision.Reason)
				h.recordSpamDrop()
				w.WriteHeader(http.StatusAccepted)
				return
			}
		}

		sessionID := payload.SessionID
		if sessionID == uuid.Nil {
			sessionID = uuid.New()
		}
		if payload.Properties == nil {
			payload.Properties = map[string]any{}
		}

		event := api.Event{
			SiteID:     ingestCtx.site.ID,
			SessionID:  sessionID,
			Name:       name,
			Properties: payload.Properties,
			Timestamp:  ingestCtx.timestamp.UTC(),
		}
		if err := h.publishJSON("events", event); err != nil {
			slog.Error("Failed to publish server-side event to NSQ", "error", err, "site_id", ingestCtx.site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

func (h *handler) handleIngest() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.isLeader() {
			h.handleIngestLeader(w, r)
		} else {
			h.handleIngestFollower(w, r)
		}
	}
}

func (h *handler) isLeader() bool {
	return h.ctx.Cluster == nil || h.ctx.Cluster.IsLeader()
}

func parseServerIngestURL(rawURL string) (*url.URL, string, string, bool) {
	parsedURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsedURL == nil {
		return nil, "", "", false
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, "", "", false
	}
	domain := normalizeOriginHostname(parsedURL.Hostname())
	if domain == "" {
		return nil, "", "", false
	}

	path := parsedURL.EscapedPath()
	if path == "" {
		path = "/"
	}
	if parsedURL.RawQuery != "" {
		path += "?" + parsedURL.RawQuery
	}
	return parsedURL, domain, path, true
}

func (h *handler) resolveServerIngestContext(w http.ResponseWriter, r *http.Request, rawURL, rawTimestamp, rawVisitorIP, rawUserAgent string) (serverIngestContext, bool) {
	apiClientAuth, _ := r.Context().Value(shared.APIClientAuthKey).(*database.APIClientAuth)
	if apiClientAuth == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return serverIngestContext{}, false
	}

	if h.ctx.Store == nil || h.ctx.Producer == nil {
		http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
		return serverIngestContext{}, false
	}

	parsedURL, domain, path, ok := parseServerIngestURL(rawURL)
	if !ok {
		h.recordRejection()
		http.Error(w, "url must be an absolute http or https URL", http.StatusBadRequest)
		return serverIngestContext{}, false
	}

	timestamp, err := time.Parse(time.RFC3339, strings.TrimSpace(rawTimestamp))
	if err != nil {
		h.recordRejection()
		http.Error(w, "timestamp must be RFC3339", http.StatusBadRequest)
		return serverIngestContext{}, false
	}

	visitorIP := strings.TrimSpace(rawVisitorIP)
	if parsedIP, err := netip.ParseAddr(visitorIP); err != nil || !parsedIP.IsValid() {
		h.recordRejection()
		http.Error(w, "visitor_ip must be a valid IP address", http.StatusBadRequest)
		return serverIngestContext{}, false
	}

	userAgent := strings.TrimSpace(rawUserAgent)
	if userAgent == "" {
		h.recordRejection()
		http.Error(w, "user_agent is required", http.StatusBadRequest)
		return serverIngestContext{}, false
	}

	site, err := h.ctx.Store.FindSiteByDomain(r.Context(), domain)
	if err != nil {
		slog.Error("Failed to find site", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return serverIngestContext{}, false
	}
	if site == nil {
		h.recordRejection()
		http.Error(w, "Site not found", http.StatusNotFound)
		return serverIngestContext{}, false
	}

	if !apiClientCanManageSiteData(apiClientAuth, site.ID) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return serverIngestContext{}, false
	}

	return serverIngestContext{
		site:      site,
		domain:    domain,
		path:      path,
		timestamp: timestamp,
		visitorIP: visitorIP,
		userAgent: userAgent,
		utm:       parsedURL.Query(),
	}, true
}

func queryValuePtr(values url.Values, key string) *string {
	value := strings.TrimSpace(values.Get(key))
	if value == "" {
		return nil
	}
	return &value
}

func (h *handler) publishJSON(topic string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal %s payload: %w", topic, err)
	}
	if h.ctx.Producer == nil {
		return fmt.Errorf("producer unavailable")
	}
	if err := h.ctx.Producer.Publish(topic, body); err != nil {
		return fmt.Errorf("publish %s payload: %w", topic, err)
	}
	return nil
}

func apiClientCanManageSiteData(apiClientAuth *database.APIClientAuth, siteID uuid.UUID) bool {
	if apiClientAuth == nil {
		return false
	}
	siteRole, ok := apiClientAuth.SiteRoles[siteID]
	return ok && siteRole.HasPermission(authcore.PermSiteManageData)
}

func countryCodeFromVisitorIP(visitorIP string, trustedProxyNets []netip.Prefix) string {
	req := &http.Request{
		Header:     http.Header{},
		RemoteAddr: net.JoinHostPort(visitorIP, "0"),
	}
	return shared.CountryCodeFromRequest(req, trustedProxyNets)
}

func (h *handler) handleIngestLeader(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		h.recordRejection()
		http.Error(w, "Origin header is required", http.StatusBadRequest)
		return
	}

	parsedURL, err := url.Parse(origin)
	if err != nil {
		h.recordRejection()
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
		h.recordRejection()
		w.WriteHeader(http.StatusAccepted)
		return
	}

	userIP := shared.GetRealIP(r, h.ctx.Config.GetTrustedProxyNetworks())
	if h.ctx.IPFilter != nil && h.ctx.IPFilter.IsBlocked(site.ID, userIP) {
		h.recordRejection()
		w.WriteHeader(http.StatusAccepted)
		return
	}

	type ingestPayload struct {
		Path           string    `json:"path"`
		Referrer       *string   `json:"referrer"`
		UserAgent      *string   `json:"ua"`
		VPWidth        *int      `json:"vp_w"`
		VPHeight       *int      `json:"vp_h"`
		SCWidth        *int      `json:"sc_w"`
		SCHeight       *int      `json:"sc_h"`
		Language       *string   `json:"lang"`
		UTMSource      *string   `json:"u_src"`
		UTMMedium      *string   `json:"u_med"`
		UTMCamp        *string   `json:"u_cmp"`
		UTMTerm        *string   `json:"u_trm"`
		UTMCont        *string   `json:"u_cnt"`
		TrackerSource  string    `json:"tsrc"`
		TrackerVersion string    `json:"tv"`
		IsUnique       bool      `json:"unique"`
		SessionID      uuid.UUID `json:"session_id"`
		PageID         uuid.UUID `json:"page_id"`
	}

	var payload ingestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.recordRejection()
		http.Error(w, "Bad request body", http.StatusBadRequest)
		return
	}

	if h.ctx.SpamFilter != nil {
		decision := h.ctx.SpamFilter.Evaluate(site.Domain, userIP, payload.Referrer)
		if decision.Blocked {
			slog.Info("Dropped spam hit", "site_id", site.ID, "reason", decision.Reason)
			h.recordSpamDrop()
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
		TrackerSource:  payload.TrackerSource,
		TrackerVersion: payload.TrackerVersion,
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
			proxyReq.Out.URL.Path = targetPath
			proxyReq.Out.URL.RawPath = ""
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
		if h.isLeader() {
			h.handleIngestEventLeader(w, r)
		} else {
			h.handleIngestEventFollower(w, r)
		}
	}
}

func (h *handler) handleIngestEventLeader(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		h.recordRejection()
		http.Error(w, "Origin header is required", http.StatusBadRequest)
		return
	}

	parsedURL, err := url.Parse(origin)
	if err != nil {
		h.recordRejection()
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
		h.recordRejection()
		w.WriteHeader(http.StatusAccepted)
		return
	}

	userIP := shared.GetRealIP(r, h.ctx.Config.GetTrustedProxyNetworks())
	if h.ctx.IPFilter != nil && h.ctx.IPFilter.IsBlocked(site.ID, userIP) {
		h.recordRejection()
		w.WriteHeader(http.StatusAccepted)
		return
	}

	type eventPayload struct {
		Name           string         `json:"n"`
		Properties     map[string]any `json:"p"`
		Referrer       *string        `json:"r"`
		SessionID      uuid.UUID      `json:"sid"`
		TrackerSource  string         `json:"tsrc"`
		TrackerVersion string         `json:"tv"`
	}

	var payload eventPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.recordRejection()
		http.Error(w, "Bad request body", http.StatusBadRequest)
		return
	}

	if h.ctx.SpamFilter != nil {
		decision := h.ctx.SpamFilter.Evaluate(site.Domain, userIP, payload.Referrer)
		if decision.Blocked {
			slog.Info("Dropped spam event", "site_id", site.ID, "reason", decision.Reason)
			h.recordSpamDrop()
			w.WriteHeader(http.StatusAccepted)
			return
		}
	}

	event := api.Event{
		SiteID:         site.ID,
		SessionID:      payload.SessionID,
		Name:           payload.Name,
		Properties:     payload.Properties,
		Timestamp:      time.Now().UTC(),
		TrackerSource:  payload.TrackerSource,
		TrackerVersion: payload.TrackerVersion,
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

func (h *handler) recordSpamDrop() {
	if h.ctx.SystemCounters != nil {
		h.ctx.SystemCounters.Spam.Add(1)
	}
}

func (h *handler) recordRejection() {
	if h.ctx.SystemCounters != nil {
		h.ctx.SystemCounters.Rejections.Add(1)
	}
}

func buildForwardURL(leaderAddr, httpAddr, targetPath string) (*url.URL, error) {
	switch targetPath {
	case "/ingest", "/ingest/event", "/api/ingest/server/pageview", "/api/ingest/server/event":
	default:
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
