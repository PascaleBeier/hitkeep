package shared

import (
	"net/http"
	"net/url"
	"strings"
)

// FetchMetadataMiddleware applies defense-in-depth request isolation rules for browser requests.
func FetchMetadataMiddleware(publicURL string, next http.Handler) http.Handler {
	configuredOrigin := parseConfiguredOrigin(publicURL)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secFetchSite := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
		path := r.URL.Path
		isStripeWebhook := path == "/api/cloud/webhooks/stripe"
		isServerIngest := path == "/api/ingest/server/pageview" || path == "/api/ingest/server/event"
		isSignupVerify := path == "/api/cloud/signup/verify"
		isMFAEmailLinkVerify := path == "/api/auth/mfa/email-link/verify"
		isGoogleSearchConsoleOAuthCallback := path == "/api/integrations/google-search-console/oauth/callback"

		if secFetchSite == "" {
			// Older/limited browsers fallback: for state-changing API requests, enforce
			// same-origin via Origin or Referer validation.
			if strings.HasPrefix(path, "/api/") && isStateChangingMethod(r.Method) && !isStripeWebhook && !isServerIngest {
				expectedOrigin := configuredOrigin
				if expectedOrigin == "" {
					expectedOrigin = requestOrigin(r)
				}
				if !matchesOriginOrReferer(r, expectedOrigin) {
					http.Error(w, "Not found", http.StatusNotFound)
					return
				}
			}

			next.ServeHTTP(w, r)
			return
		}

		if strings.HasPrefix(path, "/api/") && secFetchSite != "same-origin" && !isStripeWebhook && !isServerIngest && !isSignupVerify && !isMFAEmailLinkVerify && !isGoogleSearchConsoleOAuthCallback {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		if path == "/ingest" || strings.HasPrefix(path, "/ingest/") {
			mode := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Mode")))
			if mode == "navigate" || mode == "nested-navigate" {
				http.Error(w, "Not found", http.StatusNotFound)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func parseConfiguredOrigin(publicURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(publicURL))
	if err != nil {
		return ""
	}
	return canonicalOrigin(parsed)
}

func isStateChangingMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

func matchesOriginOrReferer(r *http.Request, expectedOrigin string) bool {
	if expectedOrigin == "" {
		return false
	}

	if origin := originFromURLString(r.Header.Get("Origin")); origin != "" {
		return origin == expectedOrigin
	}

	if refererOrigin := originFromURLString(r.Header.Get("Referer")); refererOrigin != "" {
		return refererOrigin == expectedOrigin
	}

	return false
}

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := firstCSVToken(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		scheme = strings.ToLower(forwarded)
	}

	host := strings.TrimSpace(r.Host)
	if host == "" {
		host = strings.TrimSpace(r.URL.Host)
	}

	return canonicalOrigin(&url.URL{
		Scheme: scheme,
		Host:   host,
	})
}

func originFromURLString(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return canonicalOrigin(parsed)
}

func canonicalOrigin(parsed *url.URL) string {
	if parsed == nil || parsed.Hostname() == "" {
		return ""
	}

	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return ""
	}

	host := strings.ToLower(parsed.Host)
	switch scheme {
	case "http":
		host = strings.TrimSuffix(host, ":80")
	case "https":
		host = strings.TrimSuffix(host, ":443")
	}

	return scheme + "://" + host
}

func firstCSVToken(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ",")
	return strings.TrimSpace(parts[0])
}
