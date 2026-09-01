package devtool

import (
	"os"
	"strings"
	"testing"
)

func TestValidateDefaultTenantMigrationAcceptanceWorkflowIsManualOnly(t *testing.T) {
	if err := validateDefaultTenantMigrationAcceptanceWorkflow([]byte("on:\n  workflow_dispatch:\n")); err != nil {
		t.Fatalf("validateDefaultTenantMigrationAcceptanceWorkflow() error = %v", err)
	}
	if err := validateDefaultTenantMigrationAcceptanceWorkflow([]byte("on:\n  workflow_call:\n")); err == nil {
		t.Fatal("validateDefaultTenantMigrationAcceptanceWorkflow() error = nil, want reusable trigger error")
	}
	if err := validateDefaultTenantMigrationAcceptanceWorkflow([]byte("on:\n  workflow_dispatch:\n  push:\n")); err == nil {
		t.Fatal("validateDefaultTenantMigrationAcceptanceWorkflow() error = nil, want automatic trigger error")
	}
}

func TestValidateReleaseWorkflowGraphRejectsMigrationInterruptionGate(t *testing.T) {
	raw, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := strings.Replace(string(raw), "\n  upgrade-from-supported-floor:\n", `
  manual-migration-check:
    needs:
      - release-please
      - build-release
    uses: ./.github/workflows/default-tenant-migration-acceptance.yml

  upgrade-from-supported-floor:
`, 1)
	if err := validateReleaseWorkflowGraph([]byte(workflow)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() error = nil, want manual migration workflow coupling error")
	}
}
