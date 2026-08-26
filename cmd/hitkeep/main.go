package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	hitkeepcmd "hitkeep/cmd"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	root := hitkeepcmd.NewRootCommand(logger)
	if code := execute(context.Background(), root, logger); code != 0 {
		os.Exit(code)
	}
}

func execute(ctx context.Context, root *cobra.Command, logger *slog.Logger) int {
	if err := root.ExecuteContext(ctx); err != nil {
		if recoveryErr, ok := errors.AsType[*hitkeepcmd.RecoveryError](err); ok {
			return recoveryErr.Code
		} else if healthcheckErr, ok := errors.AsType[*hitkeepcmd.HealthcheckError](err); ok {
			_, _ = fmt.Fprintln(root.ErrOrStderr(), healthcheckErr)
		} else {
			logger.Error("Command failed", "error", err)
		}
		return 1
	}
	return 0
}
