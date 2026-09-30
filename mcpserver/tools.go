package mcpserver

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/analyticstools"
	"hitkeep/api"
	authcore "hitkeep/auth"
	"hitkeep/opportunities"
)

func (s *service) registerTools(server *mcp.Server) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: new(false)}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "hitkeep_list_sites",
		Title:       "List HitKeep Sites",
		Description: "List the HitKeep sites this API client can read. Call this first to find the site_id every analytics tool needs.",
		Annotations: readOnly,
	}, s.listSites)
	tools := append(analyticstools.Analytics(), opportunities.ReadTool)
	tools = append(tools, analyticstools.DocsTools(s.docs)...)
	analyticstools.RegisterMCP(server, analyticstools.Scope{
		Resolve:      s.resolveSite,
		MaxRangeDays: s.conf.MCPMaxRangeDays,
	}, tools...)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "hitkeep_get_mcp_help",
		Title:       "Get HitKeep MCP Help",
		Description: "Return local MCP usage guidance, token setup, privacy boundaries, date ranges, and filter syntax.",
		Annotations: readOnly,
	}, s.getMCPHelp)
}

// resolveSite authorizes the bearer API client for one site.
func (s *service) resolveSite(ctx context.Context, siteID uuid.UUID) (analyticstools.Site, error) {
	authz, err := apiAuth(ctx)
	if err != nil {
		return analyticstools.Site{}, err
	}
	role, ok := authz.SiteRoles[siteID]
	if !ok || !role.HasPermission(authcore.PermSiteView) {
		return analyticstools.Site{}, errors.New("forbidden")
	}
	analytics := s.store
	if s.tenantStores != nil {
		if analytics, _, err = s.tenantStores.ResolveSiteStore(ctx, siteID); err != nil {
			return analyticstools.Site{}, err
		}
	}
	return analyticstools.Site{ID: siteID, UserID: authz.UserID, Control: s.store, Analytics: analytics}, nil
}

func (s *service) listSites(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listSitesOutput, error) {
	authz, err := apiAuth(ctx)
	if err != nil {
		return nil, listSitesOutput{}, err
	}

	var sites []api.Site
	switch {
	case authz.UserID != uuid.Nil:
		sites, err = s.store.GetSites(ctx, authz.UserID)
	case authz.TenantID != uuid.Nil:
		sites, err = s.store.ListSitesForTenant(ctx, authz.TenantID)
	default:
		return nil, listSitesOutput{}, errors.New("unauthorized")
	}
	if err != nil {
		return nil, listSitesOutput{}, err
	}

	filtered := make([]api.Site, 0, len(sites))
	for _, site := range sites {
		if _, ok := authz.SiteRoles[site.ID]; ok {
			filtered = append(filtered, site)
		}
	}
	return nil, listSitesOutput{Sites: toMCPSites(filtered)}, nil
}

func (s *service) getMCPHelp(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, analyticstools.DocOutput, error) {
	return nil, analyticstools.DocOutput{URL: helpMCPURI, Path: helpMCPURI, Markdown: mcpHelpMarkdown()}, nil
}
