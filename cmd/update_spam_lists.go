package hitkeepcmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"hitkeep/internal/blocking"
)

func UpdateSpamLists(args []string) {
	fs := flag.NewFlagSet("update-spam-lists", flag.ExitOnError)
	fs.SetOutput(os.Stderr)

	defaultOutput := os.Getenv("HITKEEP_SPAM_FILTER_PATH")
	if defaultOutput == "" {
		dataPath := os.Getenv("HITKEEP_DATA_PATH")
		if dataPath == "" {
			dataPath = "data"
		}
		defaultOutput = dataPath + "/spam-filter.json"
	}

	outputPath := fs.String("output", defaultOutput, "Output path for the compiled spam filter cache")
	_ = fs.Parse(args)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	data, err := blocking.FetchSpamFeedData(ctx, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not fetch spam feeds: %v\n", err)
		os.Exit(1)
	}
	if err := blocking.ValidateEmbeddedSpamFeedData(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error: refusing to write incomplete embedded spam data: %v\n", err)
		os.Exit(1)
	}
	if err := blocking.SaveSpamFeedData(*outputPath, data); err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not write spam cache: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Wrote spam filter cache to %s\n", *outputPath)
	fmt.Printf("Referrer hosts: %d\n", len(data.ReferrerHostDenylist))
	fmt.Printf("Blocked networks: %d\n", len(data.NetworkDenylist))
}
