package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"go.uber.org/zap"

	"atmosoar.io/atmosoar-api-libraries/runtimeconfig"
)

// ErrUnknownFlag is returned by FlagSet.Set for a flag outside the declared set.
var ErrUnknownFlag = fmt.Errorf("unknown feature flag")

// Flag declares a known feature flag (a boolean kill-switch), mirroring the
// forecast ingestor's closed KNOWN_FLAGS allowlist.
type Flag struct {
	Name        string
	Default     bool
	Description string
}

// FlagView is the per-flag entry of GET /admin/<svc>/flags.
type FlagView struct {
	Name        string `json:"name"`
	Value       bool   `json:"value"`
	Default     bool   `json:"default"`
	Description string `json:"description,omitempty"`
}

// FlagSet is an atomic, lock-free-read feature-flag set backed by its own Store
// so it never collides with the runtimeconfig Manager's keys. Persisted entries
// are stored under the flag name; the closed `known` allowlist guards toggles.
type FlagSet struct {
	store  runtimeconfig.Store
	logger *zap.SugaredLogger
	known  []Flag
	byName map[string]Flag
	state  atomic.Pointer[map[string]bool]
}

// NewFlagSet builds a flag set over store with the given known flags. Pass a
// runtimeconfig.MemStore for ephemeral flags or a pgxstore for durable ones (the
// store should be dedicated to flags, e.g. a feature_flags table).
func NewFlagSet(store runtimeconfig.Store, logger *zap.SugaredLogger, known []Flag) *FlagSet {
	byName := make(map[string]Flag, len(known))
	for _, f := range known {
		byName[f.Name] = f
	}
	fs := &FlagSet{store: store, logger: logger, known: known, byName: byName}
	initial := make(map[string]bool, len(known))
	for _, f := range known {
		initial[f.Name] = f.Default
	}
	fs.state.Store(&initial)
	return fs
}

// LoadAtBoot overlays persisted flag values onto the declared defaults.
func (f *FlagSet) LoadAtBoot(ctx context.Context) error {
	stored, err := f.store.LoadAll(ctx)
	if err != nil {
		f.logger.Warnw("flags: load at boot failed; using defaults", "error", err)
		return nil
	}
	next := make(map[string]bool, len(f.known))
	for _, fl := range f.known {
		next[fl.Name] = fl.Default
	}
	for name, raw := range stored {
		if _, ok := f.byName[name]; !ok {
			continue
		}
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			f.logger.Warnw("flags: dropping invalid stored flag", "flag", name, "error", err)
			continue
		}
		next[name] = b
	}
	f.state.Store(&next)
	return nil
}

// Enabled reports the live value of a flag (false if unknown).
func (f *FlagSet) Enabled(name string) bool {
	return (*f.state.Load())[name]
}

// Set validates the flag is known, persists it, and swaps the live snapshot.
func (f *FlagSet) Set(ctx context.Context, name string, value bool) error {
	if _, ok := f.byName[name]; !ok {
		return ErrUnknownFlag
	}
	raw, _ := json.Marshal(value)
	if err := f.store.Upsert(ctx, name, raw); err != nil {
		return fmt.Errorf("persist flag %s: %w", name, err)
	}
	next := make(map[string]bool, len(*f.state.Load()))
	for k, v := range *f.state.Load() {
		next[k] = v
	}
	next[name] = value
	f.state.Store(&next)
	f.logger.Infow("flags: toggled", "flag", name, "value", value)
	return nil
}

// Snapshot returns the declared flags with their live values, in declaration
// order.
func (f *FlagSet) Snapshot() []FlagView {
	cur := *f.state.Load()
	out := make([]FlagView, 0, len(f.known))
	for _, fl := range f.known {
		out = append(out, FlagView{
			Name:        fl.Name,
			Value:       cur[fl.Name],
			Default:     fl.Default,
			Description: fl.Description,
		})
	}
	return out
}
