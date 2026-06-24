package runtimeconfig

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func testRegistry() *Registry {
	return NewRegistry().
		Register(Key{Name: "cache_ttl", Kind: KindDuration, Default: 60 * time.Second,
			Mutable: true, Unit: "seconds", Bounds: FloatBounds(1, 3600)}).
		Register(Key{Name: "max_results", Kind: KindInt, Default: 5000,
			Mutable: true, Bounds: FloatBounds(1, 100000)}).
		Register(Key{Name: "nogo_dbz", Kind: KindFloat, Default: 50.0,
			Mutable: true, Bounds: FloatBounds(0, 95)}).
		Register(Key{Name: "fetch_on_startup", Kind: KindBool, Default: true, Mutable: true}).
		Register(Key{Name: "log_level", Kind: KindEnum, Default: "info", Mutable: true,
			Enum: []string{"debug", "info", "warn", "error"}}).
		Register(Key{Name: "owm_key", Kind: KindString, Default: "", Mutable: true, Secret: true}).
		Register(Key{Name: "dwd_interval", Kind: KindDuration, Default: 600 * time.Second,
			Mutable: true, RestartRequired: true, Bounds: FloatBounds(60, 86400)}).
		Register(Key{Name: "dem_mode", Kind: KindString, Default: "tiered", Mutable: false})
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m := NewManager(testRegistry(), NewMemStore(), zap.NewNop().Sugar())
	if err := m.LoadAtBoot(context.Background()); err != nil {
		t.Fatalf("LoadAtBoot: %v", err)
	}
	return m
}

func TestGettersDefaults(t *testing.T) {
	m := newTestManager(t)
	if got := m.Duration("cache_ttl"); got != 60*time.Second {
		t.Fatalf("cache_ttl default: %v", got)
	}
	if got := m.Int("max_results"); got != 5000 {
		t.Fatalf("max_results default: %d", got)
	}
	if got := m.Float("nogo_dbz"); got != 50.0 {
		t.Fatalf("nogo_dbz default: %v", got)
	}
	if !m.Bool("fetch_on_startup") {
		t.Fatal("fetch_on_startup default")
	}
	if got := m.String("log_level"); got != "info" {
		t.Fatalf("log_level default: %q", got)
	}
}

func TestApplyAndClear(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	if err := m.Apply(ctx, "max_results", json.RawMessage(`1234`)); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := m.Int("max_results"); got != 1234 {
		t.Fatalf("after apply: %d", got)
	}
	// duration is seconds in JSON
	if err := m.Apply(ctx, "cache_ttl", json.RawMessage(`120`)); err != nil {
		t.Fatalf("apply duration: %v", err)
	}
	if got := m.Duration("cache_ttl"); got != 120*time.Second {
		t.Fatalf("after apply duration: %v", got)
	}

	if err := m.Clear(ctx, "max_results"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got := m.Int("max_results"); got != 5000 {
		t.Fatalf("after clear should revert to default: %d", got)
	}
}

func TestValidationRejects(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	cases := map[string]json.RawMessage{
		"out of bounds high": json.RawMessage(`200000`), // max_results max 100000
		"non-integer int":    json.RawMessage(`12.5`),
	}
	for name, raw := range cases {
		if err := m.Apply(ctx, "max_results", raw); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
	if err := m.Apply(ctx, "log_level", json.RawMessage(`"trace"`)); err == nil {
		t.Fatal("enum: expected rejection of out-of-set value")
	}
	if err := m.Apply(ctx, "nogo_dbz", json.RawMessage(`120`)); err == nil {
		t.Fatal("float bounds: expected rejection above max")
	}
}

func TestImmutableAndUnknown(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	if err := m.Apply(ctx, "dem_mode", json.RawMessage(`"flat"`)); err != ErrImmutable {
		t.Fatalf("expected ErrImmutable, got %v", err)
	}
	if err := m.Apply(ctx, "nope", json.RawMessage(`1`)); err != ErrUnknownKey {
		t.Fatalf("expected ErrUnknownKey, got %v", err)
	}
}

func TestRestartRequiredNotHotSwapped(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	// dwd_interval is RestartRequired: persisted but live value unchanged.
	if err := m.Apply(ctx, "dwd_interval", json.RawMessage(`1200`)); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := m.Duration("dwd_interval"); got != 600*time.Second {
		t.Fatalf("restart-required should not hot-swap; got %v", got)
	}
	// ...but a fresh Manager loading the same store sees the persisted value.
	store := NewMemStore()
	m2 := NewManager(testRegistry(), store, zap.NewNop().Sugar())
	_ = m2.LoadAtBoot(ctx)
	_ = m2.Apply(ctx, "dwd_interval", json.RawMessage(`1200`))
	m3 := NewManager(testRegistry(), store, zap.NewNop().Sugar())
	_ = m3.LoadAtBoot(ctx)
	if got := m3.Duration("dwd_interval"); got != 1200*time.Second {
		t.Fatalf("after restart should pick up persisted value; got %v", got)
	}
}

func TestMergedViewRedactsSecret(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	_ = m.Apply(ctx, "owm_key", json.RawMessage(`"super-secret-key"`))
	for _, v := range m.MergedView() {
		if v.Key == "owm_key" {
			if v.Value == "super-secret-key" {
				t.Fatal("secret value leaked in merged view")
			}
			if v.Source != "override" {
				t.Fatalf("expected override source, got %s", v.Source)
			}
		}
		if v.Key == "cache_ttl" {
			// duration rendered as seconds number
			if f, ok := v.Value.(float64); !ok || f != 60 {
				t.Fatalf("cache_ttl value should be 60 seconds, got %v", v.Value)
			}
		}
	}
}

func TestConcurrentReadDuringSwap(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = m.Int("max_results")
					_ = m.Duration("cache_ttl")
				}
			}
		}()
	}
	for i := 0; i < 200; i++ {
		_ = m.Apply(ctx, "max_results", json.RawMessage(`777`))
		_ = m.Clear(ctx, "max_results")
	}
	close(stop)
	wg.Wait()
}
