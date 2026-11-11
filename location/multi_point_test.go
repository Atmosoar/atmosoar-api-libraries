package location

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ATMO-405: "points:" is a set of independent sites, never a route or a
// rectangle, and is capped at MaxMultiPoints.

func multiPoints(t *testing.T, loc *Location) []PointLocation {
	t.Helper()
	require.Equal(t, LocationTypeMultiPoint, loc.Type)
	mp, ok := loc.Payload.(MultiPointLocation)
	require.True(t, ok, "payload must be MultiPointLocation, got %T", loc.Payload)
	return mp.Points
}

func TestParseLocation_MultiPoint(t *testing.T) {
	t.Run("three sites hundreds of km apart keep order", func(t *testing.T) {
		loc, err := ParseLocation("points:52.52,13.405|48.137,11.575|50.11,8.682")
		require.NoError(t, err)
		assert.Equal(t, []PointLocation{
			{Lat: 52.52, Lon: 13.405},
			{Lat: 48.137, Lon: 11.575},
			{Lat: 50.11, Lon: 8.682},
		}, multiPoints(t, loc))
	})

	t.Run("single point", func(t *testing.T) {
		loc, err := ParseLocation("points:52.52,13.405")
		require.NoError(t, err)
		assert.Equal(t, []PointLocation{{Lat: 52.52, Lon: 13.405}}, multiPoints(t, loc))
	})

	t.Run("NE/SW pair stays two points, not a rectangle", func(t *testing.T) {
		loc, err := ParseLocation("points:55.0,15.0|47.0,6.0")
		require.NoError(t, err)
		assert.Len(t, multiPoints(t, loc), 2)
	})

	t.Run("points closer than 100 m are all kept, no dedupe", func(t *testing.T) {
		// ~45 m apart, plus an exact duplicate.
		loc, err := ParseLocation("points:52.5200,13.4050|52.5204,13.4050|52.5200,13.4050")
		require.NoError(t, err)
		assert.Len(t, multiPoints(t, loc), 3)
	})

	t.Run("semicolon separator", func(t *testing.T) {
		loc, err := ParseLocation("points:52.52,13.405;48.137,11.575")
		require.NoError(t, err)
		assert.Len(t, multiPoints(t, loc), 2)
	})

	t.Run("prefix is case-insensitive", func(t *testing.T) {
		loc, err := ParseLocation("POINTS:52.52,13.405|48.137,11.575")
		require.NoError(t, err)
		assert.Len(t, multiPoints(t, loc), 2)
	})

	t.Run("whitespace around values is tolerated", func(t *testing.T) {
		loc, err := ParseLocation("points: 52.52, 13.405 | 48.137 ,11.575")
		require.NoError(t, err)
		assert.Len(t, multiPoints(t, loc), 2)
	})

	t.Run("exactly MaxMultiPoints is accepted", func(t *testing.T) {
		loc, err := ParseLocation(nPoints(MaxMultiPoints))
		require.NoError(t, err)
		assert.Len(t, multiPoints(t, loc), MaxMultiPoints)
	})
}

func TestParseLocation_MultiPointErrors(t *testing.T) {
	t.Run("over the cap names the cap", func(t *testing.T) {
		_, err := ParseLocation(nPoints(MaxMultiPoints + 1))
		require.Error(t, err)
		assert.Contains(t, err.Error(), fmt.Sprintf("at most %d", MaxMultiPoints))
		assert.Contains(t, err.Error(), fmt.Sprintf("got %d", MaxMultiPoints+1))
	})

	cases := map[string]string{
		"empty":                            "points:",
		"blank":                            "points:   ",
		"trailing separator":               "points:52.52,13.405|",
		"latitude out of range":            "points:91,13.405",
		"longitude out of range":           "points:52.52,181",
		"not a number":                     "points:abc,13.405",
		"single value":                     "points:52.52",
		"generated polyline form rejected": "points:52.0,13.0,53.0,14.0,10",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseLocation(in)
			require.Error(t, err)
		})
	}

	t.Run("empty error names the cap", func(t *testing.T) {
		_, err := ParseLocation("points:")
		require.Error(t, err)
		assert.Contains(t, err.Error(), fmt.Sprintf("1 to %d", MaxMultiPoints))
	})
}

