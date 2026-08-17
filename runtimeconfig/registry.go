// Package runtimeconfig provides a typed, atomically-swappable runtime
// configuration overlay for Atmosoar services. It is modeled on the gateway's
// dispatcher (atomic.Pointer holding an immutable snapshot, swapped wholesale so
// hot-path reads never lock) and on the forecast ingestor's proven
// runtime_config key/value pattern (validate-before-apply, persist-first,
// merged-view-with-source).
//
// A service declares its tunables in a Registry (key, kind, default, bounds,
// mutability), backs them with a Store (Postgres via the pgxstore sub-package, or
// in-memory), and reads them on the hot path through the Manager's typed getters
// (Int/Float/Bool/String/Duration). The admin package exposes the
// GET/PUT/DELETE /admin/<svc>/config contract over the Manager.
package runtimeconfig

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Kind is the value type of a config key. It determines JSON encoding, bounds
// semantics, and which typed getter applies.
type Kind string

// The supported Kind values. Each maps a config key to its JSON encoding and
// the typed getter that reads it.
const (
	KindInt      Kind = "int"      // JSON number (integral); getter Int/Int64
	KindFloat    Kind = "float"    // JSON number; getter Float
	KindBool     Kind = "bool"     // JSON bool; getter Bool
	KindString   Kind = "string"   // JSON string; getter String
	KindEnum     Kind = "enum"     // JSON string constrained to Enum; getter String
	KindDuration Kind = "duration" // JSON number of SECONDS; getter Duration
)

// Bounds is an inclusive numeric range for KindInt/KindFloat/KindDuration. For
// durations the bounds are expressed in seconds.
type Bounds struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

// FloatBounds is a convenience constructor. The parameters are named lo/hi
// rather than min/max so they do not shadow the min/max builtins.
func FloatBounds(lo, hi float64) *Bounds { return &Bounds{Min: &lo, Max: &hi} }

// Key declares one runtime-tunable configuration value.
type Key struct {
	Name        string
	Kind        Kind
	Default     any // typed default; normalized at registration
	Description string
	Unit        string // optional display hint, e.g. "seconds", "dBZ", "meters"
	Mutable     bool   // false => read-only introspection only
	// RestartRequired keys are persisted and reported on a PUT but NOT hot-swapped
	// into the live overlay; the new value takes effect on the next boot (when
	// LoadAtBoot republishes all persisted values). Use for values read once at
	// construction (loop intervals, pool sizes, caches built at startup).
	RestartRequired bool
	// Secret redacts the value in the merged config view (presence only, never the
	// actual value).
	Secret bool
	Bounds *Bounds  // numeric kinds
	Enum   []string // KindEnum allowed values
}

// Registry is the per-service declaration of config keys, in registration order.
type Registry struct {
	mu    sync.RWMutex
	order []string
	keys  map[string]Key
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{keys: make(map[string]Key)}
}

// Register adds a key. It panics on a duplicate name or a Default that cannot be
// normalized to the key's Kind — both are programmer errors caught at boot.
// Returns the registry for chaining.
func (r *Registry) Register(k Key) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if k.Name == "" {
		panic("runtimeconfig: key name must not be empty")
	}
	if _, dup := r.keys[k.Name]; dup {
		panic(fmt.Sprintf("runtimeconfig: duplicate key %q", k.Name))
	}
	nv, err := normalizeValue(k.Kind, k.Default)
	if err != nil {
		panic(fmt.Sprintf("runtimeconfig: key %q default invalid: %v", k.Name, err))
	}
	k.Default = nv
	r.keys[k.Name] = k
	r.order = append(r.order, k.Name)
	return r
}

// Lookup returns the key descriptor for name.
func (r *Registry) Lookup(name string) (Key, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	k, ok := r.keys[name]
	return k, ok
}

// Keys returns all keys in registration order.
func (r *Registry) Keys() []Key {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Key, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.keys[n])
	}
	return out
}

