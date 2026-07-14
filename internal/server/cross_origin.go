package server

import "net/http"

// newCrossOriginProtection protects unsafe browser requests while leaving
// safe navigation and non-browser clients alone. Browser tracker ingest is
// intentionally cross-origin and enforces its own origin and site checks.
func newCrossOriginProtection() *http.CrossOriginProtection {
	protection := http.NewCrossOriginProtection()
	for _, pattern := range []string{
		"POST /ingest",
		"POST /ingest/event",
		"POST /ingest/web-vitals",
	} {
		protection.AddInsecureBypassPattern(pattern)
	}
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Not found", http.StatusNotFound)
	}))
	return protection
}
