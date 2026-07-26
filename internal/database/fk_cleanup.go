package database

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// fkEdge is one child→parent foreign key relationship: rows in table whose
// column matches referencedColumn of referencedTable. Edges are discovered
// from the live schema via duckdb_constraints(), or registered manually for
// relationships the schema does not declare (e.g. site_import_files.import_id
// → site_imports.id, which only has an index).
type fkEdge struct {
	table            string
	column           string
	referencedTable  string
	referencedColumn string
}

// scopedDeleteSpec describes how to derive a delete plan for everything that
// belongs to one row of rootTable.
type scopedDeleteSpec struct {
	// scopeColumns are the column names that mark a table's rows as owned by
	// the scope (e.g. ["site_id"], or ["tenant_id", "team_id"] where both
	// names denote the same entity).
	scopeColumns []string
	rootTable    string
	// extraEdges registers child→parent relationships that carry no FOREIGN
	// KEY constraint in the schema.
	extraEdges []fkEdge
	// policyTables reference the root through a foreign key but are cleaned
	// up by dedicated policy code (e.g. rows that are nulled instead of
	// deleted); the plan skips them instead of failing the coverage check.
	policyTables []string
}

// scopedDeleteStep is one DELETE statement of a scoped delete plan. Every
// query takes the scope ID as its single argument.
type scopedDeleteStep struct {
	table string
	query string
}

