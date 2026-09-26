// Command update refreshes the signed extension bundle for the DuckDB selected
// by go.mod. Run from the repository root. -check never writes or downloads.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"hitkeep/internal/duckdbextensions/refresh"
)

func main() {
	check := flag.Bool("check", false, "verify the bundle against go.mod without downloading or writing")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected arguments")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := refresh.Run(ctx, *check, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
