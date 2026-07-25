package aifetch

import (
	"encoding/json"
	"net/http"
	"sync"

	"hitkeep/internal/aianalytics"
	"hitkeep/internal/api"
)

// aiAgentCatalog derives the dashboard-facing catalog from the embedded AI
// agent master list once per process. Multiple tokens can share a display
// name (for example variant tokens of the same bot); the first entry with an
// icon host wins.
var aiAgentCatalog = sync.OnceValue(func() *api.AIAgentCatalog {
	data := aianalytics.MustEmbeddedAIAgentData()

	catalog := &api.AIAgentCatalog{
		GeneratedAt: data.GeneratedAt,
		Agents:      make([]api.AIAgentCatalogAgent, 0, len(data.Agents)),
		AIReferrers: make([]api.AIAgentCatalogReferrer, 0, len(data.AIReferrers)),
	}

	seen := make(map[string]int, len(data.Agents))
	for _, agent := range data.Agents {
		iconHost := aianalytics.IconHostFromURL(agent.URL)
		if index, ok := seen[agent.Name]; ok {
			if catalog.Agents[index].IconHost == "" {
				catalog.Agents[index].IconHost = iconHost
			}
			continue
		}
		seen[agent.Name] = len(catalog.Agents)
		catalog.Agents = append(catalog.Agents, api.AIAgentCatalogAgent{
			Name:     agent.Name,
			Family:   agent.Family,
			Category: agent.Category,
			IconHost: iconHost,
		})
	}

	for _, referrer := range data.AIReferrers {
		entry := api.AIAgentCatalogReferrer{Name: referrer.Name}
		if len(referrer.Hosts) > 0 {
			entry.IconHost = referrer.Hosts[0]
		}
		catalog.AIReferrers = append(catalog.AIReferrers, entry)
	}

	return catalog
})

// aiAgentCatalogJSON encodes the static catalog once instead of per request.
var aiAgentCatalogJSON = sync.OnceValue(func() []byte {
	raw, err := json.Marshal(aiAgentCatalog())
	if err != nil {
		// The catalog is built from embedded data with plain JSON types.
		panic("aifetch: cannot encode AI agent catalog: " + err.Error())
	}
	return raw
})

func (h *handler) handleGetAIAgentCatalog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// The catalog is static per release; let clients cache it for a day.
		w.Header().Set("Cache-Control", "private, max-age=86400")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(aiAgentCatalogJSON())
	}
}
