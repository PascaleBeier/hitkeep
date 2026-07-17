package devtool

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
)

type agentOutputContextKey struct{}

// WithAgentOutput asks finite operations to prefer machine-readable output
// from the tools they invoke. The surrounding hk envelope remains the public
// contract; this only improves the diagnostic logs captured inside it.
func WithAgentOutput(ctx context.Context) context.Context {
	return context.WithValue(ctx, agentOutputContextKey{}, true)
}

func agentOutputEnabled(ctx context.Context) bool {
	enabled, _ := ctx.Value(agentOutputContextKey{}).(bool)
	return enabled
}

func agentOptimizedCommand(args []string) []string {
	result := slices.Clone(args)
	if len(result) == 0 {
		return result
	}

	if filepath.Base(result[0]) == "hk" && !hasArgument(result, "--output") {
		return append(result, "--output", "json")
	}
	if filepath.Base(result[0]) == "go" && len(result) > 1 {
		switch result[1] {
		case "test", "vet":
			if !hasExactArgument(result[2:], "-json") {
				result = slices.Insert(result, 2, "-json")
			}
		case "run":
			if len(result) > 2 && strings.Contains(result[2], "staticcheck") && !hasArgument(result[3:], "-f") {
				result = slices.Insert(result, 3, "-f=json")
			}
			if len(result) > 2 && strings.Contains(result[2], "govulncheck") && !hasArgument(result[3:], "-format") && !hasExactArgument(result[3:], "-json") {
				result = slices.Insert(result, 3, "-format=json")
			}
		}
		return result
	}
	if strings.Contains(filepath.Base(result[0]), "golangci-lint") {
		if !hasArgument(result, "--output.json.path") {
			result = append(result, "--output.text.path=", "--output.json.path=stdout")
		}
		if !hasArgument(result, "--show-stats") {
			result = append(result, "--show-stats=false")
		}
		if !hasArgument(result, "--color") {
			result = append(result, "--color=never")
		}
		return result
	}
	if filepath.Base(result[0]) == "zizmor" {
		if !hasArgument(result, "--format") {
			result = append(result, "--format=json")
		}
		if !hasExactArgument(result, "--no-progress") {
			result = append(result, "--no-progress")
		}
		if !hasArgument(result, "--color") {
			result = append(result, "--color=never")
		}
		return result
	}
	if filepath.Base(result[0]) == "docker" && len(result) > 2 {
		if result[1] == "buildx" && result[2] == "build" && !hasArgument(result[3:], "--progress") {
			return append(result, "--progress=rawjson")
		}
		if result[1] == "compose" && !hasArgument(result[2:], "--progress") {
			return slices.Insert(result, 2, "--progress=json")
		}
	}
	return result
}

func hasExactArgument(args []string, name string) bool {
	return slices.Contains(args, name)
}

func hasArgument(args []string, name string) bool {
	for _, argument := range args {
		if argument == name || strings.HasPrefix(argument, name+"=") {
			return true
		}
	}
	return false
}
