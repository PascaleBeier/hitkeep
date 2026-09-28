package admin

import (
	"os"
	"strings"
	"testing"
)

func TestGlobalExclusionRoutesUseNarrowInstancePermission(t *testing.T) {
	source, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatalf("read handlers.go: %v", err)
	}

	for _, route := range []string{
		`GET /api/admin/exclusions`,
		`POST /api/admin/exclusions`,
		`DELETE /api/admin/exclusions/{ruleID}`,
	} {
		index := strings.Index(string(source), route)
		if index < 0 {
			t.Fatalf("route %q not found", route)
		}
		block := string(source[index:min(index+260, len(source))])
		if !strings.Contains(block, "PermInstanceManageSiteExclusions") {
			t.Fatalf("route %q must require PermInstanceManageSiteExclusions, block was:\n%s", route, block)
		}
		if strings.Contains(block, "PermInstanceViewAllSites") {
			t.Fatalf("route %q must not use broad PermInstanceViewAllSites, block was:\n%s", route, block)
		}
	}
}
