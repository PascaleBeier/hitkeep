package mcpserver

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/skills"
)

// registerPrompts offers the public analytics procedures, the same ones that
// ground Ask AI, as prompts an MCP client can start an analysis from.
func registerPrompts(server *mcp.Server) {
	for _, procedure := range skills.AnalyticsProcedures() {
		server.AddPrompt(&mcp.Prompt{
			Name:        procedure.Name,
			Title:       procedure.Title,
			Description: procedure.Title + " with HitKeep's read-only analytics tools.",
			Arguments: []*mcp.PromptArgument{{
				Name:        "question",
				Description: "What you want to find out, including the site and timeframe if known.",
			}},
		}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			text := procedure.Markdown
			if question := strings.TrimSpace(req.Params.Arguments["question"]); question != "" {
				text += "\n\nQuestion: " + question
			}
			return &mcp.GetPromptResult{
				Description: procedure.Title,
				Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: text}}},
			}, nil
		})
	}
}
