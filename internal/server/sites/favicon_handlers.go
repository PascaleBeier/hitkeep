package sites

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

func (h *handler) handleGetFavicon() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domain := normalizeFaviconDomain(r.PathValue("domain"))
		if !isValidFaviconDomain(domain) {
			http.Error(w, "Invalid domain", http.StatusBadRequest)
			return
		}

		ddgURL := (&url.URL{
			Scheme: "https",
			Host:   "icons.duckduckgo.com",
			Path:   fmt.Sprintf("/ip3/%s.ico", domain),
		}).String()

		target, err := url.Parse(ddgURL)
		if err != nil {
			http.Error(w, "Upstream error", http.StatusBadGateway)
			return
		}

		proxy := &httputil.ReverseProxy{
			Rewrite: func(proxyReq *httputil.ProxyRequest) {
				rewrittenURL := *target
				proxyReq.Out.URL = &rewrittenURL
				proxyReq.Out.Host = ""
				proxyReq.Out.Method = http.MethodGet
				proxyReq.Out.Body = nil
				proxyReq.Out.ContentLength = 0
				proxyReq.Out.Header.Del("Authorization")
				proxyReq.Out.Header.Del("Cookie")
				proxyReq.Out.Header.Del("X-Api-Key")
			},
			Transport: faviconProxyTransport,
			ModifyResponse: func(resp *http.Response) error {
				resp.Header.Set("Cache-Control", "public, max-age=86400")
				if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
					_ = resp.Body.Close()
					resp.StatusCode = http.StatusNoContent
					resp.Status = http.StatusText(http.StatusNoContent)
					resp.Body = http.NoBody
					resp.ContentLength = 0
					resp.Header.Del("Content-Length")
					resp.Header.Del("Content-Type")
				}
				return nil
			},
			ErrorHandler: func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
				slog.Warn("Failed to fetch favicon upstream", "domain", domain, "error", proxyErr)
				rw.Header().Set("Cache-Control", "public, max-age=300")
				rw.WriteHeader(http.StatusNoContent)
			},
		}

		proxy.ServeHTTP(w, r)
	}
}

func normalizeFaviconDomain(domain string) string {
	trimmed := strings.TrimSpace(domain)
	return strings.TrimSuffix(strings.ToLower(trimmed), ".")
}

func isValidFaviconDomain(domain string) bool {
	if domain == "" {
		return false
	}
	if strings.ContainsAny(domain, `/\?#`) {
		return false
	}
	return domainRegex.MatchString(domain)
}

func newFaviconProxyTransport(timeout time.Duration) http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = timeout
	return transport
}
