package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRouteTemplateRoundTrip(t *testing.T) {
	ctx := context.Background()
	if _, ok := RouteTemplate(ctx); ok {
		t.Fatal("a bare context must carry no template")
	}
	if WithRouteTemplate(ctx, "") != ctx {
		t.Fatal("an empty template must leave the context unchanged")
	}
	got, ok := RouteTemplate(WithRouteTemplate(ctx, "/forecast/v1"))
	if !ok || got != "/forecast/v1" {
		t.Fatalf("RouteTemplate = %q, %v", got, ok)
	}
}

// A NoRoute handler that stores a template gets it as the path label; one that
// does not still counts as /unknown, and a Gin-matched route keeps FullPath.
func TestMetricsMiddlewareUsesRouteTemplateForUnmatchedRoutes(t *testing.T) {
	_ = RegisterDefaultCollectors()
	r := gin.New()
	r.Use(HTTPMetricsMiddleware(""))
	r.GET("/native/:id", func(c *gin.Context) {
		c.Request = c.Request.WithContext(WithRouteTemplate(c.Request.Context(), "/ignored"))
		c.Status(http.StatusOK)
	})
	r.NoRoute(func(c *gin.Context) {
		if c.Request.URL.Path == "/proxied/abc123" {
			c.Request = c.Request.WithContext(WithRouteTemplate(c.Request.Context(), "/proxied/:id"))
			c.Status(http.StatusAccepted)
			return
		}
		c.Status(http.StatusNotFound)
	})

	proxied := HTTPRequestsTotal.WithLabelValues("GET", "/proxied/:id", "202")
	unknown := HTTPRequestsTotal.WithLabelValues("GET", "/unknown", "404")
	native := HTTPRequestsTotal.WithLabelValues("GET", "/native/:id", "200")
	beforeProxied, beforeUnknown, beforeNative := counterValue(t, proxied), counterValue(t, unknown),
		counterValue(t, native)
	beforeRaw := counterValue(t, HTTPRequestsTotal.WithLabelValues("GET", "/proxied/abc123", "202"))

	for _, p := range []string{"/proxied/abc123", "/nothing/here", "/native/7"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, p, nil))
	}

	if got := counterValue(t, proxied); got != beforeProxied+1 {
		t.Fatalf("template label: got %v want %v", got, beforeProxied+1)
	}
	if got := counterValue(t, unknown); got != beforeUnknown+1 {
		t.Fatalf("no template must stay /unknown: got %v want %v", got, beforeUnknown+1)
	}
	if got := counterValue(t, native); got != beforeNative+1 {
		t.Fatalf("Gin's FullPath must win over a stored template: got %v want %v", got, beforeNative+1)
	}
	if got := counterValue(t, HTTPRequestsTotal.WithLabelValues("GET", "/proxied/abc123", "202")); got != beforeRaw {
		t.Fatal("the raw request path must never become a label")
	}
}
