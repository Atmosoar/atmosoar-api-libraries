package claims

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"atmosoar.io/atmosoar-api-libraries/httputils"
)

// ContextKey is the gin.Context key under which FromHeader stores the parsed
// *Claims. Preserves the key name used by the pre-extraction per-service mirrors
// so in-tree readers migrate with a single import swap.
const ContextKey = "claims"

// EnvTrustHeaders is the canonical environment variable that toggles trust-header
// enforcement. Set to "false" (case-insensitive) in local dev to run a backend
// directly without the gateway. Default (unset) is enforce.
const EnvTrustHeaders = "ATMOSOAR_TRUST_GATEWAY_HEADERS"

// legacyTrustEnvVars are the per-service env vars that toggled enforcement before
// this package centralized the contract. EnvTrustHeaders wins if set; otherwise
// the first legacy var that is set decides. This lets services adopt the shared
// package without rewriting their manifests in lockstep.
//
//nolint:gochecknoglobals // immutable contract list
var legacyTrustEnvVars = []string{
	"MMA_TRUST_GATEWAY_HEADERS",
	"RADAR_TRUST_GATEWAY_HEADERS",
	"OBSERVATIONS_TRUST_GATEWAY_HEADERS",
	"OBSERVATION_TRUST_GATEWAY_HEADERS",
	"METAR_TRUST_GATEWAY_HEADERS",
	"ELEVATION_TRUST_GATEWAY_HEADERS",
}

// TrustEnabled reports whether trust-header enforcement is on. EnvTrustHeaders
// takes precedence; otherwise the first set legacy var decides; default true.
func TrustEnabled() bool {
	if v, ok := os.LookupEnv(EnvTrustHeaders); ok {
		return !strings.EqualFold(v, "false")
	}
	for _, k := range legacyTrustEnvVars {
		if v, ok := os.LookupEnv(k); ok {
			return !strings.EqualFold(v, "false")
		}
	}
	return true
}

// propagateLogger receives the one-shot warning emitted by PropagateTrustHeaders
// when an east-west call is attempted with no trust headers attached. It is
// process-wide because PropagateTrustHeaders is package-level with no per-request
// DI seam.
//
//nolint:gochecknoglobals // process-wide injectable logger; not request state
var (
	propagateLogger     *zap.SugaredLogger
	propagateWarnedOnce sync.Once
)

// SetPropagateLogger injects the process logger used by PropagateTrustHeaders for
// one-shot misconfiguration warnings. First non-nil wiring wins; later calls are
// ignored.
func SetPropagateLogger(l *zap.SugaredLogger) {
	if propagateLogger == nil {
		propagateLogger = l
	}
}

// Claims is the parsed identity extracted from the gateway-trust headers.
type Claims struct {
	Email string
	Sub   string
	Tier  string
	Roles []string
}

// appRoles is the set of Atmosoar application-level roles surfaced to backends.
//
//nolint:gochecknoglobals // immutable lookup set
var appRoles = map[string]bool{
	"administrator": true,
	"dispatcher":    true,
	"pilot":         true,
}

// GetApplicationRoles returns only the Atmosoar application roles (administrator,
// dispatcher, pilot), discarding Keycloak-internal roles like offline_access.
func (c *Claims) GetApplicationRoles() []string {
	out := make([]string, 0, len(c.Roles))
	for _, r := range c.Roles {
		if appRoles[r] {
			out = append(out, r)
		}
	}
	return out
}

// trustHeadersCtxKey is a private type for the context key under which captured
// trust headers are stored, avoiding collisions with other packages.
type trustHeadersCtxKey struct{}

// WithTrustHeaders returns a new context carrying the given trust headers.
// Exposed mainly for tests; production callers rely on FromHeader.
func WithTrustHeaders(ctx context.Context, h http.Header) context.Context {
	return context.WithValue(ctx, trustHeadersCtxKey{}, h.Clone())
}

// TrustHeadersFromContext returns the captured X-Atmosoar-* trust headers from
// ctx, or nil if none were attached.
func TrustHeadersFromContext(ctx context.Context) http.Header {
	h, _ := ctx.Value(trustHeadersCtxKey{}).(http.Header)
	return h
}

