package hitkeepcmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"hitkeep/internal/listrefresh"
)

func UpdateAIAgentLists(ctx context.Context, outputPath string, out, errOut io.Writer, logger *slog.Logger) error {
	data, changed, err := listrefresh.RunAI(ctx, outputPath, logger)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Error: %v\n", err)
		return &ExitError{Code: 1}
	}
	if changed {
		_, _ = fmt.Fprintf(out, "Wrote AI agent master list to %s\n", outputPath)
	} else {
		_, _ = fmt.Fprintf(out, "AI agent master list unchanged at %s\n", outputPath)
	}
	_, _ = fmt.Fprintf(out, "Agents: %d\nAI referrers: %d\n", len(data.Agents), len(data.AIReferrers))
	return nil
}
