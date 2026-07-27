package controlstore

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// fkEdge is one child→parent foreign key relationship: rows in table whose
// column matches referencedColumn of referencedTable. Edges are discovered
// from SQLite's foreign-key pragmas or registered manually for relationships
// the schema does not declare.
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

// listFKEdges returns every SQLite foreign-key edge in the connected control
// database using the live table_list/foreign_key_list catalog surfaces.
func listFKEdges(ctx context.Context, q queryer) ([]fkEdge, error) {
	tables, err := listTables(ctx, q)
	if err != nil {
		return nil, err
	}
	var edges []fkEdge
	for table := range tables {
		tableEdges, err := listTableFKEdges(ctx, q, table)
		if err != nil {
			return nil, err
		}
		edges = append(edges, tableEdges...)
	}
	return edges, nil
}

func listTableFKEdges(ctx context.Context, q queryer, table string) ([]fkEdge, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, seq, "table", "from", "to", on_update, on_delete, "match"
		FROM pragma_foreign_key_list(?)
		ORDER BY id, seq
	`, table)
	if err != nil {
		return nil, fmt.Errorf("could not list foreign keys for %s: %w", table, err)
	}
	defer rows.Close()

	var edges []fkEdge
	for rows.Next() {
		var id, seq int
		var referencedTable, column, referencedColumn, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &referencedTable, &column, &referencedColumn, &onUpdate, &onDelete, &match); err != nil {
			return nil, fmt.Errorf("could not scan foreign key for %s: %w", table, err)
		}
		edges = append(edges, fkEdge{table: table, column: column, referencedTable: referencedTable, referencedColumn: referencedColumn})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read foreign keys for %s: %w", table, err)
	}
	return edges, nil
}

// listScopedTables maps each existing table that carries one of the scope
// columns to the matching column name.
func listScopedTables(ctx context.Context, q queryer, scopeColumns []string) (map[string]string, error) {
	tables, err := listTables(ctx, q)
	if err != nil {
		return nil, err
	}
	scoped := make(map[string]string)
	for table := range tables {
		columns, err := listVisibleTableColumns(ctx, q, table)
		if err != nil {
			return nil, err
		}
		for _, column := range columns {
			if slices.Contains(scopeColumns, column) {
				if _, exists := scoped[table]; !exists {
					scoped[table] = column
				}
			}
		}
	}
	return scoped, nil
}

func listVisibleTableColumns(ctx context.Context, q queryer, table string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT name FROM pragma_table_xinfo(?) WHERE hidden = 0 ORDER BY cid`, table)
	if err != nil {
		return nil, fmt.Errorf("could not list columns for %s: %w", table, err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return nil, fmt.Errorf("could not scan column for %s: %w", table, err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read columns for %s: %w", table, err)
	}
	return columns, nil
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
