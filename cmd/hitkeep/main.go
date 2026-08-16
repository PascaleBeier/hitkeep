package main

import (
	"log/slog"
	"os"

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
		hitkeepcmd.Import(os.Args[2:])
		return
	}
	hitkeepcmd.Run(logger)
}
