package analyticstools

import (
	"context"
	"errors"
	"strings"
)

const (
	ToolSearchDocs      = "hitkeep_search_docs"
	ToolGetDoc          = "hitkeep_get_doc"
	ToolGetAPIReference = "hitkeep_get_api_reference"
)

type docQueryInput struct {
	Query string `json:"query" jsonschema:"Search query for official HitKeep docs."`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum rows to return. Defaults to 10 and is capped at 50."`
}

type docPathInput struct {
	Path string `json:"path" jsonschema:"Official HitKeep docs path or URL."`
}

type apiReferenceInput struct {
	PathOrOperation string `json:"path_or_operation" jsonschema:"API docs path, operation slug, or official docs URL."`
}

type DocSearchOutput struct {
	Results []docSearchResult `json:"results"`
}

// DocOutput is one official docs page as markdown.
type DocOutput struct {
	URL      string `json:"url"`
	Path     string `json:"path"`
	Markdown string `json:"markdown"`
}

// DocsTools returns the official docs lookup tools, or none when docs are off.
func DocsTools(docs *Docs) []Tool {
	if docs == nil {
		return nil
	}
	fetch := func(ctx context.Context, path string) (DocOutput, error) {
		page, err := docs.GetMarkdown(ctx, path)
		return DocOutput(page), err
	}
	return []Tool{
		define(ToolSearchDocs, "Search HitKeep Docs",
			"Search official HitKeep documentation for setup, metric definitions, filters, limits, and product behavior. Use it before answering how HitKeep works.",
			true, func(ctx context.Context, _ Call, in docQueryInput) (DocSearchOutput, error) {
				results, err := docs.Search(ctx, in.Query, normalizeLimit(in.Limit))
				return DocSearchOutput{Results: results}, err
			}),
		define(ToolGetDoc, "Get HitKeep Doc",
			"Fetch one official HitKeep docs page as markdown, by path or URL from hitkeep_search_docs.",
			true, func(ctx context.Context, _ Call, in docPathInput) (DocOutput, error) {
				return fetch(ctx, in.Path)
			}),
		define(ToolGetAPIReference, "Get HitKeep API Reference",
			"Fetch an official HitKeep REST API reference page as markdown by path or operation slug.",
			true, func(ctx context.Context, _ Call, in apiReferenceInput) (DocOutput, error) {
				path := strings.TrimSpace(in.PathOrOperation)
				if path == "" {
					return DocOutput{}, errors.New("path_or_operation is required")
				}
				if !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
					path = "/api/operations/" + strings.Trim(path, "/") + "/"
				}
				return fetch(ctx, path)
			}),
	}
}
