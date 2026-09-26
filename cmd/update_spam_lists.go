package hitkeepcmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"hitkeep/internal/listrefresh"
)

func UpdateSpamLists(ctx context.Context, outputPath string, out, errOut io.Writer, logger *slog.Logger) error {
	data, changed, err := listrefresh.RunSpam(ctx, outputPath, logger)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Error: %v\n", err)
		return &ExitError{Code: 1}
	}
	if changed {
		_, _ = fmt.Fprintf(out, "Wrote spam filter cache to %s\n", outputPath)
	} else {
		_, _ = fmt.Fprintf(out, "Spam filter cache unchanged at %s\n", outputPath)
	}
	_, _ = fmt.Fprintf(out, "Referrer hosts: %d\nBlocked networks: %d\n", len(data.ReferrerHostDenylist), len(data.NetworkDenylist))
	return nil
}
