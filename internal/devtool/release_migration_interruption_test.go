package devtool

import (
	"os"
	"strings"
	"testing"
)

func TestValidateDefaultTenantMigrationAcceptanceWorkflowRequiresCallableTrigger(t *testing.T) {
	raw, err := os.ReadFile("../../.github/workflows/default-tenant-migration-acceptance.yml")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDefaultTenantMigrationAcceptanceWorkflow([]byte(strings.Replace(string(raw), "workflow_call:", "# workflow_call:", 1))); err == nil {
		t.Fatal("validateDefaultTenantMigrationAcceptanceWorkflow() error = nil, want callable trigger error")
	}
}

func TestValidateReleaseWorkflowGraphRequiresMigrationInterruptionGate(t *testing.T) {
	raw, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		workflow string
	}{
		{
			name:     "missing release job",
			workflow: strings.Replace(string(raw), "migration-interruption:", "# migration-interruption:", 1),
		},
		{
			name: "missing finalizer dependency",
			workflow: func() string {
				const dependency = "\n      - migration-interruption\n"
				index := strings.LastIndex(string(raw), dependency)
				return string(raw[:index]) + "\n" + string(raw[index+len(dependency):])
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateReleaseWorkflowGraph([]byte(test.workflow)); err == nil {
				t.Fatal("validateReleaseWorkflowGraph() error = nil, want migration-interruption contract error")
			}
		})
	}
}
