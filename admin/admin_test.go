package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"atmosoar.io/atmosoar-api-libraries/claims"
	"atmosoar.io/atmosoar-api-libraries/runtimeconfig"
)

// Package-level setup for a test binary belongs in TestMain; an init() in a
// _test.go file is what gochecknoinits flags.
func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func testManager() *runtimeconfig.Manager {
	reg := runtimeconfig.NewRegistry().
		Register(runtimeconfig.Key{Name: "max_results", Kind: runtimeconfig.KindInt,
			Default: 5000, Mutable: true, Bounds: runtimeconfig.FloatBounds(1, 100000)}).
		Register(runtimeconfig.Key{Name: "dem_mode", Kind: runtimeconfig.KindString,
			Default: "tiered", Mutable: false})
	m := runtimeconfig.NewManager(reg, runtimeconfig.NewMemStore(), zap.NewNop().Sugar())
	return m
}

func setupEngine(t *testing.T, flags *FlagSet) *gin.Engine {
	t.Helper()
	t.Setenv(claims.EnvTrustHeaders, "true")
	r := gin.New()
	Register(r, Config{
		Prefix:   "/admin/test",
		Service:  "test",
		Version:  "9.9.9",
		Stage:    "development",
		Manager:  testManager(),
		Flags:    flags,
		Logger:   zap.NewNop().Sugar(),
		BootTime: time.Now().Add(-90 * time.Second),
	})
	return r
}

func do(r *gin.Engine, method, path, body string, admin bool) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	var rd *strings.Reader
	if body != "" {
		rd = strings.NewReader(body)
	} else {
		rd = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(claims.HeaderIdentityVersion, claims.CurrentIdentityVersion)
	if admin {
		req.Header.Set(claims.HeaderAdmin, claims.AdminHeaderValue)
	}
	r.ServeHTTP(w, req)
	return w
}

func TestAdminAuthGate(t *testing.T) {
	r := setupEngine(t, nil)
	// identity present but no admin header -> 403
	if w := do(r, http.MethodGet, "/admin/test/info", "", false); w.Code != http.StatusForbidden {
		t.Fatalf("want 403 without admin header, got %d", w.Code)
	}
	if w := do(r, http.MethodGet, "/admin/test/info", "", true); w.Code != http.StatusOK {
		t.Fatalf("want 200 with admin header, got %d", w.Code)
	}
}

func TestInfo(t *testing.T) {
	r := setupEngine(t, nil)
	w := do(r, http.MethodGet, "/admin/test/info", "", true)
	var info Info
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info.Service != "test" || info.Version != "9.9.9" || info.Store != "memory" {
		t.Fatalf("info wrong: %+v", info)
	}
	if info.ContractVersion != ContractVersion {
		t.Fatalf("contract version: %s", info.ContractVersion)
	}
}

func TestConfigCRUD(t *testing.T) {
	r := setupEngine(t, nil)

	if w := do(r, http.MethodGet, "/admin/test/config", "", true); w.Code != http.StatusOK {
		t.Fatalf("get config: %d", w.Code)
	}
	// valid apply
	if w := do(r, http.MethodPut, "/admin/test/config/max_results", `{"value": 1234}`, true); w.Code != http.StatusOK {
		t.Fatalf("put valid: %d body=%s", w.Code, w.Body.String())
	}
	// out of bounds -> 400
	if w := do(
		r,
		http.MethodPut,
		"/admin/test/config/max_results",
		`{"value": 999999}`,
		true,
	); w.Code != http.StatusBadRequest {
		t.Fatalf("put oob: want 400 got %d", w.Code)
	}
	// immutable -> 409
	if w := do(
		r,
		http.MethodPut,
		"/admin/test/config/dem_mode",
		`{"value": "flat"}`,
		true,
	); w.Code != http.StatusConflict {
		t.Fatalf("put immutable: want 409 got %d", w.Code)
	}
	// unknown -> 404
	if w := do(r, http.MethodPut, "/admin/test/config/nope", `{"value": 1}`, true); w.Code != http.StatusNotFound {
		t.Fatalf("put unknown: want 404 got %d", w.Code)
	}
	// delete revert
	if w := do(r, http.MethodDelete, "/admin/test/config/max_results", "", true); w.Code != http.StatusOK {
		t.Fatalf("delete: %d", w.Code)
	}
}

