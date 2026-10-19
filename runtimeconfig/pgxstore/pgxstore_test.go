package pgxstore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// recordingDB captures the SQL each call sends; Query fails so LoadAll needs
// no fake pgx.Rows.
type recordingDB struct{ sql []string }

func (r *recordingDB) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	r.sql = append(r.sql, sql)
	return nil, errors.New("no rows in test")
}

func (r *recordingDB) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	r.sql = append(r.sql, sql)
	return pgconn.CommandTag{}, nil
}

func TestNewWithTableTargetsTable(t *testing.T) {
	db := &recordingDB{}
	s, err := NewWithTable(db, "feature_flags")
	if err != nil {
		t.Fatalf("NewWithTable: %v", err)
	}
	ctx := context.Background()
	_, _ = s.LoadAll(ctx)
	_ = s.Upsert(ctx, "k", json.RawMessage(`true`))
	_ = s.Delete(ctx, "k")
	if err := EnsureSchemaFor(ctx, db, "feature_flags"); err != nil {
		t.Fatalf("EnsureSchemaFor: %v", err)
	}
	if len(db.sql) != 4 {
		t.Fatalf("want 4 statements, got %d", len(db.sql))
	}
	for _, q := range db.sql {
		if !strings.Contains(q, `"feature_flags"`) || strings.Contains(q, "runtime_config") {
			t.Fatalf("statement does not target feature_flags: %s", q)
		}
	}
}

func TestNewKeepsRuntimeConfig(t *testing.T) {
	db := &recordingDB{}
	_ = New(db).Delete(context.Background(), "k")
	if len(db.sql) != 1 || !strings.Contains(db.sql[0], `"runtime_config"`) {
		t.Fatalf("New must default to runtime_config, got %v", db.sql)
	}
}

func TestNewWithTableRejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{
		"", "Flags", "1flags", "flags;drop table x", `fl"ags`, "public.flags", strings.Repeat("a", 64),
	} {
		if _, err := NewWithTable(&recordingDB{}, name); err == nil {
			t.Fatalf("table %q: want error", name)
		}
		if _, err := CreateTableSQLFor(name); err == nil {
			t.Fatalf("CreateTableSQLFor(%q): want error", name)
		}
	}
}
