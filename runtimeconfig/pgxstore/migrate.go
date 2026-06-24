package pgxstore

import "context"

// CreateTableSQL is the idempotent DDL for the runtime_config table, ported from
// the forecast ingestor. Services with a versioned migrations directory should
// add it as a numbered migration instead of calling EnsureSchema.
const CreateTableSQL = `
CREATE TABLE IF NOT EXISTS runtime_config (
	key        TEXT PRIMARY KEY,
	value      JSONB NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`

// EnsureSchema creates the runtime_config table if it does not exist. It is the
// create-on-connect path for services without a migration tool (e.g. radar). Safe
// to call on every boot.
func EnsureSchema(ctx context.Context, db Querier) error {
	_, err := db.Exec(ctx, CreateTableSQL)
	return err
}
