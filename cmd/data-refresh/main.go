// Command data-refresh updates one embedded maintenance target per invocation.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	duckdbrefresh "hitkeep/internal/duckdbextensions/refresh"
	iprefresh "hitkeep/internal/ipmeta/refresh"
	"hitkeep/internal/listrefresh"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("choose one target: ai-agents, spam, ipmeta, duckdb")
	}
	switch args[0] {
	case "ai-agents", "spam":
		flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		defaultPath := "internal/aianalytics/default_ai_agents.json"
		if args[0] == "spam" {
			defaultPath = "internal/blocking/default_spam_filter.json"
		}
		outputPath := flags.String("output", defaultPath, "generated list path")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected arguments: %v", flags.Args())
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if args[0] == "ai-agents" {
			data, changed, err := listrefresh.RunAI(ctx, *outputPath, slog.Default())
			if err != nil {
				return err
			}
			if changed {
				_, _ = fmt.Fprintf(out, "Wrote AI agent master list to %s\n", *outputPath)
			} else {
				_, _ = fmt.Fprintf(out, "AI agent master list unchanged at %s\n", *outputPath)
			}
			_, _ = fmt.Fprintf(out, "Agents: %d\nAI referrers: %d\n", len(data.Agents), len(data.AIReferrers))
			return nil
		}
		data, changed, err := listrefresh.RunSpam(ctx, *outputPath, slog.Default())
		if err != nil {
			return err
		}
		if changed {
			_, _ = fmt.Fprintf(out, "Wrote spam filter cache to %s\n", *outputPath)
		} else {
			_, _ = fmt.Fprintf(out, "Spam filter cache unchanged at %s\n", *outputPath)
		}
		_, _ = fmt.Fprintf(out, "Referrer hosts: %d\nBlocked networks: %d\n", len(data.ReferrerHostDenylist), len(data.NetworkDenylist))
		return nil
	case "ipmeta":
		return iprefresh.Run(ctx, args[1:], out)
	case "duckdb":
		flags := flag.NewFlagSet("duckdb", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		check := flags.Bool("check", false, "verify the bundle without downloading or writing")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected arguments: %v", flags.Args())
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return duckdbrefresh.Run(ctx, *check, out)
	default:
		return fmt.Errorf("unknown data refresh target %q", args[0])
	}
}
