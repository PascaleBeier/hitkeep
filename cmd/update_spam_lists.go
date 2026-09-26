package hitkeepcmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"hitkeep/internal/blocking"
)

func UpdateSpamLists(ctx context.Context, outputPath string, out, errOut io.Writer, logger *slog.Logger) error {
	return runUpdateList(
		ctx,
		outputPath,
		out,
		errOut,
		func(ctx context.Context) (blocking.SpamFeedData, error) {
			return blocking.FetchSpamFeedData(ctx, nil, logger)
		},
		blocking.ValidateEmbeddedSpamFeedData,
		blocking.SaveSpamFeedData,
		"could not fetch spam feeds",
		"refusing to write incomplete embedded spam data",
		"could not write spam cache",
		func(out io.Writer, data blocking.SpamFeedData, outputPath string) {
			_, _ = fmt.Fprintf(out, "Wrote spam filter cache to %s\n", outputPath)
			_, _ = fmt.Fprintf(out, "Referrer hosts: %d\n", len(data.ReferrerHostDenylist))
			_, _ = fmt.Fprintf(out, "Blocked networks: %d\n", len(data.NetworkDenylist))
		},
	)
}
