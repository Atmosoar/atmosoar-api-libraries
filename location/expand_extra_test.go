package location

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSteppedValues_NoDuplicateUpper verifies BUG-003: with lower=0,
// upper=1, step=0.1 the accumulated drift in `for v := lower; ; v += step`
// leaves the last computed value at 0.9999999999999999 on most CPUs. The
// epsilon snap must NOT then append a duplicate 1.0 — the result has exactly
// 11 points.
func TestSteppedValues_NoDuplicateUpper(t *testing.T) {
	got := steppedValues(0, 1, 0.1)
	assert.Len(t, got, 11, "expected 11 evenly spaced points, got %v", got)
	assert.InDelta(t, 0.0, got[0], 1e-12)
	assert.InDelta(t, 1.0, got[len(got)-1], 1e-12)
}

// BenchmarkExpandRadius_100km exercises the BUG-004 hot path. 100 km @ ring
// step 3 km → 34 rings × 3..36 bearings ≈ 700 points.
func BenchmarkExpandRadius_100km(b *testing.B) {
	loc := Location{
		Type:    LocationTypeRadius,
		Payload: RadiusLocation{Lat: 50.0, Lon: 8.0, Radius: 100.0},
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = ExpandLocations(loc)
	}
}
