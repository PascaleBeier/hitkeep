package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	hitkeepcmd "hitkeep/cmd"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if len(os.Args) > 1 && os.Args[1] == "recover" {
		hitkeepcmd.Recover(logger)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "update-spam-lists" {
		hitkeepcmd.UpdateSpamLists(os.Args[2:], logger)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "update-ai-agent-lists" {
		hitkeepcmd.UpdateAIAgentLists(os.Args[2:], logger)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "import" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		hitkeepcmd.Import(ctx, os.Args[2:])
		return
	}
	hitkeepcmd.Run(logger)
}
