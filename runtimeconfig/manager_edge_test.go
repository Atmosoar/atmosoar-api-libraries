package runtimeconfig

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
)

// brokenStore fails every operation, like a Postgres that is down.
type brokenStore struct{}

var errStoreDown = errors.New("store down")

func (brokenStore) LoadAll(context.Context) (map[string]json.RawMessage, error) {
	return nil, errStoreDown
}
func (brokenStore) Upsert(context.Context, string, json.RawMessage) error { return errStoreDown }
func (brokenStore) Delete(context.Context, string) error                  { return errStoreDown }
func (brokenStore) Kind() string                                          { return "broken" }

func TestLoadAtBootFailOpen(t *testing.T) {
	m := NewManager(testRegistry(), brokenStore{}, zap.NewNop().Sugar())
	if err := m.LoadAtBoot(context.Background()); err != nil {
		t.Fatalf("a store error at boot must not fail startup: %v", err)
	}
	if m.Int("max_results") != 5000 || m.StoreKind() != "broken" {
		t.Fatal("expected registry defaults over the broken store")
	}
}

func TestLoadAtBootDropsInvalidAndUnknownRows(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	_ = store.Upsert(ctx, "max_results", json.RawMessage(`999999999`)) // above max
	_ = store.Upsert(ctx, "gone_key", json.RawMessage(`1`))
	_ = store.Upsert(ctx, "nogo_dbz", json.RawMessage(`42.5`))
	m := NewManager(testRegistry(), store, zap.NewNop().Sugar())
	if err := m.LoadAtBoot(ctx); err != nil {
		t.Fatal(err)
	}
	if m.Int("max_results") != 5000 {
		t.Fatalf("invalid stored row must fall back to default, got %d", m.Int("max_results"))
	}
	if m.Float("nogo_dbz") != 42.5 {
		t.Fatalf("valid stored row must load, got %v", m.Float("nogo_dbz"))
	}
}

func TestClearEdgeCases(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	if err := m.Clear(ctx, "nope"); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("want ErrUnknownKey, got %v", err)
	}
	if err := m.Clear(ctx, "dem_mode"); !errors.Is(err, ErrImmutable) {
		t.Fatalf("want ErrImmutable, got %v", err)
	}
	if err := m.Clear(ctx, "dwd_interval"); err != nil {
		t.Fatalf("restart-required clear: %v", err)
	}
	broken := NewManager(testRegistry(), brokenStore{}, zap.NewNop().Sugar())
	if err := broken.Clear(ctx, "max_results"); !errors.Is(err, errStoreDown) {
		t.Fatalf("want wrapped store error, got %v", err)
	}
}

func TestGettersOnWrongKindOrUnknownKey(t *testing.T) {
	m := newTestManager(t)
	// Each getter asked for a key of another kind returns its zero value.
	if m.Int64("log_level") != 0 || m.Float("log_level") != 0 || m.Bool("max_results") ||
		m.String("max_results") != "" || m.Duration("max_results") != 0 {
		t.Fatal("getters must return zero values on a kind mismatch")
	}
	if m.Int("unregistered") != 0 || m.String("unregistered") != "" {
		t.Fatal("unregistered key must read as zero")
	}
	if !m.Bool("fetch_on_startup") || m.String("log_level") != "info" {
		t.Fatal("defaults for bool/enum")
	}
}

func TestRegisterNormalizesDefaults(t *testing.T) {
	reg := NewRegistry().
		Register(Key{Name: "i64", Kind: KindInt, Default: int64(3)}).
		Register(Key{Name: "if", Kind: KindInt, Default: 4.0}).
		Register(Key{Name: "fi", Kind: KindFloat, Default: 2}).
		Register(Key{Name: "fi64", Kind: KindFloat, Default: int64(2)}).
		Register(Key{Name: "d", Kind: KindDuration, Default: time.Minute}).
		Register(Key{Name: "di", Kind: KindDuration, Default: 5}).
		Register(Key{Name: "di64", Kind: KindDuration, Default: int64(6)}).
		Register(Key{Name: "df", Kind: KindDuration, Default: 1.5})
	m := NewManager(reg, NewMemStore(), zap.NewNop().Sugar())
	if m.Int("i64") != 3 || m.Int("if") != 4 || m.Float("fi") != 2 || m.Float("fi64") != 2 {
		t.Fatal("numeric defaults not normalized")
	}
	if m.Duration("d") != time.Minute || m.Duration("di") != 5*time.Second ||
		m.Duration("di64") != 6*time.Second || m.Duration("df") != 1500*time.Millisecond {
		t.Fatal("duration defaults not normalized to seconds")
	}
}

func TestRegisterPanicsOnBadDefinition(t *testing.T) {
	for name, k := range map[string]Key{
		"empty name":    {Kind: KindInt, Default: 1},
		"int from str":  {Name: "a", Kind: KindInt, Default: "1"},
		"non-integer":   {Name: "a", Kind: KindInt, Default: 1.5},
		"float from b":  {Name: "a", Kind: KindFloat, Default: true},
		"bool from int": {Name: "a", Kind: KindBool, Default: 1},
		"str from int":  {Name: "a", Kind: KindString, Default: 1},
		"dur from str":  {Name: "a", Kind: KindDuration, Default: "1s"},
		"unknown kind":  {Name: "a", Kind: Kind("blob"), Default: 1},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s: want panic", name)
				}
			}()
			NewRegistry().Register(k)
		}()
	}
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate key: want panic")
		}
	}()
	NewRegistry().Register(Key{Name: "a", Kind: KindBool, Default: true}).
		Register(Key{Name: "a", Kind: KindBool, Default: true})
}

func TestValidateRejectsWrongJSONTypes(t *testing.T) {
	reg := testRegistry()
	for _, c := range []struct{ key, raw string }{
		{"max_results", `"1"`},
		{"cache_ttl", `"60s"`},
		{"cache_ttl", `0`}, // below min 1
		{"owm_key", `1`},
		{"log_level", `1`},
	} {
		key, raw := c.key, c.raw
		if _, err := reg.Validate(key, json.RawMessage(raw)); !errors.Is(err, ErrInvalidValue) {
			t.Fatalf("%s=%s: want ErrInvalidValue, got %v", key, raw, err)
		}
	}
	if _, err := reg.Validate("nope", json.RawMessage(`1`)); err == nil {
		t.Fatal("unknown key must fail validation")
	}
}

func TestModuleLoadsOverridesOnStart(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	_ = store.Upsert(ctx, "max_results", json.RawMessage(`77`))
	var m *Manager
	app := fxtest.New(t,
		fx.Supply(testRegistry(), fx.Annotate(store, fx.As(new(Store))), zap.NewNop().Sugar()),
		Module,
		fx.Populate(&m),
	)
	app.RequireStart()
	defer app.RequireStop()
	if m.Int("max_results") != 77 {
		t.Fatalf("OnStart must load persisted overrides, got %d", m.Int("max_results"))
	}
}
