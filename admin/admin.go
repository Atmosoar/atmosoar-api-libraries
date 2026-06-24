// Package admin provides the reusable admin REST surface every Atmosoar Go
// service exposes for the gateway-hosted admin dashboard. It registers the
// canonical contract (info / config / flags + optional service-specific actions)
// on the service's raw gin engine — deliberately outside the customer OpenAPI
// spec, like /<svc>/metrics — over a runtimeconfig.Manager.
//
// Routes are mounted under a prefix that matches what the gateway forwards 1:1,
// e.g. "/admin/radar-airspace", and are guarded by the shared claims middleware:
// claims.FromHeader (identity-version check) + claims.RequireAdmin (the gateway-
// stamped X-Atmosoar-Admin header). The gateway enforces the api:admin role and
// stamps that header; backends trust the network boundary.
package admin

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"atmosoar.io/atmosoar-api-libraries/claims"
	"atmosoar.io/atmosoar-api-libraries/runtimeconfig"
)

// ContractVersion is the admin REST contract version the SPA negotiates against.
const ContractVersion = "1"

// Endpoint is a service-specific admin action mounted relative to the prefix,
// e.g. {Method: "POST", Path: "/cache/purge", Handler: ...} -> POST
// /admin/<svc>/cache/purge. It runs behind the same admin auth middleware.
type Endpoint struct {
	Method  string
	Path    string
	Handler gin.HandlerFunc
}

// Config wires the admin surface for one service.
type Config struct {
	// Prefix is the route group, matching the gateway forward path, e.g.
	// "/admin/radar-airspace".
	Prefix string
	// Service is the short service name reported by /info, e.g. "radar-airspace".
	Service string
	// Version is the running build version (typically the embedded VERSION).
	Version string
	// Stage is the deployment stage, e.g. "development" / "production".
	Stage string
	// Manager is the runtime config the config endpoints read and mutate.
	Manager *runtimeconfig.Manager
	// Flags is the optional feature-flag set; when nil, /flags returns empty and
	// toggles 404.
	Flags *FlagSet
	// Endpoints are optional service-specific actions.
	Endpoints []Endpoint
	// Logger is required.
	Logger *zap.SugaredLogger
	// BootTime seeds the /info uptime; defaults to registration time when zero.
	BootTime time.Time
}

// Info is the GET /admin/<svc>/info payload: service identity plus the operational
// metadata the SPA needs (notably Store, so operators know whether overrides
// survive a restart).
type Info struct {
	Service         string `json:"service"`
	Version         string `json:"version"`
	Stage           string `json:"stage"`
	Store           string `json:"store"` // "postgres" | "memory"
	Uptime          string `json:"uptime"`
	ContractVersion string `json:"contract_version"`
}

// Register mounts the admin contract on engine under cfg.Prefix, guarded by the
// admin auth middleware. Call it from the service's server constructor next to
// where /<svc>/healthz and /<svc>/metrics are wired.
func Register(engine *gin.Engine, cfg Config) {
	if cfg.BootTime.IsZero() {
		cfg.BootTime = time.Now()
	}
	h := &handlers{cfg: cfg}

	grp := engine.Group(cfg.Prefix)
	grp.Use(claims.FromHeader(cfg.Logger), claims.RequireAdmin(cfg.Logger))

	grp.GET("/info", h.getInfo)
	grp.GET("/config", h.getConfig)
	grp.PUT("/config/:key", h.putConfig)
	grp.DELETE("/config/:key", h.deleteConfig)
	grp.GET("/flags", h.getFlags)
	grp.POST("/flags/:flag", h.toggleFlag)

	for _, e := range cfg.Endpoints {
		grp.Handle(e.Method, e.Path, e.Handler)
	}
}
