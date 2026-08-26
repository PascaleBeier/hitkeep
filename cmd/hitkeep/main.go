package main

import (
	"log/slog"
	"os"

	hitkeepcmd "hitkeep/cmd"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := hitkeepcmd.NewRootCommand(logger).Execute(); err != nil {
		logger.Error("Command failed", "error", err)
		os.Exit(1)
	}
}
