//go:build !billing

package cloud

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"hitkeep/internal/config"
	"hitkeep/internal/server/shared"
)

func TestRegisterDoesNotExposeDiscoveryRoutesInOSSBuild(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, &shared.Context{Config: &config.Config{CloudHosted: true, PublicURL: "https://cloud.hitkeep.eu"}})

	for _, path := range []string{"/openapi.json", "/.well-known/mcp/server-card.json", "/.well-known/integrations.json", "/.well-known/api-catalog"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected status 404, got %d body %q", rec.Code, rec.Body.String())
			}
		})
	}
}
