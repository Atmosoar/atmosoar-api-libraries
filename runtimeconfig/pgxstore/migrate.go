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

// CreateTableSQLFor returns the CreateTableSQL DDL for another table name, for
// a store built with NewWithTable. It rejects the same names NewWithTable does.
func CreateTableSQLFor(table string) (string, error) {
	q, err := quoteTable(table)
	if err != nil {
		return "", err
	}
	return `
CREATE TABLE IF NOT EXISTS ` + q + ` (
	key        TEXT PRIMARY KEY,
	value      JSONB NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`, nil
}

// EnsureSchema creates the runtime_config table if it does not exist. It is the
// create-on-connect path for services without a migration tool (e.g. radar). Safe
// to call on every boot.
func EnsureSchema(ctx context.Context, db Querier) error {
	_, err := db.Exec(ctx, CreateTableSQL)
	return err
}

// EnsureSchemaFor is EnsureSchema for a table used with NewWithTable.
func EnsureSchemaFor(ctx context.Context, db Querier, table string) error {
	ddl, err := CreateTableSQLFor(table)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, ddl)
	return err
}
