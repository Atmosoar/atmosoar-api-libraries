package location

import (
	"encoding/json"
	"math"
	"testing"

	"atmosoar.io/atmosoar-api-libraries/shapefile"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseLocation_RejectsNonFiniteAndOutOfRange covers the one-request OOM:
// strconv.ParseFloat accepts NaN, Inf and 1e300, and a rectangle built from
// them sent ExpandLocations into an endless loop that ended in a fatal (not
// recoverable) out-of-memory error. Every grammar must now fail at parse time.
func TestParseLocation_RejectsNonFiniteAndOutOfRange(t *testing.T) {
	inputs := []string{
		"NaN,NaN",
		"Inf,0",
		"0,-Inf",
		"91,0",
		"0,181",
		"1e300,1e300|-1e300,-1e300",
		"point:NaN,1",
		"polyline:1,2;1e300,3",
		"1,2,NaN,4,10",
		"1,2,3,400,10",
		"rectangle:-Inf,-Inf,Inf,Inf",
		"rectangle:-90,-1e17,90,1e17",
		"rectangle:95,0|-95,0",
		"radius:NaN,1|10",
		"91,0|10",
		"bbox:-Inf,-90,Inf,90",
		"bbox:-200,-90,180,90",
		"bbox:0,-91,10,10",
		"bbox:NaN,0,1,1",
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			_, err := ParseLocation(in)
			require.Error(t, err)
		})
	}

	// The edges of the valid range still parse.
	for _, in := range []string{"90,180", "-90,-180", "rectangle:-90,-180,90,180", "bbox:-180,-90,180,90"} {
		t.Run("valid "+in, func(t *testing.T) {
			_, err := ParseLocation(in)
			require.NoError(t, err)
		})
	}
}

// TestUnmarshalLocation_RejectsOutOfRange covers the structured JSON payload
// path, which bypasses the string parsers.
func TestUnmarshalLocation_RejectsOutOfRange(t *testing.T) {
	bad := []string{
		`{"type":"point","payload":{"lat":1e300,"lon":0}}`,
		`{"type":"point_list","payload":{"points":[{"lat":0,"lon":0},{"lat":0,"lon":999}]}}`,
		`{"type":"polyline","payload":{"polyline":[{"lat":-100,"lon":0}]}}`,
		`{"type":"rectangle","payload":{"north_east":{"lat":1e17,"lon":1e17},"south_west":{"lat":0,"lon":0}}}`,
		`{"type":"radius","payload":{"lat":200,"lon":0,"radius":10}}`,
		`{"type":"polygon","payload":{"rings":[[{"lat":0,"lon":0},{"lat":1,"lon":500},{"lat":1,"lon":1}]]}}`,
		`{"type":"bbox","payload":{"west":-1e300,"south":0,"east":1,"north":1}}`,
		`{"type":"bbox","payload":{"west":0,"south":0,"east":1,"north":1,"density":1000}}`,
		`{"type":"bbox","payload":{"west":0,"south":0,"east":1,"north":1,"density":-1}}`,
		`{"type":"bbox","payload":{"west":1,"south":0,"east":0,"north":1}}`,
	}
	for _, in := range bad {
		t.Run(in, func(t *testing.T) {
			var loc Location
			require.Error(t, json.Unmarshal([]byte(in), &loc))
		})
	}

	var ok Location
	require.NoError(t, json.Unmarshal(
		[]byte(`{"type":"bbox","payload":{"west":5,"south":47,"east":15,"north":55,"density":10}}`), &ok,
	))
	assert.Equal(t, 10, ok.Payload.(BboxLocation).Density)
}

// TestUnmarshalLocation_PolygonVertexCap bounds the point-in-polygon cost of a
// user-supplied polygon.
func TestUnmarshalLocation_PolygonVertexCap(t *testing.T) {
	ring := make([]PointLocation, MaxPolygonVertices+1)
	raw, err := json.Marshal(map[string]any{
		"type":    "polygon",
		"payload": PolygonLocation{Rings: [][]PointLocation{ring}},
	})
	require.NoError(t, err)
	var loc Location
	err = json.Unmarshal(raw, &loc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vertices")
}

// TestUnmarshalLocation_PointCountCap keeps a structured polyline or
// point_list from carrying more points than the query grammar allows, since
// each point is a downstream upstream lookup.
func TestUnmarshalLocation_PointCountCap(t *testing.T) {
	pts := make([]PointLocation, MaxPolylinePoints+1)
	cases := map[string]any{
		"polyline":   PolylineLocation{Coordinates: pts},
		"point_list": PointListLocation{Points: pts},
	}
	for typ, payload := range cases {
		t.Run(typ, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"type": typ, "payload": payload})
			require.NoError(t, err)
			var loc Location
			err = json.Unmarshal(raw, &loc)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "at most")
		})
	}

	raw, err := json.Marshal(map[string]any{
		"type":    "polyline",
		"payload": PolylineLocation{Coordinates: pts[:MaxPolylinePoints]},
	})
	require.NoError(t, err)
	var ok Location
	require.NoError(t, json.Unmarshal(raw, &ok))
}

