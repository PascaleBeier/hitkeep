package hitkeepcmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"time"

	runtimeconfig "hitkeep/config"
	"hitkeep/internal/blocking"
)

func UpdateSpamLists(ctx context.Context, args []string, out, errOut io.Writer, configFile string, logger *slog.Logger) error {
	conf, err := runtimeconfig.LoadArgs(nil, configFile, logger)
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("update-spam-lists", flag.ContinueOnError)
	fs.SetOutput(errOut)

	defaultOutput := conf.SpamFilterPath
	if defaultOutput == "" {
		defaultOutput = conf.DataPath + "/spam-filter.json"
	}

	outputPath := fs.String("output", defaultOutput, "Output path for the compiled spam filter cache")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return &ExitError{Code: 2}
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	data, err := blocking.FetchSpamFeedData(ctx, nil, logger)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Error: could not fetch spam feeds: %v\n", err)
		return &ExitError{Code: 1}
	}
	if err := blocking.ValidateEmbeddedSpamFeedData(data); err != nil {
		_, _ = fmt.Fprintf(errOut, "Error: refusing to write incomplete embedded spam data: %v\n", err)
		return &ExitError{Code: 1}
	}
	if err := blocking.SaveSpamFeedData(*outputPath, data); err != nil {
		_, _ = fmt.Fprintf(errOut, "Error: could not write spam cache: %v\n", err)
		return &ExitError{Code: 1}
	}

	_, _ = fmt.Fprintf(out, "Wrote spam filter cache to %s\n", *outputPath)
	_, _ = fmt.Fprintf(out, "Referrer hosts: %d\n", len(data.ReferrerHostDenylist))
	_, _ = fmt.Fprintf(out, "Blocked networks: %d\n", len(data.NetworkDenylist))
	return nil
}
