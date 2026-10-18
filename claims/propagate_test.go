package claims

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestPropagateTrustHeadersCopiesOnlyTrustHeaders(t *testing.T) {
	src := http.Header{}
	src.Set(HeaderUserEmail, "pilot@example.com")
	src.Set(HeaderUserSub, "sub-1")
	src.Set(HeaderIdentityVersion, CurrentIdentityVersion)
	src.Set(HeaderAdmin, AdminHeaderValue) // not propagated east-west
	src.Set("Authorization", "Bearer x")
	ctx := WithTrustHeaders(context.Background(), src)

	// WithTrustHeaders stores a clone: later edits to src do not leak in.
	src.Set(HeaderUserSub, "changed")

	req := httptest.NewRequest(http.MethodGet, "http://svc/x", nil)
	PropagateTrustHeaders(ctx, req)
	if req.Header.Get(HeaderUserEmail) != "pilot@example.com" || req.Header.Get(HeaderUserSub) != "sub-1" ||
		req.Header.Get(HeaderIdentityVersion) != CurrentIdentityVersion {
		t.Fatalf("trust headers not copied: %v", req.Header)
	}
	if req.Header.Get(HeaderAdmin) != "" || req.Header.Get("Authorization") != "" {
		t.Fatalf("non-propagated headers copied: %v", req.Header)
	}
	if req.Header.Get(HeaderUserTier) != "" {
		t.Fatal("an absent header must stay absent")
	}
}

func TestPropagateTrustHeadersWithoutCaptureWarnsOnce(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	prev := propagateLogger
	propagateLogger = nil
	t.Cleanup(func() { propagateLogger = prev })

	SetPropagateLogger(zap.New(core).Sugar())
	SetPropagateLogger(zap.NewNop().Sugar()) // first non-nil wiring wins

	if TrustHeadersFromContext(context.Background()) != nil {
		t.Fatal("bare context must carry no trust headers")
	}
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "http://svc/x", nil)
		PropagateTrustHeaders(context.Background(), req)
		if len(req.Header) != 0 {
			t.Fatalf("no headers expected, got %v", req.Header)
		}
	}
	// propagateWarnedOnce is process-wide; another test may have spent it.
	if n := logs.Len(); n > 1 {
		t.Fatalf("misconfiguration warning must fire at most once, got %d", n)
	}
}