// TestExpandLocations_NonFiniteRectangleTerminates is the defence in depth for
// a hand-built Location that never went through a parser: expansion must
// terminate with a bounded result instead of looping until OOM.
func TestExpandLocations_NonFiniteRectangleTerminates(t *testing.T) {
	rects := []RectangleLocation{
		{
			SouthWest: PointLocation{Lat: math.Inf(-1), Lon: math.Inf(-1)},
			NorthEast: PointLocation{Lat: math.Inf(1), Lon: math.Inf(1)},
		},
		{SouthWest: PointLocation{Lat: math.NaN(), Lon: 0}, NorthEast: PointLocation{Lat: 1, Lon: 1}},
		{SouthWest: PointLocation{Lat: -1e300, Lon: -1e300}, NorthEast: PointLocation{Lat: 1e300, Lon: 1e300}},
		{SouthWest: PointLocation{Lat: -90, Lon: -1e17}, NorthEast: PointLocation{Lat: 90, Lon: 1e17}},
	}
	for _, r := range rects {
		pts := ExpandLocations(Location{Type: LocationTypeRectangle, Payload: r})
		assert.NotEmpty(t, pts)
		assert.LessOrEqual(t, len(pts), (maxGridPoints+3)*(maxGridPoints+3))
	}
}

// TestSteppedValues_NoProgressIsBounded: at 1e17 adding a step of 1 does not
// change v, which used to loop forever.
func TestSteppedValues_NoProgressIsBounded(t *testing.T) {
	vals := steppedValues(1e17, 2e17, 1)
	assert.LessOrEqual(t, len(vals), maxGridPoints+3)
	assert.Equal(t, 2e17, vals[len(vals)-1])
}

// TestGridPointCount_Saturates: an Inf span used to convert to an
// implementation-defined int that wrapped to 1 and ended the coarsening loop.
func TestGridPointCount_Saturates(t *testing.T) {
	assert.Equal(t, math.MaxInt, gridPointCount(math.Inf(1), 1, 1))
	assert.Equal(t, math.MaxInt, gridPointCount(math.NaN(), 1, 1))
	assert.Equal(t, math.MaxInt, gridPointCount(1e17, 1e17, 1))
}

// TestBboxGridDims covers the non-allocating size check that callers must run
// before expanding a bbox: bbox:-180,-90,180,90 at density 1000 used to ask
// make() for ~1 TB and abort the process.
func TestBboxGridDims(t *testing.T) {
	world := BboxLocation{West: -180, South: -90, East: 180, North: 90}

	rows, cols, err := BboxGridDims(world, 1, 0)
	require.NoError(t, err)
	assert.Equal(t, 181, rows)
	assert.Equal(t, 361, cols)

	_, _, err = BboxGridDims(world, 100, 0)
	require.Error(t, err, "648M points must be rejected")
	assert.Contains(t, err.Error(), "exceeds")

	_, _, err = BboxGridDims(world, 1000, 0)
	require.Error(t, err)
	_, _, err = BboxGridDims(world, 0, 0)
	require.Error(t, err)
	_, _, err = BboxGridDims(BboxLocation{West: math.Inf(-1), South: 0, East: 1, North: 1}, 1, 0)
	require.Error(t, err)
	_, _, err = BboxGridDims(BboxLocation{West: 1, South: 0, East: 0, North: 1}, 1, 0)
	require.Error(t, err)

	// A caller-supplied limit below the package ceiling applies.
	_, _, err = BboxGridDims(world, 1, 1000)
	require.Error(t, err)
}

func TestExpandBboxToGridChecked(t *testing.T) {
	pts, rows, cols, err := ExpandBboxToGridChecked(BboxLocation{West: 10, South: 50, East: 11, North: 51}, 2, 0)
	require.NoError(t, err)
	assert.Equal(t, 3, rows)
	assert.Equal(t, 3, cols)
	assert.Len(t, pts, 9)
	assert.Equal(t, PointLocation{Lat: 51, Lon: 10}, pts[0])

	pts, _, _, err = ExpandBboxToGridChecked(BboxLocation{West: -180, South: -90, East: 180, North: 90}, 100, 0)
	require.Error(t, err)
	assert.Nil(t, pts)
}

