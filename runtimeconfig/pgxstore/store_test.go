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

// fakeRows serves canned (key, value) rows. Scan reuses one value buffer per
// call, as pgx does, so LoadAll must copy what it keeps.
type fakeRows struct {
	rows    [][2]string
	i       int
	buf     []byte
	scanErr error
	err     error
	closed  bool
}

func (r *fakeRows) Close()                                       { r.closed = true }
func (r *fakeRows) Err() error                                   { return r.err }
func (r *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeRows) Values() ([]any, error)                       { return nil, nil }
func (r *fakeRows) RawValues() [][]byte                          { return nil }
func (r *fakeRows) Conn() *pgx.Conn                              { return nil }

func (r *fakeRows) Next() bool {
	if r.i >= len(r.rows) {
		return false
	}
	r.i++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	row := r.rows[r.i-1]
	*dest[0].(*string) = row[0]
	r.buf = append(r.buf[:0], row[1]...)
	*dest[1].(*[]byte) = r.buf
	return nil
}

type call struct {
	sql  string
	args []any
}

// fakeDB returns rows from Query and err from both Query and Exec.
type fakeDB struct {
	rows  *fakeRows
	err   error
	calls []call
}

func (d *fakeDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	d.calls = append(d.calls, call{sql, args})
	if d.err != nil {
		return nil, d.err
	}
	return d.rows, nil
}

func (d *fakeDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	d.calls = append(d.calls, call{sql, args})
	return pgconn.CommandTag{}, d.err
}

func TestLoadAllCopiesEachValue(t *testing.T) {
	rows := &fakeRows{rows: [][2]string{{"a", `1`}, {"b", `"two"`}}}
	got, err := New(&fakeDB{rows: rows}).LoadAll(context.Background())
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if string(got["a"]) != `1` || string(got["b"]) != `"two"` {
		t.Fatalf("values aliased the scan buffer: %q", got)
	}
	if !rows.closed {
		t.Fatal("rows not closed")
	}
}

func TestLoadAllWrapsErrors(t *testing.T) {
	boom := errors.New("boom")
	for name, db := range map[string]*fakeDB{
		"select": {err: boom},
		"scan":   {rows: &fakeRows{rows: [][2]string{{"a", `1`}}, scanErr: boom}},
		"rows":   {rows: &fakeRows{err: boom}},
	} {
		_, err := New(db).LoadAll(context.Background())
		if !errors.Is(err, boom) || !strings.Contains(err.Error(), "runtime_config "+name) {
			t.Fatalf("%s: want wrapped boom naming the step, got %v", name, err)
		}
	}
}

func TestUpsertSendsKeyAndJSONText(t *testing.T) {
	db := &fakeDB{}
	if err := New(db).Upsert(context.Background(), "max_points", json.RawMessage(`42`)); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	c := db.calls[0]
	if !strings.Contains(c.sql, "$2::jsonb") || !strings.Contains(c.sql, "ON CONFLICT (key)") {
		t.Fatalf("unexpected upsert SQL: %s", c.sql)
	}
	// The value goes as text so the ::jsonb cast parses it; []byte would be bytea.
	if len(c.args) != 2 || c.args[0] != "max_points" || c.args[1] != "42" {
		t.Fatalf("unexpected args: %#v", c.args)
	}
}

func TestWriteErrorsAreWrapped(t *testing.T) {
	boom := errors.New("boom")
	s := New(&fakeDB{err: boom})
	ctx := context.Background()
	if err := s.Upsert(
		ctx,
		"k",
		json.RawMessage(`1`),
	); !errors.Is(err, boom) ||
		!strings.Contains(err.Error(), "upsert k") {
		t.Fatalf("Upsert: %v", err)
	}
	if err := s.Delete(ctx, "k"); !errors.Is(err, boom) || !strings.Contains(err.Error(), "delete k") {
		t.Fatalf("Delete: %v", err)
	}
	if err := EnsureSchema(ctx, &fakeDB{err: boom}); !errors.Is(err, boom) {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if err := EnsureSchemaFor(ctx, &fakeDB{}, "Bad"); err == nil {
		t.Fatal("EnsureSchemaFor must reject an invalid table name")
	}
}

func TestEnsureSchemaRunsDDL(t *testing.T) {
	db := &fakeDB{}
	if err := EnsureSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if len(db.calls) != 1 || db.calls[0].sql != CreateTableSQL {
		t.Fatalf("unexpected DDL calls: %#v", db.calls)
	}
	if New(db).Kind() != "postgres" {
		t.Fatal("Kind must be postgres")
	}
}