// listFKEdges returns every single-column FOREIGN KEY edge in the connected
// database. Multi-column foreign keys are rejected so schema changes that
// would silently break scoped delete plans fail loudly instead.
func listFKEdges(ctx context.Context, q queryer) ([]fkEdge, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT table_name, constraint_column_names, referenced_table, referenced_column_names
		FROM duckdb_constraints()
		WHERE constraint_type = 'FOREIGN KEY'
			AND schema_name NOT IN ('information_schema', 'pg_catalog')
	`)
	if err != nil {
		return nil, fmt.Errorf("could not list foreign key constraints: %w", err)
	}
	defer rows.Close()

	var edges []fkEdge
	for rows.Next() {
		var table, referencedTable string
		var columns, referencedColumns any
		if err := rows.Scan(&table, &columns, &referencedTable, &referencedColumns); err != nil {
			return nil, fmt.Errorf("could not scan foreign key constraint: %w", err)
		}
		column, okColumn := singleListElement(columns)
		referencedColumn, okReferenced := singleListElement(referencedColumns)
		if !okColumn || !okReferenced {
			return nil, fmt.Errorf("unsupported multi-column foreign key on table %s", table)
		}
		edges = append(edges, fkEdge{
			table:            table,
			column:           column,
			referencedTable:  referencedTable,
			referencedColumn: referencedColumn,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list foreign key constraints: %w", err)
	}
	return edges, nil
}

// singleListElement unwraps a one-element DuckDB LIST value scanned as any.
func singleListElement(value any) (string, bool) {
	switch v := value.(type) {
	case []any:
		if len(v) != 1 {
			return "", false
		}
		s, ok := v[0].(string)
		return s, ok
	case []string:
		if len(v) != 1 {
			return "", false
		}
		return v[0], true
	case string:
		return v, v != ""
	default:
		return "", false
	}
}

// listScopedTables maps each existing table that carries one of the scope
// columns to the matching column name.
func listScopedTables(ctx context.Context, q queryer, scopeColumns []string) (map[string]string, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(scopeColumns)), ", ")
	args := make([]any, 0, len(scopeColumns))
	for _, column := range scopeColumns {
		args = append(args, column)
	}

	// #nosec G201 -- placeholders holds only "?" markers; values are bound parameters.
	rows, err := q.QueryContext(ctx, fmt.Sprintf(`
		SELECT table_name, column_name
		FROM information_schema.columns
		WHERE column_name IN (%s)
			AND table_schema NOT IN ('information_schema', 'pg_catalog')
	`, placeholders), args...)
	if err != nil {
		return nil, fmt.Errorf("could not list scope tables: %w", err)
	}
	defer rows.Close()

	scoped := make(map[string]string)
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			return nil, fmt.Errorf("could not scan scope table: %w", err)
		}
		if _, ok := scoped[table]; !ok {
			scoped[table] = column
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list scope tables: %w", err)
	}
	return scoped, nil
}

// buildScopedDeletePlan derives the ordered DELETE statements that remove all
// rows belonging to one row of spec.rootTable from the connected database.
// The plan is discovered from the live schema on every call: tables carrying
// a scope column are deleted directly, tables reachable only through a
// (declared or registered) foreign key are deleted through IN-subqueries on
// their parent, and foreign-key children always precede their parents. The
// root row itself is not part of the plan. Tables that reference the root
// without a scope column must be listed in extraEdges or policyTables;
// otherwise the plan fails so new schema additions surface immediately.
func buildScopedDeletePlan(ctx context.Context, q queryer, spec scopedDeleteSpec) ([]scopedDeleteStep, error) {
	tables, err := listTables(ctx, q)
	if err != nil {
		return nil, err
	}
	scoped, err := listScopedTables(ctx, q, spec.scopeColumns)
	if err != nil {
		return nil, err
	}
	delete(scoped, spec.rootTable)
	// information_schema.columns also lists view columns; only base tables
	// can be deleted from.
	for table := range scoped {
		if _, ok := tables[table]; !ok {
			delete(scoped, table)
		}
	}

	edges, err := listFKEdges(ctx, q)
	if err != nil {
		return nil, err
	}
	if err := appendExistingExtraEdges(tables, &edges, spec.extraEdges); err != nil {
		return nil, err
	}

	policy := make(map[string]struct{}, len(spec.policyTables))
	for _, table := range spec.policyTables {
		policy[table] = struct{}{}
	}

	for table, column := range scoped {
		if !isSafeIdentifier(table) || !isSafeIdentifier(column) {
			return nil, fmt.Errorf("unsafe identifier in scoped table %s.%s", table, column)
		}
	}
	for _, edge := range edges {
		if !isSafeIdentifier(edge.table) || !isSafeIdentifier(edge.column) || !isSafeIdentifier(edge.referencedTable) || !isSafeIdentifier(edge.referencedColumn) {
			return nil, fmt.Errorf("unsafe identifier in foreign key %s.%s -> %s.%s", edge.table, edge.column, edge.referencedTable, edge.referencedColumn)
		}
	}

	// Every declared FK child of the root must be deletable through the plan,
	// or the root row delete would fail with a dangling reference.
	for _, edge := range edges {
		if edge.referencedTable != spec.rootTable || edge.table == spec.rootTable {
			continue
		}
		if _, ok := scoped[edge.table]; ok {
			continue
		}
		if _, ok := policy[edge.table]; ok {
			continue
		}
		return nil, fmt.Errorf(
			"table %s references %s but has no scope column (%s); add a scope column, an extra edge, or register it as a policy table",
			edge.table, spec.rootTable, strings.Join(spec.scopeColumns, ", "),
		)
	}

	// predicates maps each member table to the WHERE clauses (one scope ID
	// argument each) that select its scope-owned rows.
	predicates := make(map[string][]string, len(scoped))
	for table, column := range scoped {
		predicates[table] = []string{fmt.Sprintf("%s = ?", column)}
	}

	// Pull in tables reachable only through foreign keys on member tables
	// until the membership stabilizes.
	for range tables {
		changed := false
		for _, edge := range sortedEdges(edges) {
			parentPredicates, parentIsMember := predicates[edge.referencedTable]
			if !parentIsMember || edge.table == spec.rootTable {
				continue
			}
			if _, isScoped := scoped[edge.table]; isScoped {
				continue
			}
			if _, isPolicy := policy[edge.table]; isPolicy {
				continue
			}
			for _, parentPredicate := range parentPredicates {
				derived := fmt.Sprintf("%s IN (SELECT %s FROM %s WHERE %s)", edge.column, edge.referencedColumn, edge.referencedTable, parentPredicate)
				if slices.Contains(predicates[edge.table], derived) {
					continue
				}
				predicates[edge.table] = append(predicates[edge.table], derived)
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	members := make(map[string]struct{}, len(predicates))
	for table := range predicates {
		members[table] = struct{}{}
	}
	order, err := orderChildrenFirst(members, edges)
	if err != nil {
		return nil, err
	}

	steps := make([]scopedDeleteStep, 0, len(order))
	for _, table := range order {
		for _, predicate := range predicates[table] {
			// #nosec G201 -- identifiers are validated via isSafeIdentifier and discovered from the schema.
			steps = append(steps, scopedDeleteStep{
				table: table,
				query: fmt.Sprintf("DELETE FROM %s WHERE %s", table, predicate),
			})
		}
	}
	return steps, nil
}

func sortedEdges(edges []fkEdge) []fkEdge {
	sorted := slices.Clone(edges)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].table != sorted[j].table {
			return sorted[i].table < sorted[j].table
		}
		return sorted[i].referencedTable < sorted[j].referencedTable
	})
	return sorted
}

// validateCleanupPlans builds each spec's delete plan against the connected
// schema and reports the first failure. It runs after migrations so schema
// mistakes (e.g. a new table referencing sites without a site_id column)
// fail at startup instead of at the first delete.
func validateCleanupPlans(ctx context.Context, q queryer, specs ...scopedDeleteSpec) error {
	for _, spec := range specs {
		if _, err := buildScopedDeletePlan(ctx, q, spec); err != nil {
			return fmt.Errorf("%s cleanup plan is invalid: %w", spec.rootTable, err)
		}
	}
	return nil
}

// listScopedCopyTables returns the base tables carrying scopeColumn that
// exist in both the source and destination schemas, ordered so foreign-key
// parents come before their children (safe insert order for copies). The
// root table is excluded; callers mirror it explicitly.
func listScopedCopyTables(ctx context.Context, source, destination queryer, scopeColumn, rootTable string, extraEdges []fkEdge) ([]string, error) {
	sourceTables, err := listTables(ctx, source)
	if err != nil {
		return nil, err
	}
	sourceScoped, err := listScopedTables(ctx, source, []string{scopeColumn})
	if err != nil {
		return nil, err
	}
	destinationTables, err := listTables(ctx, destination)
	if err != nil {
		return nil, err
	}
	destinationScoped, err := listScopedTables(ctx, destination, []string{scopeColumn})
	if err != nil {
		return nil, err
	}

	members := make(map[string]struct{})
	for table := range sourceScoped {
		if table == rootTable {
			continue
		}
		if !isSafeIdentifier(table) {
			return nil, fmt.Errorf("unsafe identifier in scoped table %s", table)
		}
		if _, ok := sourceTables[table]; !ok {
			continue
		}
		if _, ok := destinationScoped[table]; !ok {
			continue
		}
		if _, ok := destinationTables[table]; !ok {
			continue
		}
		members[table] = struct{}{}
	}

	edges, err := listFKEdges(ctx, destination)
	if err != nil {
		return nil, err
	}
	if err := appendExistingExtraEdges(destinationTables, &edges, extraEdges); err != nil {
		return nil, err
	}
	// A copied table whose foreign key points outside the copy set (and not
	// at the mirrored root) would land rows without their parents. Fail
	// loudly instead of silently skipping or violating the constraint.
	for _, edge := range edges {
		if _, ok := members[edge.table]; !ok {
			continue
		}
		if edge.referencedTable == rootTable {
			continue
		}
		if _, ok := members[edge.referencedTable]; !ok {
			// Tenant-local schemas intentionally omit the control-plane tenants
			// table. Its IDs remain valid when rows are copied back to the
			// control store, so treat that parent as shared identity only when it
			// is absent from the source catalog. Full shared-to-shared copies
			// remain rejected below rather than silently crossing tenant scope.
			if edge.referencedTable == "tenants" {
				if _, sourceHasTenants := sourceTables[edge.referencedTable]; !sourceHasTenants {
					continue
				}
			}
			return nil, fmt.Errorf("cannot derive a safe copy plan: %s references %s, which is not copied", edge.table, edge.referencedTable)
		}
	}

	childrenFirst, err := orderChildrenFirst(members, edges)
	if err != nil {
		return nil, err
	}
	slices.Reverse(childrenFirst)
	return childrenFirst, nil
}

func appendExistingExtraEdges(tables map[string]struct{}, edges *[]fkEdge, extraEdges []fkEdge) error {
	for _, edge := range extraEdges {
		if !isSafeIdentifier(edge.table) || !isSafeIdentifier(edge.column) || !isSafeIdentifier(edge.referencedTable) || !isSafeIdentifier(edge.referencedColumn) {
			return fmt.Errorf("unsafe identifier in extra foreign key %s.%s -> %s.%s", edge.table, edge.column, edge.referencedTable, edge.referencedColumn)
		}
		if _, ok := tables[edge.table]; !ok {
			continue
		}
		if _, ok := tables[edge.referencedTable]; !ok {
			continue
		}
		*edges = append(*edges, edge)
	}
	return nil
}

// orderChildrenFirst returns the member tables ordered so every foreign-key
// child is deleted before its parent.
func orderChildrenFirst(members map[string]struct{}, edges []fkEdge) ([]string, error) {
	// pendingChildren counts, per member parent, how many member children
	// still hold rows that reference it.
	pendingChildren := make(map[string]int, len(members))
	childrenOf := make(map[string][]string, len(members))
	for table := range members {
		pendingChildren[table] = 0
	}
	for _, edge := range edges {
		if edge.table == edge.referencedTable {
			continue
		}
		if _, ok := members[edge.table]; !ok {
			continue
		}
		if _, ok := members[edge.referencedTable]; !ok {
			continue
		}
		pendingChildren[edge.referencedTable]++
		childrenOf[edge.table] = append(childrenOf[edge.table], edge.referencedTable)
	}

	ready := make([]string, 0, len(members))
	for table, pending := range pendingChildren {
		if pending == 0 {
			ready = append(ready, table)
		}
	}
	sort.Strings(ready)

	order := make([]string, 0, len(members))
	for len(ready) > 0 {
		table := ready[0]
		ready = ready[1:]
		order = append(order, table)
		released := childrenOf[table]
		sort.Strings(released)
		for _, parent := range released {
			pendingChildren[parent]--
			if pendingChildren[parent] == 0 {
				ready = append(ready, parent)
			}
		}
		sort.Strings(ready)
	}
	if len(order) != len(members) {
		remaining := make([]string, 0)
		for table, pending := range pendingChildren {
			if pending > 0 {
				remaining = append(remaining, table)
			}
		}
		sort.Strings(remaining)
		return nil, fmt.Errorf("foreign key cycle among tables: %s", strings.Join(remaining, ", "))
	}
	return order, nil
}
