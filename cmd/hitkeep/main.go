package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	hitkeepcmd "hitkeep/cmd"

	"github.com/spf13/cobra"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	root := hitkeepcmd.NewRootCommand(logger)
	if execute(context.Background(), root, logger) != 0 {
		os.Exit(1)
	}
}

func execute(ctx context.Context, root *cobra.Command, logger *slog.Logger) int {
	if err := root.ExecuteContext(ctx); err != nil {
		var healthcheckErr *hitkeepcmd.HealthcheckError
		if errors.As(err, &healthcheckErr) {
			_, _ = fmt.Fprintln(root.ErrOrStderr(), healthcheckErr)
		} else {
			logger.Error("Command failed", "error", err)
		}
		return 1
	}
	return 0
}