// TestExpandBboxToGrid_ClampsInsteadOfOOM: the deprecated wrapper keeps its
// signature but never allocates more than MaxBboxGridPoints.
func TestExpandBboxToGrid_ClampsInsteadOfOOM(t *testing.T) {
	world := BboxLocation{West: -180, South: -90, East: 180, North: 90}
	for _, ppd := range []int{100, 1000, 100000} {
		pts, rows, cols := ExpandBboxToGrid(world, ppd)
		assert.LessOrEqual(t, len(pts), MaxBboxGridPoints)
		assert.Equal(t, rows*cols, len(pts), "rows/cols must describe the grid actually produced")
		assert.Positive(t, len(pts))
	}

	pts, rows, cols := ExpandBboxToGrid(BboxLocation{West: math.Inf(-1), South: 0, East: math.Inf(1), North: 1}, 10)
	assert.Empty(t, pts)
	assert.Zero(t, rows)
	assert.Zero(t, cols)

	// Small grids are unchanged.
	pts, rows, cols = ExpandBboxToGrid(BboxLocation{West: 10, South: 50, East: 11, North: 51}, 2)
	assert.Len(t, pts, 9)
	assert.Equal(t, 3, rows)
	assert.Equal(t, 3, cols)
}

// TestExpandLocations_CountryPolygonsCoverMainland is the regression test for
// the flattened-MultiPolygon bug: Rings[0] of France is French Guiana, of
// Norway Svalbard and of the United Kingdom Northern Ireland, and expansion
// used to return only that ring's vertices.
func TestExpandLocations_CountryPolygonsCoverMainland(t *testing.T) {
	sp := shapefile.NewShapeProvider(nil)
	type box struct{ latMin, latMax, lonMin, lonMax float64 }
	tests := []struct {
		name     string
		mainland box
		minIn    int
	}{
		{"France", box{42, 51.2, -5, 8.5}, 20},
		{"Norway", box{57.9, 71.5, 4.5, 31.5}, 20},
		{"United Kingdom", box{49.9, 59, -8.2, 2}, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cp := sp.Lookup(tt.name)
			require.NotNil(t, cp)
			rings := make([][]PointLocation, 0, len(cp.Rings))
			for _, r := range cp.Rings {
				ring := make([]PointLocation, 0, len(r))
				for _, p := range r {
					ring = append(ring, PointLocation{Lat: p.Lat, Lon: p.Lon})
				}
				rings = append(rings, ring)
			}
			pts := ExpandLocations(Location{Type: LocationTypePolygon, Payload: PolygonLocation{Rings: rings}})
			require.NotEmpty(t, pts)
			assert.LessOrEqual(t, len(pts), maxGridPoints)

			in := 0
			for _, p := range pts {
				assert.True(t, pointInRings(p.Lat, p.Lon, rings), "%v is outside %s", p, tt.name)
				m := tt.mainland
				if p.Lat >= m.latMin && p.Lat <= m.latMax && p.Lon >= m.lonMin && p.Lon <= m.lonMax {
					in++
				}
			}
			assert.GreaterOrEqual(t, in, tt.minIn, "%s must be sampled across its mainland", tt.name)
		})
	}
}

// figureEight returns one self-intersecting ring of n vertices whose two lobes
// span the whole longitude range with opposite orientation, so its signed
// shoelace area cancels to ~0 while its bounding box is nearly the globe.
func figureEight(n int) []PointLocation {
	edge := func(lat, fromLon, toLon float64) []PointLocation {
		pts := make([]PointLocation, 0, n/4)
		for i := 0; i < n/4; i++ {
			pts = append(pts, PointLocation{Lat: lat, Lon: fromLon + (toLon-fromLon)*float64(i)/float64(n/4)})
		}
		return pts
	}
	var ring []PointLocation
	ring = append(ring, edge(-80, -179, 179)...)
	ring = append(ring, edge(0, 179, -179)...)
	ring = append(ring, edge(80, -179, 179)...)
	ring = append(ring, edge(0, 179, -179)...)
	return ring
}

// TestExpandPolygonToGrid_BoundsWorkForLargePayloads: a structured polygon at
// MaxPolygonVertices whose areas cancel started at the finest step over a
// global bounding box and cost ~300 ms of point-in-polygon tests per request.
// The candidate grid now shrinks with the vertex count.
func TestExpandPolygonToGrid_BoundsWorkForLargePayloads(t *testing.T) {
	ring := figureEight(MaxPolygonVertices)
	require.Len(t, ring, MaxPolygonVertices)
	require.InDelta(t, 0, ringArea(ring), 1e-6, "lobes must cancel for the test to mean anything")

	step := polygonGridStep(160, 358, math.Abs(ringArea(ring)), len(ring))
	assert.LessOrEqual(t, gridPointCount(160, 358, step)*len(ring), polygonEdgeBudget)

	pts := expandPolygonToGrid([][]PointLocation{ring})
	assert.NotEmpty(t, pts)
	assert.LessOrEqual(t, len(pts), maxGridPoints)

	// Embedded countries stay at the full candidate grid.
	assert.Equal(t,
		math.Max(math.Sqrt(1000.0/maxGridPoints), minPolygonGridStep),
		polygonGridStep(10, 10, 1000, 794))
}
