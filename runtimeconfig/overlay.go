package runtimeconfig

// Overlay is an immutable snapshot of the active config overrides (key ->
// normalized typed value). A key absent from the map resolves to its registry
// default. An Overlay is published into the Manager's atomic pointer and never
// mutated in place — every change clones a fresh Overlay and swaps it wholesale,
// so concurrent readers are race-free without locking.
type Overlay struct {
	values map[string]any
}

func newOverlay() *Overlay { return &Overlay{values: make(map[string]any)} }

// clone returns a shallow copy with an independent map, ready to be mutated and
// re-published.
func (o *Overlay) clone() *Overlay {
	cp := make(map[string]any, len(o.values)+1)
	for k, v := range o.values {
		cp[k] = v
	}
	return &Overlay{values: cp}
}

// get returns the override value for name and whether one is present.
func (o *Overlay) get(name string) (any, bool) {
	v, ok := o.values[name]
	return v, ok
}
