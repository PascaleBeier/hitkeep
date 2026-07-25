package system

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (h *handler) handleGetAPIDocVersions() http.HandlerFunc {
	type versionInfo struct {
		Version    string `json:"version"`
		OpenAPIURL string `json:"openapi_url"`
		Latest     bool   `json:"latest"`
	}
	type response struct {
		Latest   string        `json:"latest"`
		Versions []versionInfo `json:"versions"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		resp := response{
			Latest: "v1",
			Versions: []versionInfo{
				{
					Version:    "v1",
					OpenAPIURL: "/api/docs/v1/openapi.json",
					Latest:     true,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// handleGetAPIDocV1 assembles and encodes the v1 specification once, at route
// registration time: the public URL is process-constant, so the served bytes
// never change and every request just writes the same buffer.
func (h *handler) handleGetAPIDocV1() http.HandlerFunc {
	publicURL := strings.TrimSpace(h.ctx.Config.PublicURL)
	if publicURL == "" {
		publicURL = "http://localhost:8080"
	}

	document, err := json.Marshal(OpenAPISpecV1(publicURL))
	if err != nil {
		// The specification is assembled from plain JSON types.
		panic("system: cannot encode OpenAPI document: " + err.Error())
	}

	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(document)
	}
}

// OpenAPISpecV1 exposes the generated v1 API specification so docs tooling can
