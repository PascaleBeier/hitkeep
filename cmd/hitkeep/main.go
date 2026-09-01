package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	hitkeepcmd "hitkeep/cmd"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	root := hitkeepcmd.NewRootCommand(logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if code := execute(ctx, root, logger); code != 0 {
		os.Exit(code)
	}
}

func execute(ctx context.Context, root *cobra.Command, logger *slog.Logger) int {
	if err := hitkeepcmd.ExecuteRoot(ctx, root, os.Args[1:]); err != nil {
		if exitErr, ok := errors.AsType[*hitkeepcmd.ExitError](err); ok {
			return exitErr.Code
		} else if recoveryErr, ok := errors.AsType[*hitkeepcmd.RecoveryError](err); ok {
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
