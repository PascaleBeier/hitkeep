package devtool

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const productionCommandPackage = "./cmd/hitkeep"
const developerPackagePrefix = "hitkeep/internal/devtool"

// ValidateProductionBoundary proves that no canonical production build variant
// can reach the developer platform through the Go package dependency graph.
// Go only links packages reachable from the selected main package, so this is
// the boundary that keeps cmd/hk and its adapters out of the HitKeep binary.
func (a *App) ValidateProductionBoundary(ctx context.Context) error {
	for _, variant := range CatalogSnapshot().Variants {
		arguments := []string{"list", "-deps", goTrimpathFlag, "-tags", strings.Join(variant.BuildTags, ","), productionCommandPackage}
		command := exec.CommandContext(ctx, a.commandExecutable("go"), arguments...) //nolint:gosec // The executable and arguments come from closed catalogs.
		command.Dir = a.workspace.Root
		command.Env = a.commandEnvironment(nil)
		output, err := command.CombinedOutput()
		if err != nil {
			return fmt.Errorf("inspect %s production dependencies: %w: %s", variant.ID, err, boundedCommandOutput(output))
		}
		if err := rejectDeveloperDependencies(variant.ID, string(output)); err != nil {
			return err
		}
	}
	return nil
}

func rejectDeveloperDependencies(variantID, dependencies string) error {
	for dependency := range strings.Lines(dependencies) {
		dependency = strings.TrimSpace(dependency)
		if dependency == developerPackagePrefix || strings.HasPrefix(dependency, developerPackagePrefix+"/") {
			return fmt.Errorf("production variant %s depends on developer-only package %s", variantID, dependency)
		}
	}
	return nil
}

func boundedCommandOutput(output []byte) string {
	const limit = 4096
	if len(output) > limit {
		output = output[:limit]
	}
	return redactError(strings.TrimSpace(string(output)))
}