// Validate decodes raw against the key's Kind, enforces bounds/enum, and returns
// the normalized typed value (int64/float64/bool/string/time.Duration). It is the
// single gate every PUT and every boot-time load passes through.
func (r *Registry) Validate(name string, raw json.RawMessage) (any, error) {
	k, ok := r.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("unknown config key %q", name)
	}
	return validateAgainst(k, raw)
}

func validateAgainst(k Key, raw json.RawMessage) (any, error) {
	switch k.Kind {
	case KindBool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, fmt.Errorf("%s: expected bool: %w", k.Name, err)
		}
		return b, nil

	case KindString:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("%s: expected string: %w", k.Name, err)
		}
		return s, nil

	case KindEnum:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("%s: expected string: %w", k.Name, err)
		}
		for _, e := range k.Enum {
			if e == s {
				return s, nil
			}
		}
		return nil, fmt.Errorf("%s: %q not in allowed set %v", k.Name, s, k.Enum)

	case KindInt:
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("%s: expected number: %w", k.Name, err)
		}
		if f != float64(int64(f)) {
			return nil, fmt.Errorf("%s: expected integer, got %v", k.Name, f)
		}
		if err := checkBounds(k, f); err != nil {
			return nil, err
		}
		return int64(f), nil

	case KindFloat:
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("%s: expected number: %w", k.Name, err)
		}
		if err := checkBounds(k, f); err != nil {
			return nil, err
		}
		return f, nil

	case KindDuration:
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("%s: expected number of seconds: %w", k.Name, err)
		}
		if err := checkBounds(k, f); err != nil {
			return nil, err
		}
		return time.Duration(f * float64(time.Second)), nil

	default:
		return nil, fmt.Errorf("%s: unsupported kind %q", k.Name, k.Kind)
	}
}

func checkBounds(k Key, f float64) error {
	if k.Bounds == nil {
		return nil
	}
	if k.Bounds.Min != nil && f < *k.Bounds.Min {
		return fmt.Errorf("%s: %v below minimum %v", k.Name, f, *k.Bounds.Min)
	}
	if k.Bounds.Max != nil && f > *k.Bounds.Max {
		return fmt.Errorf("%s: %v above maximum %v", k.Name, f, *k.Bounds.Max)
	}
	return nil
}

// normalizeValue coerces a Go value to the canonical type for a Kind so that
// registry defaults and validated overrides share one type per kind:
// int64, float64, bool, string, or time.Duration. For durations, a plain
// int/float is interpreted as seconds.
func normalizeValue(kind Kind, v any) (any, error) {
	switch kind {
	case KindInt:
		switch n := v.(type) {
		case int:
			return int64(n), nil
		case int64:
			return n, nil
		case float64:
			if n != float64(int64(n)) {
				return nil, fmt.Errorf("non-integer %v for int kind", n)
			}
			return int64(n), nil
		default:
			return nil, fmt.Errorf("cannot use %T as int", v)
		}
	case KindFloat:
		switch n := v.(type) {
		case int:
			return float64(n), nil
		case int64:
			return float64(n), nil
		case float64:
			return n, nil
		default:
			return nil, fmt.Errorf("cannot use %T as float", v)
		}
	case KindBool:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("cannot use %T as bool", v)
		}
		return b, nil
	case KindString, KindEnum:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("cannot use %T as string", v)
		}
		return s, nil
	case KindDuration:
		switch n := v.(type) {
		case time.Duration:
			return n, nil
		case int:
			return time.Duration(n) * time.Second, nil
		case int64:
			return time.Duration(n) * time.Second, nil
		case float64:
			return time.Duration(n * float64(time.Second)), nil
		default:
			return nil, fmt.Errorf("cannot use %T as duration", v)
		}
	default:
		return nil, fmt.Errorf("unsupported kind %q", kind)
	}
}

// toJSONValue converts a normalized typed value to its JSON representation
// (durations become a number of seconds).
func toJSONValue(kind Kind, v any) any {
	if kind == KindDuration {
		if d, ok := v.(time.Duration); ok {
			return d.Seconds()
		}
	}
	return v
}