func TestFlags(t *testing.T) {
	// no flagset -> empty + toggle 404
	r := setupEngine(t, nil)
	if w := do(r, http.MethodGet, "/admin/test/flags", "", true); w.Code != http.StatusOK {
		t.Fatalf("flags get: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/admin/test/flags/x", `{"value":true}`, true); w.Code != http.StatusNotFound {
		t.Fatalf("toggle without flagset: want 404 got %d", w.Code)
	}

	// with flagset
	fs := NewFlagSet(runtimeconfig.NewMemStore(), zap.NewNop().Sugar(),
		[]Flag{{Name: "disable_upstream", Default: false, Description: "kill switch"}})
	r2 := setupEngine(t, fs)
	if w := do(
		r2,
		http.MethodPost,
		"/admin/test/flags/disable_upstream",
		`{"value":true}`,
		true,
	); w.Code != http.StatusOK {
		t.Fatalf("toggle known: %d body=%s", w.Code, w.Body.String())
	}
	if !fs.Enabled("disable_upstream") {
		t.Fatal("flag not enabled after toggle")
	}
	if w := do(
		r2,
		http.MethodPost,
		"/admin/test/flags/unknown",
		`{"value":true}`,
		true,
	); w.Code != http.StatusNotFound {
		t.Fatalf("toggle unknown: want 404 got %d", w.Code)
	}
}

// TestToggleFlagRequiresValue: a body without "value" used to bind to false
// and switch the flag off with a 200.
func TestToggleFlagRequiresValue(t *testing.T) {
	fs := NewFlagSet(runtimeconfig.NewMemStore(), zap.NewNop().Sugar(),
		[]Flag{{Name: "nowcast_enabled", Default: true}})
	r := setupEngine(t, fs)
	for _, body := range []string{`{}`, `{"enabled":true}`, `{"value":null}`} {
		w := do(r, http.MethodPost, "/admin/test/flags/nowcast_enabled", body, true)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body %s: want 400, got %d", body, w.Code)
		}
		if !fs.Enabled("nowcast_enabled") {
			t.Fatalf("body %s switched the flag off", body)
		}
	}
	w := do(r, http.MethodPost, "/admin/test/flags/nowcast_enabled", `{"value":false}`, true)
	if w.Code != http.StatusOK {
		t.Fatalf("explicit false: want 200, got %d", w.Code)
	}
	if fs.Enabled("nowcast_enabled") {
		t.Fatal("explicit false did not switch the flag off")
	}
}

// brokenStore stands in for an unreachable Postgres.
type brokenStore struct{ *runtimeconfig.MemStore }

func (brokenStore) Upsert(context.Context, string, json.RawMessage) error {
	return errors.New("runtime_config upsert: failed to connect to `host=db.internal user=svc`")
}

// TestPutConfigStoreFailureIs500: a store outage is a 500 with a generic
// message, not a 400 that echoes the database error to the client.
func TestPutConfigStoreFailureIs500(t *testing.T) {
	t.Setenv(claims.EnvTrustHeaders, "true")
	reg := runtimeconfig.NewRegistry().
		Register(runtimeconfig.Key{Name: "max_results", Kind: runtimeconfig.KindInt,
			Default: 5000, Mutable: true, Bounds: runtimeconfig.FloatBounds(1, 100000)})
	m := runtimeconfig.NewManager(reg, brokenStore{runtimeconfig.NewMemStore()}, zap.NewNop().Sugar())
	r := gin.New()
	Register(r, Config{
		Prefix:   "/admin/test",
		Service:  "test",
		Manager:  m,
		Logger:   zap.NewNop().Sugar(),
		BootTime: time.Now(),
	})

	w := do(r, http.MethodPut, "/admin/test/config/max_results", `{"value": 10}`, true)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("store failure: want 500, got %d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "db.internal") {
		t.Fatalf("store error leaked to client: %s", w.Body.String())
	}
	// A validation failure is still a 400 with the reason.
	w = do(r, http.MethodPut, "/admin/test/config/max_results", `{"value": 999999}`, true)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "INVALID_VALUE") {
		t.Fatalf("invalid value: want 400 INVALID_VALUE, got %d body=%s", w.Code, w.Body.String())
	}
}

// yieldingStore widens the window between persist and publish so concurrent
// toggles would interleave their copy-and-store without FlagSet.writeMu.
type yieldingStore struct{ *runtimeconfig.MemStore }

func (s yieldingStore) Upsert(ctx context.Context, key string, value json.RawMessage) error {
	runtime.Gosched()
	return s.MemStore.Upsert(ctx, key, value)
}

func TestConcurrentFlagSetKeepsEveryFlag(t *testing.T) {
	const n = 100
	known := make([]Flag, n)
	for i := range known {
		known[i] = Flag{Name: fmt.Sprintf("f%03d", i)}
	}
	fs := NewFlagSet(yieldingStore{runtimeconfig.NewMemStore()}, zap.NewNop().Sugar(), known)
	ctx := context.Background()
	var wg sync.WaitGroup
	for _, fl := range known {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			if err := fs.Set(ctx, name, true); err != nil {
				t.Errorf("Set %s: %v", name, err)
			}
		}(fl.Name)
	}
	wg.Wait()
	for _, v := range fs.Snapshot() {
		if !v.Value {
			t.Fatalf("%s lost its concurrent toggle", v.Name)
		}
	}
}
