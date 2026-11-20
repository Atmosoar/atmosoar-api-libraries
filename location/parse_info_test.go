package location

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ATMO-430: the bare `a|b|...` form picks its type from the point count and
// order. ParseLocationWithInfo reports that choice so a service can warn,
// while the parsed Location stays exactly what ParseLocation returns.

func TestParseLocationWithInfo_Inferred(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantType LocationType
		inferred bool
	}{
		{"one bare point", "50.11,8.68", LocationTypePoint, false},
		{"two bare points NE then SW", "52.52,13.40|48.14,11.58", LocationTypeRectangle, true},
		{"two bare points SW then NE", "48.14,11.58|52.52,13.40", LocationTypePolyline, true},
		{"two bare points NW then SE", "52.52,8.68|48.14,13.40", LocationTypePolyline, true},
		{"three bare points", "52.52,13.40|48.14,11.58|50.11,8.68", LocationTypePolyline, true},
		{"four bare points", "52.52,13.40|48.14,11.58|50.11,8.68|53.55,9.99", LocationTypePolyline, true},
		{"points: one", "points:50.11,8.68", LocationTypeMultiPoint, false},
		{"points: two NE then SW", "points:52.52,13.40|48.14,11.58", LocationTypeMultiPoint, false},
		{"points: two SW then NE", "points:48.14,11.58|52.52,13.40", LocationTypeMultiPoint, false},
		{"points: three", "points:52.52,13.40|48.14,11.58|50.11,8.68", LocationTypeMultiPoint, false},
		{"points: four", "points:52.52,13.40|48.14,11.58|50.11,8.68|53.55,9.99", LocationTypeMultiPoint, false},
		{"rectangle: prefix", "rectangle:52.52,13.40|48.14,11.58", LocationTypeRectangle, false},
		{"polyline: prefix", "polyline:52.52,13.40|48.14,11.58|50.11,8.68", LocationTypePolyline, false},
		{"point: prefix", "point:50.11,8.68", LocationTypePoint, false},
		{"radius", "50.11,8.68|25", LocationTypeRadius, false},
		{"generated polyline", "50,8,51,9,5", LocationTypePolyline, false},
		{"bbox", "bbox:5,47,15,55", LocationTypeBbox, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loc, info, err := ParseLocationWithInfo(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.wantType, loc.Type)
			assert.Equal(t, tc.inferred, info.Inferred)

			plain, err := ParseLocation(tc.in)
			require.NoError(t, err)
			assert.Equal(t, plain, loc, "ParseLocationWithInfo must not change the parsed Location")
		})
	}
}

func TestParseLocationWithInfo_ErrorsMatchParseLocation(t *testing.T) {
	for _, in := range []string{
		"", "abc", "points:", "rectangle:1,2|3,4|5,6", "point:1,2|3,4", "91,0|0,0", "polygon:1,2|3,4|5,6",
	} {
		t.Run(in, func(t *testing.T) {
			loc, info, err := ParseLocationWithInfo(in)
			_, plainErr := ParseLocation(in)
			require.Error(t, err)
			assert.Equal(t, plainErr.Error(), err.Error())
			assert.Nil(t, loc)
			assert.False(t, info.Inferred)
		})
	}
}
