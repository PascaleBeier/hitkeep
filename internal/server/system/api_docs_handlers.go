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

func (h *handler) handleGetAPIDocV1() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publicURL := strings.TrimSpace(h.ctx.Config.PublicURL)
		if publicURL == "" {
			publicURL = "http://localhost:8080"
		}

		spec := OpenAPISpecV1(publicURL)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(spec)
	}
}

// OpenAPISpecV1 exposes the generated v1 API specification so docs tooling can
