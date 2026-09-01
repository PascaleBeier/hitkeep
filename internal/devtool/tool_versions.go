package devtool

import (
	_ "embed"
	"fmt"

	json "hitkeep/jsonapi"
)

// toolVersionsJSON is the single source of truth for development tools which
// are not already pinned by go.mod or the dashboard package metadata.
//
//go:embed tool-versions.json
var toolVersionsJSON []byte

var toolVersions = mustLoadToolVersions()

func ToolVersion(name string) string {
	version, ok := toolVersions[name]
	if !ok {
		panic(fmt.Sprintf("development tool %q has no pinned version", name))
	}
	return version
}

func mustLoadToolVersions() map[string]string {
	versions := map[string]string{}
	if err := json.Unmarshal(toolVersionsJSON, &versions); err != nil {
		panic(fmt.Sprintf("load development tool versions: %v", err))
	}
	for name, version := range versions {
		if name == "" || version == "" {
			panic("development tool names and versions must not be empty")
		}
	}
	return versions
}
