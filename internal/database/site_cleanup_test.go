package database

import (
	"bufio"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"hitkeep/internal/database/migrations"
)

func TestSiteDeleteStepsCoverAllSiteTables(t *testing.T) {
	entries, err := migrations.Fs.ReadDir(".")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	siteTables := map[string]struct{}{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		path := filepath.Join(".", entry.Name())
		contents, err := migrations.Fs.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", entry.Name(), err)
		}
		for table := range extractSiteIDTables(string(contents)) {
			siteTables[table] = struct{}{}
		}
	}

	var missing []string
	for table := range siteTables {
		if table == "sites" || isMigrationScratchTable(table) {
			continue
		}
		if _, ok := knownSiteDeleteTables[table]; !ok {
			missing = append(missing, table)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("site delete steps missing tables: %s", strings.Join(missing, ", "))
	}
}

func isMigrationScratchTable(table string) bool {
	return strings.HasSuffix(table, "_without_money_contract")
}

func TestGoalAndFunnelRollupTablesCovered(t *testing.T) {
	entries, err := migrations.Fs.ReadDir(".")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	expectedGoals, expectedFunnels := map[string]struct{}{}, map[string]struct{}{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		path := filepath.Join(".", entry.Name())
		contents, err := migrations.Fs.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", entry.Name(), err)
		}
		for table := range extractSiteIDTables(string(contents)) {
			if strings.HasPrefix(table, "goal_rollups_") {
				expectedGoals[table] = struct{}{}
			}
			if strings.HasPrefix(table, "funnel_rollups_") {
				expectedFunnels[table] = struct{}{}
			}
		}
	}

	assertTablesCovered(t, "goal_rollups", expectedGoals, extractTablesFromDeletes(goalRollupQueries))
	assertTablesCovered(t, "funnel_rollups", expectedFunnels, extractTablesFromDeletes(funnelRollupQueries))
}

func TestSiteTransferTablesCoverAnalyticsCleanupTables(t *testing.T) {
	expected := map[string]struct{}{}
	for _, step := range siteAnalyticsDeleteSteps {
		expected[step.table] = struct{}{}
	}

	assertTablesCovered(t, "site analytics transfer", expected, siteAnalyticsTransferTables)
}

func extractSiteIDTables(sql string) map[string]struct{} {
	createTableRe := regexp.MustCompile(`(?i)^\s*create\s+table\s+(if\s+not\s+exists\s+)?([a-zA-Z0-9_]+)`)
	endTableRe := regexp.MustCompile(`\);`)
	siteIDRe := regexp.MustCompile(`\bsite_id\b`)

	tables := map[string]struct{}{}
	scanner := bufio.NewScanner(strings.NewReader(sql))

	inCreate := false
	tableName := ""
	hasSiteID := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}

		if !inCreate {
			if matches := createTableRe.FindStringSubmatch(trimmed); matches != nil {
				tableName = matches[2]
				inCreate = true
				hasSiteID = false
			}
			continue
		}

		if siteIDRe.MatchString(trimmed) {
			hasSiteID = true
		}

		if endTableRe.MatchString(trimmed) {
			if hasSiteID && tableName != "" {
				tables[tableName] = struct{}{}
			}
			inCreate = false
			tableName = ""
			hasSiteID = false
		}
	}

	if inCreate && hasSiteID && tableName != "" {
		tables[tableName] = struct{}{}
	}

	return tables
}

func assertTablesCovered(t *testing.T, label string, expected map[string]struct{}, actual []string) {
	t.Helper()

	actualSet := map[string]struct{}{}
	for _, table := range actual {
		actualSet[table] = struct{}{}
	}

	var missing []string
	for table := range expected {
		if _, ok := actualSet[table]; !ok {
			missing = append(missing, table)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("%s tables missing from cleanup list: %s", label, strings.Join(missing, ", "))
	}
}

func extractTablesFromDeletes(queries []string) []string {
	tables := make([]string, 0, len(queries))
	for _, query := range queries {
		table := extractTableFromDelete(query)
		if table == "" {
			continue
		}
		tables = append(tables, table)
	}
	return tables
}

func extractTableFromDelete(query string) string {
	const token = "DELETE FROM "
	if !strings.HasPrefix(query, token) {
		return ""
	}
	rest := query[len(token):]
	for i, r := range rest {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return rest[:i]
		}
	}
	return rest
}
