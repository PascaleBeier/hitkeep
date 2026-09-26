package hitkeepcmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"hitkeep/internal/aianalytics"
)

func UpdateAIAgentLists(ctx context.Context, outputPath string, out, errOut io.Writer, logger *slog.Logger) error {
	return runUpdateList(
		ctx,
		outputPath,
		out,
		errOut,
		func(ctx context.Context) (aianalytics.AIAgentData, error) {
			return aianalytics.FetchAIAgentData(ctx, nil, logger)
		},
		aianalytics.ValidateEmbeddedAIAgentData,
		aianalytics.SaveAIAgentData,
		"could not fetch AI agent lists",
		"refusing to write incomplete embedded AI agent data",
		"could not write AI agent data",
		func(out io.Writer, data aianalytics.AIAgentData, outputPath string) {
			_, _ = fmt.Fprintf(out, "Wrote AI agent master list to %s\n", outputPath)
			_, _ = fmt.Fprintf(out, "Agents: %d\n", len(data.Agents))
			_, _ = fmt.Fprintf(out, "AI referrers: %d\n", len(data.AIReferrers))
		},
	)
}
