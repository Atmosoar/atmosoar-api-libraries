// Package pgxstore is the Postgres-backed runtimeconfig.Store, used by services
// that already own a SQL database (radar, observation, metar). It is a separate
// sub-package so DB-less consumers (gateway, MMA, elevation) that import only
// runtimeconfig never link pgx into their binary.
//
// The schema is a single key/value table ported verbatim from the forecast
// ingestor's proven runtime_config table. New uses runtime_config; NewWithTable
// points a store at another table of the same shape, e.g. feature_flags for an
// admin.FlagSet, so flags and config keys never share rows.
package pgxstore

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Querier is the minimal database surface pgxstore needs; *pgxpool.Pool satisfies
// it, so callers pass their existing pool with no adapter.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// DefaultTable is the table New uses.
const DefaultTable = "runtime_config"

// tableNameRE restricts table names to plain lower-case identifiers. The name
// is interpolated into SQL (identifiers cannot be bind parameters), so it is
// validated here and quoted with pgx.Identifier as well.
var tableNameRE = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// Store implements runtimeconfig.Store over a Postgres key/value table.
type Store struct {
	db    Querier
	table string // unquoted, for error messages

	selectSQL string
	upsertSQL string
	deleteSQL string
}

// New returns a Postgres-backed store over db's runtime_config table.
func New(db Querier) *Store {
	s, err := NewWithTable(db, DefaultTable)
	if err != nil {
		panic(err) // DefaultTable is a valid identifier
	}
	return s
}

// NewWithTable returns a store over the named table, which must have the
// CreateTableSQLFor(table) shape. Use a dedicated table such as
// "feature_flags" for an admin.FlagSet so it does not share rows with the
// runtimeconfig Manager. table must match ^[a-z_][a-z0-9_]*$ (at most 63
// characters).
func NewWithTable(db Querier, table string) (*Store, error) {
	q, err := quoteTable(table)
	if err != nil {
		return nil, err
	}
	return &Store{
		db:        db,
		table:     table,
		selectSQL: `SELECT key, value FROM ` + q,
		upsertSQL: `
		INSERT INTO ` + q + ` (key, value, updated_at)
		VALUES ($1, $2::jsonb, now())
		ON CONFLICT (key) DO UPDATE SET value = $2::jsonb, updated_at = now()`,
		deleteSQL: `DELETE FROM ` + q + ` WHERE key = $1`,
	}, nil
}

// quoteTable validates table and returns it as a quoted SQL identifier.
func quoteTable(table string) (string, error) {
	if !tableNameRE.MatchString(table) {
		return "", fmt.Errorf("pgxstore: invalid table name %q", table)
	}
	return pgx.Identifier{table}.Sanitize(), nil
}

// LoadAll returns every persisted override.
func (s *Store) LoadAll(ctx context.Context) (map[string]json.RawMessage, error) {
	rows, err := s.db.Query(ctx, s.selectSQL)
	if err != nil {
		return nil, fmt.Errorf("%s select: %w", s.table, err)
	}
	defer rows.Close()

	out := make(map[string]json.RawMessage)
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("%s scan: %w", s.table, err)
		}
		cp := make(json.RawMessage, len(value))
		copy(cp, value)
		out[key] = cp
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", s.table, err)
	}
	return out, nil
}

// Upsert persists value under key. The ::jsonb cast keeps the JSON value typed
// correctly regardless of pgx's []byte/string encoding default.
func (s *Store) Upsert(ctx context.Context, key string, value json.RawMessage) error {
	_, err := s.db.Exec(ctx, s.upsertSQL, key, string(value))
	if err != nil {
		return fmt.Errorf("%s upsert %s: %w", s.table, key, err)
	}
	return nil
}

// Delete removes the override for key.
func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.db.Exec(ctx, s.deleteSQL, key)
	if err != nil {
		return fmt.Errorf("%s delete %s: %w", s.table, key, err)
	}
	return nil
}

// Kind reports "postgres".
func (s *Store) Kind() string { return "postgres" }
