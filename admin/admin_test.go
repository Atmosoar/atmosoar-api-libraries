package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"atmosoar.io/atmosoar-api-libraries/claims"
	"atmosoar.io/atmosoar-api-libraries/runtimeconfig"
)

func init() { gin.SetMode(gin.TestMode) }

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
	if w := do(r, http.MethodPut, "/admin/test/config/max_results", `{"value": 999999}`, true); w.Code != http.StatusBadRequest {
		t.Fatalf("put oob: want 400 got %d", w.Code)
	}
	// immutable -> 409
	if w := do(r, http.MethodPut, "/admin/test/config/dem_mode", `{"value": "flat"}`, true); w.Code != http.StatusConflict {
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
	if w := do(r2, http.MethodPost, "/admin/test/flags/disable_upstream", `{"value":true}`, true); w.Code != http.StatusOK {
		t.Fatalf("toggle known: %d body=%s", w.Code, w.Body.String())
	}
	if !fs.Enabled("disable_upstream") {
		t.Fatal("flag not enabled after toggle")
	}
	if w := do(r2, http.MethodPost, "/admin/test/flags/unknown", `{"value":true}`, true); w.Code != http.StatusNotFound {
		t.Fatalf("toggle unknown: want 404 got %d", w.Code)
	}
}
