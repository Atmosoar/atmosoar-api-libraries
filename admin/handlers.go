package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"atmosoar.io/atmosoar-api-libraries/httputils"
	"atmosoar.io/atmosoar-api-libraries/runtimeconfig"
)

type handlers struct {
	cfg Config
}

func (h *handlers) getInfo(c *gin.Context) {
	c.JSON(http.StatusOK, Info{
		Service:         h.cfg.Service,
		Version:         h.cfg.Version,
		Stage:           h.cfg.Stage,
		Store:           h.cfg.Manager.StoreKind(),
		Uptime:          time.Since(h.cfg.BootTime).Round(time.Second).String(),
		ContractVersion: ContractVersion,
	})
}

func (h *handlers) getConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"config": h.cfg.Manager.MergedView()})
}

// putConfigBody is the PUT /config/:key request body: {"value": <json>}.
type putConfigBody struct {
	Value json.RawMessage `json:"value"`
}

func (h *handlers) putConfig(c *gin.Context) {
	key := c.Param("key")
	var body putConfigBody
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Value) == 0 {
		httputils.SendAPIError(c, h.cfg.Logger, http.StatusBadRequest,
			"INVALID_REQUEST", "request body must be {\"value\": <value>}")
		return
	}
	switch err := h.cfg.Manager.Apply(c.Request.Context(), key, body.Value); {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"ok": true, "key": key})
	case errors.Is(err, runtimeconfig.ErrUnknownKey):
		httputils.SendNotFound(c, h.cfg.Logger)
	case errors.Is(err, runtimeconfig.ErrImmutable):
		httputils.SendAPIError(c, h.cfg.Logger, http.StatusConflict,
			"IMMUTABLE", "config key is not mutable at runtime")
	default:
		// Validation failures (bounds/enum/kind) are client errors; surface the
		// reason. A persistence failure is also reported here as a 400 with the
		// underlying message — acceptable since the value was not applied.
		httputils.SendAPIError(c, h.cfg.Logger, http.StatusBadRequest,
			"INVALID_VALUE", err.Error())
	}
}

func (h *handlers) deleteConfig(c *gin.Context) {
	key := c.Param("key")
	switch err := h.cfg.Manager.Clear(c.Request.Context(), key); {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"ok": true, "key": key})
	case errors.Is(err, runtimeconfig.ErrUnknownKey):
		httputils.SendNotFound(c, h.cfg.Logger)
	case errors.Is(err, runtimeconfig.ErrImmutable):
		httputils.SendAPIError(c, h.cfg.Logger, http.StatusConflict,
			"IMMUTABLE", "config key is not mutable at runtime")
	default:
		httputils.SendInternalError(c, h.cfg.Logger, err)
	}
}

func (h *handlers) getFlags(c *gin.Context) {
	if h.cfg.Flags == nil {
		c.JSON(http.StatusOK, gin.H{"flags": []any{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"flags": h.cfg.Flags.Snapshot()})
}

// toggleFlagBody is the POST /flags/:flag request body: {"value": <bool>}.
type toggleFlagBody struct {
	Value bool `json:"value"`
}

func (h *handlers) toggleFlag(c *gin.Context) {
	if h.cfg.Flags == nil {
		httputils.SendNotFound(c, h.cfg.Logger)
		return
	}
	flag := c.Param("flag")
	var body toggleFlagBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httputils.SendAPIError(c, h.cfg.Logger, http.StatusBadRequest,
			"INVALID_REQUEST", "request body must be {\"value\": <bool>}")
		return
	}
	if err := h.cfg.Flags.Set(c.Request.Context(), flag, body.Value); err != nil {
		if errors.Is(err, ErrUnknownFlag) {
			httputils.SendNotFound(c, h.cfg.Logger)
			return
		}
		httputils.SendInternalError(c, h.cfg.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "flag": flag, "value": body.Value})
}
