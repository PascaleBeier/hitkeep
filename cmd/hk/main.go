package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"hitkeep/internal/devtool/cli"
)

var version = "snapshot"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := cli.Execute(ctx, version, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if !cli.IsReported(err) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
