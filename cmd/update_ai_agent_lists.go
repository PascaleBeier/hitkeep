package hitkeepcmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"time"

	"hitkeep/internal/aianalytics"
)

func UpdateAIAgentLists(ctx context.Context, args []string, out, errOut io.Writer, logger *slog.Logger) error {
	fs := flag.NewFlagSet("update-ai-agent-lists", flag.ContinueOnError)
	fs.SetOutput(errOut)

	outputPath := fs.String("output", "internal/aianalytics/default_ai_agents.json", "Output path for the assembled AI agent master list")
	if err := fs.Parse(args); err != nil {
		return &ExitError{Code: 2}
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	data, err := aianalytics.FetchAIAgentData(ctx, nil, logger)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Error: could not fetch AI agent lists: %v\n", err)
		return &ExitError{Code: 1}
	}
	if err := aianalytics.ValidateEmbeddedAIAgentData(data); err != nil {
		_, _ = fmt.Fprintf(errOut, "Error: refusing to write incomplete embedded AI agent data: %v\n", err)
		return &ExitError{Code: 1}
	}
	if err := aianalytics.SaveAIAgentData(*outputPath, data); err != nil {
		_, _ = fmt.Fprintf(errOut, "Error: could not write AI agent data: %v\n", err)
		return &ExitError{Code: 1}
	}

	_, _ = fmt.Fprintf(out, "Wrote AI agent master list to %s\n", *outputPath)
	_, _ = fmt.Fprintf(out, "Agents: %d\n", len(data.Agents))
	_, _ = fmt.Fprintf(out, "AI referrers: %d\n", len(data.AIReferrers))
	return nil
}
