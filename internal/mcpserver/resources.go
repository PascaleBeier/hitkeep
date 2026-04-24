package mcpserver

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	helpMCPURI     = "hitkeep://help/mcp"
	helpMetricsURI = "hitkeep://help/metrics"
	docsLLMSURI    = "hitkeep://docs/llms"
)

func (s *service) registerResources(server *mcp.Server) {
	server.AddResource(&mcp.Resource{
		URI:         helpMCPURI,
		Name:        "hitkeep-mcp-help",
		Title:       "HitKeep MCP Help",
		Description: "Usage guidance for the official HitKeep MCP server.",
		MIMEType:    "text/markdown",
	}, s.readResource)
	server.AddResource(&mcp.Resource{
		URI:         helpMetricsURI,
		Name:        "hitkeep-metric-definitions",
		Title:       "HitKeep Metric Definitions",
		Description: "Definitions for core HitKeep analytics metrics returned by MCP tools.",
		MIMEType:    "text/markdown",
	}, s.readResource)
	if s.docs != nil {
		server.AddResource(&mcp.Resource{
			URI:         docsLLMSURI,
			Name:        "hitkeep-docs-llms",
			Title:       "HitKeep LLM Docs Catalog",
			Description: "The official HitKeep llms.txt catalog.",
			MIMEType:    "text/markdown",
		}, s.readResource)
		server.AddResourceTemplate(&mcp.ResourceTemplate{
			URITemplate: "hitkeep://docs/{+path}",
			Name:        "hitkeep-docs-page",
			Title:       "HitKeep Docs Page",
			Description: "Official HitKeep documentation page returned as markdown.",
			MIMEType:    "text/markdown",
		}, s.readResource)
	}
}

func (s *service) readResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	if req == nil || req.Params == nil {
		return nil, errors.New("resource URI is required")
	}
	uri := req.Params.URI
	var text string
	switch uri {
	case helpMCPURI:
		text = mcpHelpMarkdown()
	case helpMetricsURI:
		text = metricDefinitionsMarkdown()
	case docsLLMSURI:
		if s.docs == nil {
			return nil, errors.New("MCP docs resources are disabled")
		}
		page, err := s.docs.GetMarkdown(ctx, "/llms.txt")
		if err != nil {
			return nil, err
		}
		text = page.Markdown
	default:
		if s.docs == nil {
			return nil, errors.New("MCP docs resources are disabled")
		}
		page, err := s.docs.GetMarkdown(ctx, uri)
		if err != nil {
			return nil, err
		}
		text = page.Markdown
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      uri,
			MIMEType: "text/markdown",
			Text:     text,
		}},
	}, nil
}

func metricDefinitionsMarkdown() string {
	return `# HitKeep Metric Definitions

- **Total pageviews**: Count of tracked pageview hits in the selected range.
- **Unique sessions**: Count of distinct cookieless sessions observed in the selected range.
- **Bounce rate**: Share of sessions with only one pageview.
- **Average session duration**: Mean session duration in seconds.
- **Pages per session**: Average pageviews per session.
- **Live visitors**: Distinct sessions seen in the last five minutes.
- **Top pages**: Highest pageview paths in the selected range.
- **Top referrers**: Highest referrer groups after HitKeep's referrer normalization.
- **Goal conversions**: Count of goal matches in the selected range.
- **Ecommerce revenue**: Sum of purchase revenue derived from ecommerce events.
- **AI fetches**: Server-side crawler fetch records from known AI assistants.
- **AI-referred visits**: Human visits with AI assistant referrers.
`
}