// PropagateTrustHeaders copies the propagated gateway-trust headers (see
// PropagatedTrustHeaders) from ctx onto req. It is the contract for any east-west
// call that re-uses the gateway identity.
func PropagateTrustHeaders(ctx context.Context, req *http.Request) {
	src := TrustHeadersFromContext(ctx)
	if src == nil {
		if propagateLogger != nil {
			propagateWarnedOnce.Do(func() {
				propagateLogger.Warnw(
					"east-west call without gateway trust headers; outbound request will be unauthenticated. If ATMOSOAR_TRUST_GATEWAY_HEADERS=false this is expected in local dev; in production this indicates a misrouted request or middleware misconfiguration",
				)
			})
		}
		return
	}
	for _, name := range PropagatedTrustHeaders() {
		if v := src.Get(name); v != "" {
			req.Header.Set(name, v)
		}
	}
}

// FromHeader returns Gin middleware that validates HeaderIdentityVersion, extracts
// the X-Atmosoar-User-* headers into a *Claims, and stores it on gin.Context under
// ContextKey. It also snapshots the propagated trust headers onto the request
// context for east-west replay.
//
// Edge cases:
//   - missing identity-version header -> 401 AUTHENTICATION_FAILED (caller bypassed
//     the gateway, or the gateway misconfigured itself);
//   - unrecognized identity-version -> 500 INTERNAL_ERROR (gateway/backend on
//     mismatched contract versions; roll both);
//   - trust disabled (see TrustEnabled) -> no-op pass-through for local dev.
func FromHeader(logger *zap.SugaredLogger) gin.HandlerFunc {
	trust := TrustEnabled()

	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			WithTrustHeaders(c.Request.Context(), captureTrustHeaders(c)),
		)

		if !trust {
			c.Next()
			return
		}

		version := c.GetHeader(HeaderIdentityVersion)
		if version == "" {
			logger.Warnw("Gateway identity headers missing",
				"missing_header", HeaderIdentityVersion,
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"clientIP", c.ClientIP())
			httputils.SendAPIError(c, logger,
				http.StatusUnauthorized,
				"AUTHENTICATION_FAILED",
				"Gateway identity headers missing. Requests must originate from the Atmosoar Gateway.")
			c.Abort()
			return
		}
		if version != CurrentIdentityVersion {
			httputils.SendInternalError(c, logger, fmt.Errorf(
				"unsupported %s=%q (expected %q); gateway and backend must be rolled together for contract bumps",
				HeaderIdentityVersion, version, CurrentIdentityVersion))
			c.Abort()
			return
		}

		c.Set(ContextKey, &Claims{
			Email: c.GetHeader(HeaderUserEmail),
			Sub:   c.GetHeader(HeaderUserSub),
			Tier:  c.GetHeader(HeaderUserTier),
			Roles: splitRoles(c.GetHeader(HeaderUserRoles)),
		})
		c.Next()
	}
}

// RequireAdmin returns Gin middleware that rejects requests lacking the
// gateway-stamped HeaderAdmin: AdminHeaderValue. It must run AFTER FromHeader. The
// gateway stamps the header only for api:admin callers and strips any client copy
// at the trust boundary, so its presence is a trustworthy admin signal. When trust
// enforcement is disabled (local dev) this is a pass-through, mirroring FromHeader.
func RequireAdmin(logger *zap.SugaredLogger) gin.HandlerFunc {
	trust := TrustEnabled()

	return func(c *gin.Context) {
		if !trust {
			c.Next()
			return
		}
		if c.GetHeader(HeaderAdmin) != AdminHeaderValue {
			logger.Warnw("admin route accessed without gateway admin header",
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"clientIP", c.ClientIP())
			httputils.SendForbidden(c, logger,
				fmt.Errorf("missing %s; caller lacks api:admin access", HeaderAdmin))
			c.Abort()
			return
		}
		c.Next()
	}
}

// FromContext returns the parsed *Claims stored on the gin context by FromHeader,
// or nil if none is present.
func FromContext(c *gin.Context) *Claims {
	v, ok := c.Get(ContextKey)
	if !ok {
		return nil
	}
	cl, _ := v.(*Claims)
	return cl
}

// captureTrustHeaders snapshots the inbound propagated trust headers off the gin
// request so downstream east-west clients can replay them.
func captureTrustHeaders(c *gin.Context) http.Header {
	h := http.Header{}
	for _, name := range PropagatedTrustHeaders() {
		if v := c.GetHeader(name); v != "" {
			h.Set(name, v)
		}
	}
	return h
}

// splitRoles parses the comma-separated role list the gateway produces from
// strings.Join(claims.GetApplicationRoles(), ",").
func splitRoles(header string) []string {
	if header == "" {
		return nil
	}
	parts := strings.Split(header, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}
