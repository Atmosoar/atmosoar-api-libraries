package claims

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func init() { gin.SetMode(gin.TestMode) }

func testLogger() *zap.SugaredLogger { return zap.NewNop().Sugar() }

// newReq builds a request with the given headers and runs it through the given
// handler chain, returning the recorder.
func runChain(t *testing.T, headers map[string]string, handlers ...gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)
	for _, h := range handlers {
		r.Use(h)
	}
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c.Request = req
	r.ServeHTTP(w, req)
	return w
}

func TestFromHeader(t *testing.T) {
	t.Setenv(EnvTrustHeaders, "true")

	t.Run("missing version -> 401", func(t *testing.T) {
		w := runChain(t, nil, FromHeader(testLogger()))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d", w.Code)
		}
	})

	t.Run("wrong version -> 500", func(t *testing.T) {
		w := runChain(t, map[string]string{HeaderIdentityVersion: "999"}, FromHeader(testLogger()))
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("want 500, got %d", w.Code)
		}
	})

	t.Run("valid -> claims populated", func(t *testing.T) {
		w := httptest.NewRecorder()
		_, r := gin.CreateTestContext(w)
		var got *Claims
		r.Use(FromHeader(testLogger()))
		r.GET("/x", func(c *gin.Context) {
			got = FromContext(c)
			c.Status(http.StatusOK)
		})
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set(HeaderIdentityVersion, CurrentIdentityVersion)
		req.Header.Set(HeaderUserEmail, "op@atmosoar.io")
		req.Header.Set(HeaderUserRoles, "administrator, pilot ,offline_access")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", w.Code)
		}
		if got == nil || got.Email != "op@atmosoar.io" {
			t.Fatalf("claims not populated: %+v", got)
		}
		app := got.GetApplicationRoles()
		if len(app) != 2 || app[0] != "administrator" || app[1] != "pilot" {
			t.Fatalf("app roles filter wrong: %v", app)
		}
	})

	t.Run("trust disabled -> pass-through", func(t *testing.T) {
		t.Setenv(EnvTrustHeaders, "false")
		w := runChain(t, nil, FromHeader(testLogger()))
		if w.Code != http.StatusOK {
			t.Fatalf("want 200 pass-through, got %d", w.Code)
		}
	})
}

func TestRequireAdmin(t *testing.T) {
	t.Setenv(EnvTrustHeaders, "true")

	t.Run("missing admin header -> 403", func(t *testing.T) {
		w := runChain(t, nil, RequireAdmin(testLogger()))
		if w.Code != http.StatusForbidden {
			t.Fatalf("want 403, got %d", w.Code)
		}
	})

	t.Run("wrong admin value -> 403", func(t *testing.T) {
		w := runChain(t, map[string]string{HeaderAdmin: "true"}, RequireAdmin(testLogger()))
		if w.Code != http.StatusForbidden {
			t.Fatalf("want 403, got %d", w.Code)
		}
	})

	t.Run("admin header present -> pass", func(t *testing.T) {
		w := runChain(t, map[string]string{HeaderAdmin: AdminHeaderValue}, RequireAdmin(testLogger()))
		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", w.Code)
		}
	})

	t.Run("trust disabled -> pass-through", func(t *testing.T) {
		t.Setenv(EnvTrustHeaders, "false")
		w := runChain(t, nil, RequireAdmin(testLogger()))
		if w.Code != http.StatusOK {
			t.Fatalf("want 200 pass-through, got %d", w.Code)
		}
	})
}

func TestTrustEnabledPrecedence(t *testing.T) {
	t.Run("canonical wins over legacy", func(t *testing.T) {
		t.Setenv(EnvTrustHeaders, "false")
		t.Setenv("MMA_TRUST_GATEWAY_HEADERS", "true")
		if TrustEnabled() {
			t.Fatal("canonical=false should win")
		}
	})
	t.Run("legacy fallback when canonical unset", func(t *testing.T) {
		t.Setenv("RADAR_TRUST_GATEWAY_HEADERS", "false")
		if TrustEnabled() {
			t.Fatal("legacy=false should disable when canonical unset")
		}
	})
}

func TestAdminHeaderStrippedAtBoundary(t *testing.T) {
	// HeaderAdmin MUST be in AllTrustHeaders (so the gateway strips client copies)
	// and MUST NOT be in PropagatedTrustHeaders (admin is not forwarded east-west).
	found := false
	for _, h := range AllTrustHeaders() {
		if h == HeaderAdmin {
			found = true
		}
	}
	if !found {
		t.Fatal("HeaderAdmin must be in AllTrustHeaders to be stripped at the boundary")
	}
	for _, h := range PropagatedTrustHeaders() {
		if h == HeaderAdmin {
			t.Fatal("HeaderAdmin must NOT be propagated east-west")
		}
	}
}
