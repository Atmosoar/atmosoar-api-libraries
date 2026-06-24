package runtimeconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// ErrImmutable is returned by Apply/Clear for a key whose Mutable flag is false.
var ErrImmutable = fmt.Errorf("config key is not mutable")

// ErrUnknownKey is returned for a name not present in the registry.
var ErrUnknownKey = fmt.Errorf("unknown config key")

// Manager holds the live config overlay and mediates reads (typed getters) and
// writes (Apply/Clear). It is the runtime-config analogue of the gateway
// Dispatcher: one atomic.Pointer swapped wholesale, lock-free reads.
type Manager struct {
	reg    *Registry
	store  Store
	logger *zap.SugaredLogger
	state  atomic.Pointer[Overlay]
}

// NewManager builds a Manager and publishes an empty overlay so getters are safe
// before LoadAtBoot runs. LoadAtBoot (wired by Module's OnStart) then replaces it
// with the persisted overrides.
func NewManager(reg *Registry, store Store, logger *zap.SugaredLogger) *Manager {
	m := &Manager{reg: reg, store: store, logger: logger}
	m.state.Store(newOverlay())
	return m
}

// LoadAtBoot reads every persisted override, validates each against the registry,
// and publishes the initial overlay. It is fail-open: a store error or an
// individual bad/unknown stored value is logged and skipped, never fatal — the
// service boots on registry defaults (matching the ingestor's behavior when the
// DB is unreachable at startup).
func (m *Manager) LoadAtBoot(ctx context.Context) error {
	stored, err := m.store.LoadAll(ctx)
	if err != nil {
		m.logger.Warnw("runtimeconfig: load at boot failed; using defaults",
			"store", m.store.Kind(), "error", err)
		return nil
	}
	next := newOverlay()
	for name, raw := range stored {
		v, verr := m.reg.Validate(name, raw)
		if verr != nil {
			m.logger.Warnw("runtimeconfig: dropping invalid stored override",
				"key", name, "error", verr)
			continue
		}
		next.values[name] = v
	}
	m.state.Store(next)
	m.logger.Infow("runtimeconfig: loaded overrides",
		"store", m.store.Kind(), "count", len(next.values))
	return nil
}

// Apply validates raw against the registry, persists it, and (unless the key is
// RestartRequired) swaps it into the live overlay. Persist-first guarantees a
// restart re-reads the same value.
func (m *Manager) Apply(ctx context.Context, name string, raw json.RawMessage) error {
	k, ok := m.reg.Lookup(name)
	if !ok {
		return ErrUnknownKey
	}
	if !k.Mutable {
		return ErrImmutable
	}
	v, err := validateAgainst(k, raw)
	if err != nil {
		return err
	}
	if err := m.store.Upsert(ctx, name, raw); err != nil {
		return fmt.Errorf("persist %s: %w", name, err)
	}
	if k.RestartRequired {
		m.logger.Infow("runtimeconfig: override persisted; effective on next restart",
			"key", name)
		return nil
	}
	next := m.state.Load().clone()
	next.values[name] = v
	m.state.Store(next)
	m.logger.Infow("runtimeconfig: override applied", "key", name)
	return nil
}

// Clear removes any override for name, reverting to the registry default. For a
// RestartRequired key the live value is unchanged until the next boot.
func (m *Manager) Clear(ctx context.Context, name string) error {
	k, ok := m.reg.Lookup(name)
	if !ok {
		return ErrUnknownKey
	}
	if !k.Mutable {
		return ErrImmutable
	}
	if err := m.store.Delete(ctx, name); err != nil {
		return fmt.Errorf("delete %s: %w", name, err)
	}
	if k.RestartRequired {
		return nil
	}
	next := m.state.Load().clone()
	delete(next.values, name)
	m.state.Store(next)
	m.logger.Infow("runtimeconfig: override cleared", "key", name)
	return nil
}

// StoreKind reports the backing store ("postgres"/"memory") for /admin/info.
func (m *Manager) StoreKind() string { return m.store.Kind() }

// resolve returns the effective normalized value for name: the live override if
// present, otherwise the registry default. Returns nil for an unknown key.
func (m *Manager) resolve(name string) any {
	if v, ok := m.state.Load().get(name); ok {
		return v
	}
	if k, ok := m.reg.Lookup(name); ok {
		return k.Default
	}
	m.logger.Warnw("runtimeconfig: read of unregistered key", "key", name)
	return nil
}

// Int returns the effective int value for name (0 if unset/wrong kind).
func (m *Manager) Int(name string) int { return int(m.Int64(name)) }

// Int64 returns the effective int64 value for name.
func (m *Manager) Int64(name string) int64 {
	if v, ok := m.resolve(name).(int64); ok {
		return v
	}
	return 0
}

// Float returns the effective float64 value for name.
func (m *Manager) Float(name string) float64 {
	if v, ok := m.resolve(name).(float64); ok {
		return v
	}
	return 0
}

// Bool returns the effective bool value for name.
func (m *Manager) Bool(name string) bool {
	if v, ok := m.resolve(name).(bool); ok {
		return v
	}
	return false
}

// String returns the effective string value for name.
func (m *Manager) String(name string) string {
	if v, ok := m.resolve(name).(string); ok {
		return v
	}
	return ""
}

// Duration returns the effective time.Duration value for name.
func (m *Manager) Duration(name string) time.Duration {
	if v, ok := m.resolve(name).(time.Duration); ok {
		return v
	}
	return 0
}

// KeyView is the per-key entry of the merged config view (GET /admin/<svc>/config).
type KeyView struct {
	Key             string   `json:"key"`
	Value           any      `json:"value"`
	Type            Kind     `json:"type"`
	Default         any      `json:"default"`
	Source          string   `json:"source"` // "override" | "default"
	Description     string   `json:"description,omitempty"`
	Unit            string   `json:"unit,omitempty"`
	Mutable         bool     `json:"mutable"`
	RestartRequired bool     `json:"restart_required"`
	Bounds          *Bounds  `json:"bounds,omitempty"`
	Enum            []string `json:"enum,omitempty"`
	Secret          bool     `json:"secret,omitempty"`
}

// MergedView returns every registered key with its effective value and source,
// in registration order. Secret keys are redacted (presence only).
func (m *Manager) MergedView() []KeyView {
	cur := m.state.Load()
	keys := m.reg.Keys()
	out := make([]KeyView, 0, len(keys))
	for _, k := range keys {
		override, hasOverride := cur.get(k.Name)
		eff := k.Default
		source := "default"
		if hasOverride {
			eff = override
			source = "override"
		}
		view := KeyView{
			Key:             k.Name,
			Value:           redact(k, toJSONValue(k.Kind, eff)),
			Type:            k.Kind,
			Default:         redact(k, toJSONValue(k.Kind, k.Default)),
			Source:          source,
			Description:     k.Description,
			Unit:            k.Unit,
			Mutable:         k.Mutable,
			RestartRequired: k.RestartRequired,
			Bounds:          k.Bounds,
			Enum:            k.Enum,
			Secret:          k.Secret,
		}
		out = append(out, view)
	}
	return out
}

// redact masks secret values: empty stays empty, anything else becomes a fixed
// marker so the merged view reveals presence but never the secret itself.
func redact(k Key, v any) any {
	if !k.Secret {
		return v
	}
	if s, ok := v.(string); ok && s == "" {
		return ""
	}
	return "********"
}
