package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"hitkeep/internal/ipmeta/refresh"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := refresh.Run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "ipmeta-generate: %v\n", err)
		os.Exit(1)
	}
}