// The bare forms must not change: 2 points NE/SW → rectangle, other 2 points
// → polyline, 3+ points → polyline.
func TestParseLocation_BareFormsUnchanged(t *testing.T) {
	loc, err := ParseLocation("55.0,15.0|47.0,6.0")
	require.NoError(t, err)
	assert.Equal(t, LocationTypeRectangle, loc.Type)

	loc, err = ParseLocation("47.0,6.0|55.0,15.0")
	require.NoError(t, err)
	assert.Equal(t, LocationTypePolyline, loc.Type)

	loc, err = ParseLocation("52.52,13.405|48.137,11.575|50.11,8.682")
	require.NoError(t, err)
	assert.Equal(t, LocationTypePolyline, loc.Type)
	pl, ok := loc.Payload.(PolylineLocation)
	require.True(t, ok)
	assert.Len(t, pl.Coordinates, 3)

	loc, err = ParseLocation("52.52,13.405")
	require.NoError(t, err)
	assert.Equal(t, LocationTypePoint, loc.Type)
}

func TestUnmarshalJSON_MultiPoint(t *testing.T) {
	t.Run("structured payload", func(t *testing.T) {
		var loc Location
		err := json.Unmarshal(
			[]byte(
				`{"type":"multi_point","payload":{"points":[{"lat":52.52,"lon":13.405},{"lat":48.137,"lon":11.575}]}}`,
			),
			&loc,
		)
		require.NoError(t, err)
		assert.Len(t, multiPoints(t, &loc), 2)
	})

	t.Run("string form", func(t *testing.T) {
		var loc Location
		require.NoError(t, json.Unmarshal([]byte(`"points:52.52,13.405|48.137,11.575"`), &loc))
		assert.Len(t, multiPoints(t, &loc), 2)
	})

	t.Run("round trip", func(t *testing.T) {
		in := Location{
			Type:    LocationTypeMultiPoint,
			Payload: MultiPointLocation{Points: []PointLocation{{Lat: 1, Lon: 2}}},
		}
		b, err := json.Marshal(in)
		require.NoError(t, err)
		var out Location
		require.NoError(t, json.Unmarshal(b, &out))
		assert.Equal(t, in, out)
	})

	bad := map[string]string{
		"empty points":   `{"type":"multi_point","payload":{"points":[]}}`,
		"missing points": `{"type":"multi_point","payload":{}}`,
		"invalid coord":  `{"type":"multi_point","payload":{"points":[{"lat":95,"lon":0}]}}`,
		"wrong shape":    `{"type":"multi_point","payload":{"points":"x"}}`,
	}
	for name, in := range bad {
		t.Run(name, func(t *testing.T) {
			var loc Location
			require.Error(t, json.Unmarshal([]byte(in), &loc))
		})
	}

	t.Run("over the cap", func(t *testing.T) {
		pts := make([]PointLocation, MaxMultiPoints+1)
		b, err := json.Marshal(map[string]any{"type": "multi_point", "payload": MultiPointLocation{Points: pts}})
		require.NoError(t, err)
		var loc Location
		err = json.Unmarshal(b, &loc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), fmt.Sprintf("at most %d", MaxMultiPoints))
	})
}

func TestExpandLocations_MultiPoint(t *testing.T) {
	pts := []PointLocation{{Lat: 52.52, Lon: 13.405}, {Lat: 52.52, Lon: 13.405}, {Lat: 48.137, Lon: 11.575}}
	assert.Equal(
		t,
		pts,
		ExpandLocations(Location{Type: LocationTypeMultiPoint, Payload: MultiPointLocation{Points: pts}}),
	)
	assert.Equal(
		t,
		pts,
		ExpandLocations(Location{Type: LocationTypeMultiPoint, Payload: &MultiPointLocation{Points: pts}}),
	)
	assert.Empty(t, ExpandLocations(Location{Type: LocationTypeMultiPoint, Payload: (*MultiPointLocation)(nil)}))
}

func nPoints(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf("%.3f,%.3f", 40+float64(i)*0.1, 10+float64(i)*0.1)
	}
	return "points:" + strings.Join(parts, "|")
}
