package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"hitkeep/auth"
)

func main() {
	write := flag.Bool("write", false, "write the generated Angular capability file")
	check := flag.Bool("check", false, "verify the generated Angular capability file is current")
	flag.Parse()

	if *write == *check {
		fmt.Fprintln(os.Stderr, "pass exactly one of --write or --check")
		os.Exit(2)
	}

	path := filepath.Clean(auth.GeneratedTypeScriptCapabilitiesPath)
	rendered := []byte(auth.RenderTypeScriptCapabilities())

	if *write {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "create output directory: %v\n", err)
			os.Exit(1)
		}
		// #nosec G306 -- this writes a generated source file that should keep normal checkout permissions.
		if err := os.WriteFile(path, rendered, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
			os.Exit(1)
		}
		return
	}

	current, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", path, err)
		os.Exit(1)
	}
	if !bytes.Equal(current, rendered) {
		fmt.Fprintf(os.Stderr, "%s is out of date; run go run ./cmd/auth-capabilities --write\n", path)
		os.Exit(1)
	}
}
