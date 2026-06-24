// Package pgxstore is the Postgres-backed runtimeconfig.Store, used by services
// that already own a SQL database (radar, observation, metar). It is a separate
// sub-package so DB-less consumers (gateway, MMA, elevation) that import only
// runtimeconfig never link pgx into their binary.
//
// The schema is a single key/value table ported verbatim from the forecast
// ingestor's proven runtime_config table.
package pgxstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Querier is the minimal database surface pgxstore needs; *pgxpool.Pool satisfies
// it, so callers pass their existing pool with no adapter.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Store implements runtimeconfig.Store over a Postgres runtime_config table.
type Store struct {
	db Querier
}

// New returns a Postgres-backed store over db.
func New(db Querier) *Store { return &Store{db: db} }

// LoadAll returns every persisted override.
func (s *Store) LoadAll(ctx context.Context) (map[string]json.RawMessage, error) {
	rows, err := s.db.Query(ctx, `SELECT key, value FROM runtime_config`)
	if err != nil {
		return nil, fmt.Errorf("runtime_config select: %w", err)
	}
	defer rows.Close()

	out := make(map[string]json.RawMessage)
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("runtime_config scan: %w", err)
		}
		cp := make(json.RawMessage, len(value))
		copy(cp, value)
		out[key] = cp
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("runtime_config rows: %w", err)
	}
	return out, nil
}

// Upsert persists value under key. The ::jsonb cast keeps the JSON value typed
// correctly regardless of pgx's []byte/string encoding default.
func (s *Store) Upsert(ctx context.Context, key string, value json.RawMessage) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO runtime_config (key, value, updated_at)
		VALUES ($1, $2::jsonb, now())
		ON CONFLICT (key) DO UPDATE SET value = $2::jsonb, updated_at = now()`,
		key, string(value))
	if err != nil {
		return fmt.Errorf("runtime_config upsert %s: %w", key, err)
	}
	return nil
}

// Delete removes the override for key.
func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM runtime_config WHERE key = $1`, key)
	if err != nil {
		return fmt.Errorf("runtime_config delete %s: %w", key, err)
	}
	return nil
}

// Kind reports "postgres".
func (s *Store) Kind() string { return "postgres" }
