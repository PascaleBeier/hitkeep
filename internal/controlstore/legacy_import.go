package controlstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const DuckDBControlImportV1 = "duckdb_control_to_sqlite_v1"

// LegacyRows is the streaming subset of sql.Rows needed by the pure-Go
// importer. A DuckDB adapter implements this interface outside controlstore.
type LegacyRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error
}

type LegacyColumn struct {
	Name        string
	LogicalType string
}

// LegacySource abstracts the frozen DuckDB importer without introducing a
// DuckDB or CGO dependency into this package.
type LegacySource interface {
	BaseTables(context.Context) ([]string, error)
	Columns(context.Context, string) ([]LegacyColumn, error)
	Rows(context.Context, string, []LegacyColumn) (LegacyRows, error)
	RowCount(context.Context, string) (int64, error)
}

// TableFingerprint is a sanitized, order-independent full-row comparison.
// It contains no source values and is safe for private-fixture result reports.
type TableFingerprint struct {
	Rows   int64
	SHA256 string
}

// LegacyImportResult records exact source/target comparisons for copied
// control tables.
type LegacyImportResult struct {
	Tables map[string]TableFingerprint
}

// ImportLegacy creates a new SQLite work file and copies a fully split,
// control-only DuckDB source into it. It never overwrites an existing path.
func ImportLegacy(
	ctx context.Context,
	workPath string,
	source LegacySource,
	sourceSHA256 string,
	sourceSchemaSHA256 string,
) (LegacyImportResult, error) {
	if source == nil {
		return LegacyImportResult{}, errors.New("legacy control source is nil")
	}
	if _, err := os.Stat(workPath); err == nil {
		return LegacyImportResult{}, fmt.Errorf("SQLite conversion work file %q already exists", workPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return LegacyImportResult{}, fmt.Errorf("inspect SQLite conversion work file: %w", err)
	}
	if err := validateDigest("source checksum", sourceSHA256); err != nil {
		return LegacyImportResult{}, err
	}
	if err := validateDigest("source schema digest", sourceSchemaSHA256); err != nil {
		return LegacyImportResult{}, err
	}

	live, err := source.BaseTables(ctx)
	if err != nil {
		return LegacyImportResult{}, fmt.Errorf("list legacy control tables: %w", err)
	}
	classification, err := ClassifyLegacyTables(live)
	if err != nil {
		return LegacyImportResult{}, err
	}
	for _, table := range legacyEmptyAnalyticsTables {
		count, err := source.RowCount(ctx, table)
		if err != nil {
			return LegacyImportResult{}, fmt.Errorf("count discarded analytics table %s: %w", table, err)
		}
		if count != 0 {
			return LegacyImportResult{}, fmt.Errorf("legacy analytics compatibility table %s contains %d rows; default-tenant split is incomplete", table, count)
		}
	}
	for table, disposition := range classification {
		if disposition == 0 {
			return LegacyImportResult{}, fmt.Errorf("legacy table %s has no disposition", table)
		}
	}

	target, err := Open(ctx, workPath)
	if err != nil {
		return LegacyImportResult{}, err
	}
	cleanup := true
	defer func() {
		_ = target.Close()
		if cleanup {
			_ = os.Remove(workPath)
			_ = os.Remove(workPath + "-wal")
			_ = os.Remove(workPath + "-shm")
		}
	}()

	tx, err := target.db.BeginTx(ctx, nil)
	if err != nil {
		return LegacyImportResult{}, fmt.Errorf("begin SQLite control import: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	result := LegacyImportResult{Tables: make(map[string]TableFingerprint, len(legacyControlTables))}
	for _, table := range legacyControlTables {
		columns, err := source.Columns(ctx, table)
		if err != nil {
			return LegacyImportResult{}, fmt.Errorf("read source columns for %s: %w", table, err)
		}
		targetColumns, err := sqliteTableColumns(ctx, tx, table)
		if err != nil {
			return LegacyImportResult{}, err
		}
		columnNames := legacyColumnNames(columns)
		if !equalStrings(columnNames, targetColumns) {
			return LegacyImportResult{}, fmt.Errorf("control table %s column mismatch: source=%v target=%v", table, columnNames, targetColumns)
		}

		fingerprint, err := copyLegacyTable(ctx, tx, source, table, columns)
		if err != nil {
			return LegacyImportResult{}, err
		}
		targetFingerprint, err := sqliteTableFingerprint(ctx, tx, table, columnNames)
		if err != nil {
			return LegacyImportResult{}, err
		}
		if fingerprint != targetFingerprint {
			differing, detailErr := differingTableColumns(ctx, tx, source, table, columns)
			if detailErr != nil {
				return LegacyImportResult{}, fmt.Errorf("control table %s verification mismatch: source=%+v target=%+v (column diagnostics: %v)", table, fingerprint, targetFingerprint, detailErr)
			}
			return LegacyImportResult{}, fmt.Errorf("control table %s verification mismatch in columns %v: source=%+v target=%+v", table, differing, fingerprint, targetFingerprint)
		}
		result.Tables[table] = fingerprint
	}

	var splitMarkers int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*) FROM data_migrations
		WHERE name IN ('default_tenant_split_v1', 'default_tenant_split_compacted_v1')
	`).Scan(&splitMarkers); err != nil {
		return LegacyImportResult{}, fmt.Errorf("verify default-tenant split markers: %w", err)
	}
	if splitMarkers != 2 {
		return LegacyImportResult{}, fmt.Errorf("default-tenant split markers are incomplete: found %d of 2", splitMarkers)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO control_imports(name, source_sha256, source_schema_sha256, imported_at)
		VALUES (?, ?, ?, ?)
	`, DuckDBControlImportV1, sourceSHA256, sourceSchemaSHA256, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return LegacyImportResult{}, fmt.Errorf("record DuckDB control import: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return LegacyImportResult{}, fmt.Errorf("commit SQLite control import: %w", err)
	}
	committed = true

	if err := target.validate(ctx); err != nil {
		return LegacyImportResult{}, err
	}
	if err := target.Close(); err != nil {
		return LegacyImportResult{}, err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(workPath + suffix); err == nil {
			return LegacyImportResult{}, fmt.Errorf("SQLite conversion left %s sidecar after checkpoint", suffix)
		} else if !errors.Is(err, os.ErrNotExist) {
			return LegacyImportResult{}, fmt.Errorf("inspect SQLite conversion sidecar: %w", err)
		}
	}
	cleanup = false
	return result, nil
}

func differingTableColumns(ctx context.Context, tx *sql.Tx, source LegacySource, table string, columns []LegacyColumn) ([]string, error) {
	sourceRows, err := source.Rows(ctx, table, columns)
	if err != nil {
		return nil, err
	}
	sourceDigests, err := columnDigests(sourceRows, columns)
	if err != nil {
		return nil, err
	}
	quoted := make([]string, len(columns))
	for i, column := range columns {
		quoted[i] = quoteIdentifier(column.Name)
	}
	targetRows, err := tx.QueryContext(ctx, "SELECT "+strings.Join(quoted, ",")+" FROM "+quoteIdentifier(table))
	if err != nil {
		return nil, err
	}
	targetDigests, err := columnDigests(targetRows, make([]LegacyColumn, len(columns)))
	if err != nil {
		return nil, err
	}
	var differing []string
	for i := range columns {
		if sourceDigests[i] != targetDigests[i] {
			differing = append(differing, fmt.Sprintf("%s(source=%s,target=%s)", columns[i].Name, sourceDigests[i], targetDigests[i]))
		}
	}
	return differing, nil
}

func columnDigests(rows LegacyRows, columns []LegacyColumn) ([]string, error) {
	width := len(columns)
	defer rows.Close()
	components := make([][][sha256.Size]byte, width)
	for rows.Next() {
		values := make([]any, width)
		dest := make([]any, width)
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		for i, value := range values {
			_, tag, body, err := canonicalColumnValue(value, columns[i])
			if err != nil {
				return nil, err
			}
			h := sha256.New()
			h.Write([]byte{tag})
			h.Write(body)
			var digest [sha256.Size]byte
			copy(digest[:], h.Sum(nil))
			components[i] = append(components[i], digest)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]string, width)
	for i := range components {
		result[i] = finishFingerprint(components[i]).SHA256
	}
	return result, nil
}

func validateDigest(label, value string) error {
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("%s must be a lowercase SHA-256 digest", label)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || hex.EncodeToString(decoded) != value {
		return fmt.Errorf("%s must be a lowercase SHA-256 digest", label)
	}
	return nil
}

func sqliteTableColumns(ctx context.Context, queryer rowQueryer, table string) ([]string, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT cid, name, type, "notnull", dflt_value, pk, hidden FROM pragma_table_xinfo(?) ORDER BY cid`, table)
	if err != nil {
		return nil, fmt.Errorf("read SQLite columns for %s: %w", table, err)
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, primaryKey, hidden int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &primaryKey, &hidden); err != nil {
			return nil, fmt.Errorf("scan SQLite column for %s: %w", table, err)
		}
		if hidden == 0 {
			columns = append(columns, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate SQLite columns for %s: %w", table, err)
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("SQLite control table %s does not exist", table)
	}
	return columns, nil
}

func copyLegacyTable(ctx context.Context, tx *sql.Tx, source LegacySource, table string, columns []LegacyColumn) (TableFingerprint, error) {
	rows, err := source.Rows(ctx, table, columns)
	if err != nil {
		return TableFingerprint{}, fmt.Errorf("read legacy control table %s: %w", table, err)
	}
	defer rows.Close()

	quotedColumns := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	for i, column := range columns {
		quotedColumns[i] = quoteIdentifier(column.Name)
		placeholders[i] = "?"
	}
	statement := "INSERT INTO " + quoteIdentifier(table) + "(" + strings.Join(quotedColumns, ",") + ") VALUES (" + strings.Join(placeholders, ",") + ")"
	prepared, err := tx.PrepareContext(ctx, statement)
	if err != nil {
		return TableFingerprint{}, fmt.Errorf("prepare SQLite import for %s: %w", table, err)
	}
	defer prepared.Close()

	var rowDigests [][sha256.Size]byte
	for rows.Next() {
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return TableFingerprint{}, fmt.Errorf("scan legacy control table %s: %w", table, err)
		}
		canonicalValues, digest, err := canonicalRow(values, columns)
		if err != nil {
			return TableFingerprint{}, fmt.Errorf("canonicalize legacy control table %s: %w", table, err)
		}
		if _, err := prepared.ExecContext(ctx, canonicalValues...); err != nil {
			return TableFingerprint{}, fmt.Errorf("insert SQLite control table %s: %w", table, err)
		}
		rowDigests = append(rowDigests, digest)
	}
	if err := rows.Err(); err != nil {
		return TableFingerprint{}, fmt.Errorf("iterate legacy control table %s: %w", table, err)
	}
	return finishFingerprint(rowDigests), nil
}

type rowQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// CatalogFingerprint returns sanitized, order-independent full-row
// fingerprints for every SQLite control table. It exposes no row values and
// is suitable for backup and private-fixture verification.
func (s *Store) CatalogFingerprint(ctx context.Context) (map[string]TableFingerprint, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("control database is not open")
	}
	tableSet, err := listTables(ctx, s.db)
	if err != nil {
		return nil, err
	}
	tables := make([]string, 0, len(tableSet))
	for table := range tableSet {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	result := make(map[string]TableFingerprint, len(tables))
	for _, table := range tables {
		columns, err := sqliteTableColumns(ctx, s.db, table)
		if err != nil {
			return nil, err
		}
		fingerprint, err := sqliteTableFingerprint(ctx, s.db, table, columns)
		if err != nil {
			return nil, err
		}
		result[table] = fingerprint
	}
	return result, nil
}

func sqliteTableFingerprint(ctx context.Context, queryer rowQueryer, table string, columns []string) (TableFingerprint, error) {
	quoted := make([]string, len(columns))
	for i, column := range columns {
		quoted[i] = quoteIdentifier(column)
	}
	rows, err := queryer.QueryContext(ctx, "SELECT "+strings.Join(quoted, ",")+" FROM "+quoteIdentifier(table))
	if err != nil {
		return TableFingerprint{}, fmt.Errorf("read imported SQLite table %s: %w", table, err)
	}
	defer rows.Close()
	var rowDigests [][sha256.Size]byte
	for rows.Next() {
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return TableFingerprint{}, fmt.Errorf("scan imported SQLite table %s: %w", table, err)
		}
		_, digest, err := canonicalRow(values, nil)
		if err != nil {
			return TableFingerprint{}, fmt.Errorf("canonicalize imported SQLite table %s: %w", table, err)
		}
		rowDigests = append(rowDigests, digest)
	}
	if err := rows.Err(); err != nil {
		return TableFingerprint{}, fmt.Errorf("iterate imported SQLite table %s: %w", table, err)
	}
	return finishFingerprint(rowDigests), nil
}

func canonicalRow(values []any, columns []LegacyColumn) ([]any, [sha256.Size]byte, error) {
	h := sha256.New()
	canonical := make([]any, len(values))
	for i, value := range values {
		column := LegacyColumn{}
		if len(columns) == len(values) {
			column = columns[i]
		}
		normalized, tag, body, err := canonicalColumnValue(value, column)
		if err != nil {
			return nil, [sha256.Size]byte{}, err
		}
		canonical[i] = normalized
		h.Write([]byte{tag})
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(body)))
		h.Write(length[:])
		h.Write(body)
	}
	var digest [sha256.Size]byte
	copy(digest[:], h.Sum(nil))
	return canonical, digest, nil
}

func canonicalColumnValue(value any, column LegacyColumn) (normalized any, tag byte, body []byte, err error) {
	if strings.EqualFold(strings.TrimSpace(column.LogicalType), "UUID") {
		switch typed := value.(type) {
		case nil:
			return nil, 'n', nil, nil
		case uuid.UUID:
			text := typed.String()
			return text, 's', []byte(text), nil
		case []byte:
			parsed, parseErr := uuid.FromBytes(typed)
			if parseErr != nil {
				return nil, 0, nil, fmt.Errorf("decode UUID column %s: %w", column.Name, parseErr)
			}
			text := parsed.String()
			return text, 's', []byte(text), nil
		case string:
			parsed, parseErr := uuid.Parse(typed)
			if parseErr != nil {
				return nil, 0, nil, fmt.Errorf("parse UUID column %s: %w", column.Name, parseErr)
			}
			text := parsed.String()
			return text, 's', []byte(text), nil
		}
	}
	return canonicalValue(value)
}

func legacyColumnNames(columns []LegacyColumn) []string {
	names := make([]string, len(columns))
	for i, column := range columns {
		names[i] = column.Name
	}
	return names
}

func canonicalValue(value any) (normalized any, tag byte, body []byte, err error) {
	switch value := value.(type) {
	case nil:
		return nil, 'n', nil, nil
	case uuid.UUID:
		text := value.String()
		return text, 's', []byte(text), nil
	case time.Time:
		text := value.UTC().Format(time.RFC3339Nano)
		return text, 't', []byte(text), nil
	case bool:
		if value {
			return canonicalSigned(1)
		}
		return canonicalSigned(0)
	case int:
		return canonicalSigned(int64(value))
	case int8:
		return canonicalSigned(int64(value))
	case int16:
		return canonicalSigned(int64(value))
	case int32:
		return canonicalSigned(int64(value))
	case int64:
		return canonicalSigned(value)
	case uint:
		return canonicalUnsigned(uint64(value))
	case uint8:
		return canonicalUnsigned(uint64(value))
	case uint16:
		return canonicalUnsigned(uint64(value))
	case uint32:
		return canonicalUnsigned(uint64(value))
	case uint64:
		return canonicalUnsigned(value)
	case float32:
		return canonicalFloat(float64(value))
	case float64:
		return canonicalFloat(value)
	case string:
		return value, 's', []byte(value), nil
	case []byte:
		clone := make([]byte, len(value))
		copy(clone, value)
		return clone, 'x', clone, nil
	case map[string]any, []any:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, 0, nil, fmt.Errorf("encode JSON value: %w", err)
		}
		text := string(encoded)
		return text, 's', encoded, nil
	default:
		if valuer, ok := value.(interface{ Value() (any, error) }); ok {
			converted, err := valuer.Value()
			if err != nil {
				return nil, 0, nil, err
			}
			return canonicalValue(converted)
		}
		return nil, 0, nil, fmt.Errorf("unsupported database value %T", value)
	}
}

func canonicalSigned(value int64) (any, byte, []byte, error) {
	text := strconv.FormatInt(value, 10)
	return value, 'i', []byte(text), nil
}

func canonicalUnsigned(value uint64) (any, byte, []byte, error) {
	if value > math.MaxInt64 {
		return nil, 0, nil, fmt.Errorf("unsigned integer %d exceeds SQLite INTEGER", value)
	}
	return canonicalSigned(int64(value))
}

func canonicalFloat(value float64) (any, byte, []byte, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, 0, nil, fmt.Errorf("non-finite float %v", value)
	}
	text := strconv.FormatFloat(value, 'g', -1, 64)
	return value, 'f', []byte(text), nil
}

func finishFingerprint(rows [][sha256.Size]byte) TableFingerprint {
	sort.Slice(rows, func(i, j int) bool { return string(rows[i][:]) < string(rows[j][:]) })
	h := sha256.New()
	for _, row := range rows {
		h.Write(row[:])
	}
	return TableFingerprint{Rows: int64(len(rows)), SHA256: hex.EncodeToString(h.Sum(nil))}
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
