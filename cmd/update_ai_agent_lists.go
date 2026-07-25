package hitkeepcmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"hitkeep/internal/aianalytics"
)

func UpdateAIAgentLists(args []string) {
	fs := flag.NewFlagSet("update-ai-agent-lists", flag.ExitOnError)
	fs.SetOutput(os.Stderr)

	outputPath := fs.String("output", "internal/aianalytics/default_ai_agents.json", "Output path for the assembled AI agent master list")
	_ = fs.Parse(args)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	data, err := aianalytics.FetchAIAgentData(ctx, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not fetch AI agent lists: %v\n", err)
		os.Exit(1)
	}
	if err := aianalytics.ValidateEmbeddedAIAgentData(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error: refusing to write incomplete embedded AI agent data: %v\n", err)
		os.Exit(1)
	}
	if err := aianalytics.SaveAIAgentData(*outputPath, data); err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not write AI agent data: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Wrote AI agent master list to %s\n", *outputPath)
	fmt.Printf("Agents: %d\n", len(data.Agents))
	fmt.Printf("AI referrers: %d\n", len(data.AIReferrers))
}
