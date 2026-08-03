package location

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// --- expansion caps (DoS) ---

// TestParseLocation_RadiusCap verifies an oversized radius fails at PARSE
// time. "0,0|40000" used to reach ExpandLocations and pre-allocate ~267M
// points (~4.27 GB); it must now be a clean parse error.
func TestParseLocation_RadiusCap(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"at the cap", fmt.Sprintf("52.52,13.41|%g", MaxRadiusKm), false},
		{"just over the cap", fmt.Sprintf("52.52,13.41|%g", MaxRadiusKm+1), true},
		{"OOM radius", "0,0|40000", true},
		{"infinite radius", "0,0|Inf", true},
		{"NaN radius", "0,0|NaN", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseLocation(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestUnmarshalLocation_RadiusCap covers the structured JSON payload path,
// which bypasses tryParseRadius entirely — without its own check it was an
// unguarded route to the same multi-gigabyte allocation.
func TestUnmarshalLocation_RadiusCap(t *testing.T) {
	var loc Location
	err := json.Unmarshal([]byte(`{"type":"radius","payload":{"lat":0,"lon":0,"radius":40000}}`), &loc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "radius")

	var ok Location
	require.NoError(t, json.Unmarshal(
		[]byte(`{"type":"radius","payload":{"lat":0,"lon":0,"radius":25}}`), &ok,
	))
	assert.Equal(t, LocationTypeRadius, ok.Type)
}

// TestExpandLocations_RadiusClamped is the defence-in-depth check: a Location
// built by hand (not via ParseLocation) still must not allocate without bound.
func TestExpandLocations_RadiusClamped(t *testing.T) {
	pts := ExpandLocations(Location{
		Type:    LocationTypeRadius,
		Payload: RadiusLocation{Lat: 0, Lon: 0, Radius: 40000},
	})
	assert.LessOrEqual(t, len(pts), MaxExpandedRadiusPoints)

	// NaN must not reach int(math.Floor(NaN)) — it degrades to the centre point.
	nan := ExpandLocations(Location{
		Type:    LocationTypeRadius,
		Payload: RadiusLocation{Lat: 1, Lon: 2, Radius: math.NaN()},
	})
	assert.Equal(t, []PointLocation{{Lat: 1, Lon: 2}}, nan)
}

// TestParseCoordinates_PolylinePointCap verifies the 5-value polyline form
// cannot amplify ~30 characters of input into billions of points.
func TestParseCoordinates_PolylinePointCap(t *testing.T) {
	atCap, err := parseCoordinates(fmt.Sprintf("50,8,51,9,%d", MaxPolylinePoints))
	require.NoError(t, err)
	assert.Len(t, atCap, MaxPolylinePoints)

	_, err = parseCoordinates(fmt.Sprintf("50,8,51,9,%d", MaxPolylinePoints+1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "point count")

	_, err = parseCoordinates("50,8,51,9,2000000000")
	require.Error(t, err)
}

// TestParseCoordinates_ChainedSegmentCap verifies the running total is capped
// too: per-segment caps alone let '|' chaining multiply the point count.
func TestParseCoordinates_ChainedSegmentCap(t *testing.T) {
	seg := fmt.Sprintf("50,8,51,9,%d", MaxPolylinePoints)
	_, err := parseCoordinates(strings.Join([]string{seg, seg, seg}, "|"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expands to more than")
}
