package devtool

import "testing"

func TestRejectDeveloperDependencies(t *testing.T) {
	if err := rejectDeveloperDependencies("self-hosted", "standard/library\nhitkeep/internal/server\n"); err != nil {
		t.Fatalf("runtime-only dependency graph was rejected: %v", err)
	}
	for _, dependency := range []string{
		"hitkeep/internal/devtool",
		"hitkeep/internal/devtool/cli",
		"hitkeep/internal/devtool/devmcp",
	} {
		if err := rejectDeveloperDependencies("cloud", dependency+"\n"); err == nil {
			t.Fatalf("developer dependency %q was accepted", dependency)
		}
	}
}
