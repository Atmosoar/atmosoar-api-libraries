package location

import (
	"log"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"atmosoar.io/atmosoar-api-libraries/shapefile"
)

// TestLocationParsing tests the ParseLocation function
func TestLocationParsing(t *testing.T) {
	StationMap = map[string]PointLocation{
		"wmo10381": {Lat: 2, Lon: 3},
	}

	LocationShortcutsMap = map[string]RectangleLocation{
		"germany": {NorthEast: PointLocation{Lat: 1.1, Lon: 2.2}, SouthWest: PointLocation{Lat: 3.3, Lon: 4.4}},
	}

	tests := []struct {
		name        string
		input       string
		expected    *Location
		expectedErr string
	}{
		{
			name:  "valid station ID",
			input: "wmo10381",
			expected: &Location{
				Type:    LocationTypePoint,
				Payload: PointLocation{Lat: 2, Lon: 3},
			},
		},
		{
			name:  "valid shortcut",
			input: "germany",
			expected: &Location{
				Type: LocationTypeRectangle,
				Payload: RectangleLocation{
					NorthEast: PointLocation{Lat: 1.1, Lon: 2.2},
					SouthWest: PointLocation{Lat: 3.3, Lon: 4.4},
				},
			},
		},
		{
			name:  "single point",
			input: "52.52,13.41",
			expected: &Location{
				Type:    LocationTypePoint,
				Payload: PointLocation{Lat: 52.52, Lon: 13.41},
			},
		},
		{
			name:  "two points as rectangle",
			input: "53.0,14.0|52.0,13.0",
			expected: &Location{
				Type: LocationTypeRectangle,
				Payload: RectangleLocation{
					NorthEast: PointLocation{Lat: 53.0, Lon: 14.0},
					SouthWest: PointLocation{Lat: 52.0, Lon: 13.0},
				},
			},
		},
		{
			name:  "polyline with generated points",
			input: "0,0,1,1,5",
			expected: &Location{
				Type: LocationTypePolyline,
				Payload: PolylineLocation{
					Coordinates: []PointLocation{
						{0, 0},
						{0.25, 0.25},
						{0.5, 0.5},
						{0.75, 0.75},
						{1, 1},
					},
				},
			},
		},
		{
			name:  "multiple points as polyline",
			input: "52.52,13.41|40.71,-74.01|51.51,-0.12",
			expected: &Location{
				Type: LocationTypePolyline,
				Payload: PolylineLocation{
					Coordinates: []PointLocation{
						{52.52, 13.41},
						{40.71, -74.01},
						{51.51, -0.12},
					},
				},
			},
		},
		{
			name:        "invalid coordinate format",
			input:       "invalid",
			expectedErr: "invalid coordinate format invalid, expected either 2 (lat-lon for a point) or 5 (start lat-lon, end-lan-lon, resolution for a polyline) or a shortcut coseparated values optionally seperated with a pipe",
		},
		{
			name:        "non-numeric coordinate",
			input:       "abc,def",
			expectedErr: "invalid latitude: strconv.ParseFloat: parsing \"abc\"",
		},
		{
			name:        "invalid point count",
			input:       "0,0,1,1,invalid",
			expectedErr: "point count must be integer >= 2",
		},
		{
			name:        "unknown shortcut",
			input:       "UnknownCountry",
			expectedErr: "invalid coordinate format UnknownCountry, expected either 2 (lat-lon for a point) or 5 (start lat-lon, end-lan-lon, resolution for a polyline) or a shortcut coseparated values optionally seperated with a pipe",
		},
		{
			name:        "empty location",
			input:       "",
			expectedErr: "invalid coordinate format , expected either 2 (lat-lon for a point) or 5 (start lat-lon, end-lan-lon, resolution for a polyline) or a shortcut coseparated values optionally seperated with a pipe",
		},
		{
			name:        "invalid point count in polyline",
			input:       "0,0,1,1,1",
			expectedErr: "point count must be integer >= 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log.Println(tt.input)
			result, err := ParseLocation(tt.input)
			log.Println(result)
			log.Println(err)
			if tt.expectedErr != "" {
				assert.ErrorContains(t, err, tt.expectedErr)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected.Type, result.Type)

				switch expected := tt.expected.Payload.(type) {
				case PointLocation:
					assert.Equal(t, expected, result.Payload)
				case RectangleLocation:
					assert.Equal(t, expected, result.Payload)
				case PolylineLocation:
					actual := result.Payload.(PolylineLocation)
					assert.Len(t, actual.Coordinates, len(expected.Coordinates))
					for i, coord := range expected.Coordinates {
						assert.InDelta(t, coord.Lat, actual.Coordinates[i].Lat, 0.0001)
						assert.InDelta(t, coord.Lon, actual.Coordinates[i].Lon, 0.0001)
					}
				}
			}
		})
	}
}

// TestLocationHelperFunctions tests the location-side helper functions.
func TestLocationHelperFunctions(t *testing.T) {
	StationMap = map[string]PointLocation{
		"wmo10381": {Lat: 2, Lon: 3},
	}
	LocationShortcutsMap = map[string]RectangleLocation{
		"germany": {NorthEast: PointLocation{Lat: 1.1, Lon: 2.2}, SouthWest: PointLocation{Lat: 3.3, Lon: 4.4}},
	}

	t.Run("isPredefinedStation", func(t *testing.T) {
		assert.True(t, isPredefinedStation("wmo10381"))
		assert.False(t, isPredefinedStation("invalid"))
	})

	t.Run("isPredefinedShortcut", func(t *testing.T) {
		assert.True(t, isPredefinedShortcut("germany"))
		assert.False(t, isPredefinedShortcut("invalid"))
	})

	t.Run("isRectangle", func(t *testing.T) {
		valid := []PointLocation{
			{Lat: 10, Lon: 10},
			{Lat: 9, Lon: 9},
		}
		assert.True(t, isRectangle(valid))

		invalid := []PointLocation{
			{Lat: 9, Lon: 9},
			{Lat: 10, Lon: 10},
		}
		assert.False(t, isRectangle(invalid))
	})

	t.Run("generatePoints", func(t *testing.T) {
		start := PointLocation{Lat: 0, Lon: 0}
		end := PointLocation{Lat: 1, Lon: 1}

		points, err := generatePoints(start, end, 3)
		assert.NoError(t, err)
		assert.Len(t, points, 3)
		assert.Equal(t, PointLocation{0, 0}, points[0])
		assert.Equal(t, PointLocation{0.5, 0.5}, points[1])
		assert.Equal(t, PointLocation{1, 1}, points[2])

		_, err = generatePoints(start, end, 1)
		assert.Error(t, err)
	})
}

// TestExpandLocations tests the ExpandLocations function for all location types.
func TestExpandLocations(t *testing.T) {
	t.Run("point value", func(t *testing.T) {
		loc := Location{Type: LocationTypePoint, Payload: PointLocation{Lat: 52.52, Lon: 13.41}}
		pts := ExpandLocations(loc)
		assert.Len(t, pts, 1)
		assert.Equal(t, 52.52, pts[0].Lat)
	})

	t.Run("point pointer", func(t *testing.T) {
		loc := Location{Type: LocationTypePoint, Payload: &PointLocation{Lat: 48.85, Lon: 2.35}}
		pts := ExpandLocations(loc)
		assert.Len(t, pts, 1)
		assert.Equal(t, 48.85, pts[0].Lat)
	})

	t.Run("point list value", func(t *testing.T) {
		loc := Location{
			Type: LocationTypePointList,
			Payload: PointListLocation{Points: []PointLocation{
				{Lat: 1, Lon: 2}, {Lat: 3, Lon: 4},
			}},
		}
		pts := ExpandLocations(loc)
		assert.Len(t, pts, 2)
	})

	t.Run("point list pointer", func(t *testing.T) {
		loc := Location{
			Type: LocationTypePointList,
			Payload: &PointListLocation{Points: []PointLocation{
				{Lat: 1, Lon: 2},
			}},
		}
		pts := ExpandLocations(loc)
		assert.Len(t, pts, 1)
	})

	t.Run("polyline value", func(t *testing.T) {
		loc := Location{
			Type: LocationTypePolyline,
			Payload: PolylineLocation{Coordinates: []PointLocation{
				{Lat: 1, Lon: 2}, {Lat: 3, Lon: 4}, {Lat: 5, Lon: 6},
			}},
		}
		pts := ExpandLocations(loc)
		assert.Len(t, pts, 3)
	})

	t.Run("polyline pointer", func(t *testing.T) {
		loc := Location{
			Type: LocationTypePolyline,
			Payload: &PolylineLocation{Coordinates: []PointLocation{
				{Lat: 1, Lon: 2},
			}},
		}
		pts := ExpandLocations(loc)
		assert.Len(t, pts, 1)
	})

	// 152: Rectangle now expands to an adaptive grid.
	t.Run("rectangle value expands to grid", func(t *testing.T) {
		loc := Location{
			Type: LocationTypeRectangle,
			Payload: RectangleLocation{
				NorthEast: PointLocation{Lat: 55, Lon: 15},
				SouthWest: PointLocation{Lat: 47, Lon: 5},
			},
		}
		pts := ExpandLocations(loc)
		// 8 deg lat span, 10 deg lon span, min=8 -> step 1.0
		// lats: 47..55 = 9 values, lons: 5..15 = 11 values -> 99 points
		assert.Len(t, pts, 99)
		// 3: SW corner is the first point.
		assert.Equal(t, 47.0, pts[0].Lat)
		assert.Equal(t, 5.0, pts[0].Lon)
		// 3: NE corner is the last point.
		assert.Equal(t, 55.0, pts[len(pts)-1].Lat)
		assert.Equal(t, 15.0, pts[len(pts)-1].Lon)
	})

	t.Run("rectangle pointer expands to grid", func(t *testing.T) {
		loc := Location{
			Type: LocationTypeRectangle,
			Payload: &RectangleLocation{
				NorthEast: PointLocation{Lat: 55, Lon: 15},
				SouthWest: PointLocation{Lat: 47, Lon: 5},
			},
		}
		pts := ExpandLocations(loc)
		assert.Len(t, pts, 99)
	})

	t.Run("radius small (center only)", func(t *testing.T) {
		loc := Location{
			Type:    LocationTypeRadius,
			Payload: RadiusLocation{Lat: 52.52, Lon: 13.41, Radius: 2.0},
		}
		pts := ExpandLocations(loc)
		assert.Len(t, pts, 1)
		assert.InDelta(t, 52.52, pts[0].Lat, 0.001)
	})

	t.Run("radius large (rings)", func(t *testing.T) {
		loc := Location{
			Type:    LocationTypeRadius,
			Payload: RadiusLocation{Lat: 52.52, Lon: 13.41, Radius: 5.0},
		}
		pts := ExpandLocations(loc)
		assert.Greater(t, len(pts), 1, "should generate ring points for radius >= 3km")
	})

	t.Run("radius pointer", func(t *testing.T) {
		loc := Location{
			Type:    LocationTypeRadius,
			Payload: &RadiusLocation{Lat: 52.52, Lon: 13.41, Radius: 1.0},
		}
		pts := ExpandLocations(loc)
		assert.Len(t, pts, 1)
	})

	t.Run("unknown type returns nil", func(t *testing.T) {
		loc := Location{Type: "unknown", Payload: nil}
		pts := ExpandLocations(loc)
		assert.Nil(t, pts)
	})

	t.Run("nil point pointer", func(t *testing.T) {
		loc := Location{Type: LocationTypePoint, Payload: (*PointLocation)(nil)}
		pts := ExpandLocations(loc)
		assert.Nil(t, pts)
	})
}

// TestParseLocation_Radius tests radius parsing in ParseLocation.
func TestParseLocation_Radius(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		radius  float64
	}{
		{"valid radius", "52.52,13.41|5", false, 5.0},
		{"small radius", "48.85,2.35|0.5", false, 0.5},
		{"zero radius", "52.52,13.41|0", true, 0},
		{"negative radius", "52.52,13.41|-1", true, 0},
		{"invalid lat in radius", "abc,13.41|5", true, 0},
		{"invalid lon in radius", "52.52,abc|5", true, 0},
		{"invalid radius value", "52.52,13.41|abc", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := ParseLocation(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, LocationTypeRadius, loc.Type)
				rl, ok := loc.Payload.(RadiusLocation)
				assert.True(t, ok)
				assert.Equal(t, tt.radius, rl.Radius)
			}
		})
	}
}

// TestDestinationPoint tests the haversine destination calculation.
func TestDestinationPoint(t *testing.T) {
	// Moving due north from equator
	lat2, lon2 := destinationPoint(0, 0, 111.32, 0)
	assert.InDelta(t, 1.0, lat2, 0.02, "~1 degree north")
	assert.InDelta(t, 0.0, lon2, 0.01, "longitude unchanged")

	// Moving due east from equator
	lat2, lon2 = destinationPoint(0, 0, 111.32, 90)
	assert.InDelta(t, 0.0, lat2, 0.02, "latitude unchanged")
	assert.InDelta(t, 1.0, lon2, 0.02, "~1 degree east")
}

// TestParseCoordinates_InvalidLon tests invalid longitude.
func TestParseCoordinates_InvalidLon(t *testing.T) {
	_, err := ParseLocation("52.52,abc")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid longitude")
}

// TestParseCoordinates_InvalidPolylineFields tests invalid polyline fields.
func TestParseCoordinates_InvalidPolylineFields(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		errContains string
	}{
		{"invalid start lon", "0,abc,1,1,3", "invalid start longitude"},
		{"invalid end lat", "0,0,abc,1,3", "invalid end latitude"},
		{"invalid end lon", "0,0,1,abc,3", "invalid end longitude"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseLocation(tt.input)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

// TestTwoPointsAsPolyline tests two points where first is NOT northeast.
func TestTwoPointsAsPolyline(t *testing.T) {
	// When first point lat < second, it's not a rectangle → falls through to polyline.
	loc, err := ParseLocation("40.0,14.0|52.0,13.0")
	assert.NoError(t, err)
	assert.Equal(t, LocationTypePolyline, loc.Type)
}

// TestParseLocation_PolygonResolution tests 3: country shortcuts resolve to polygon
// when ActiveShapeProvider is set.
// 150: Maps use lowercase keys; ParseLocation lowercases input before lookup.
func TestParseLocation_PolygonResolution(t *testing.T) {
	// Setup: register a shortcut and enable the shape provider.
	LocationShortcutsMap = map[string]RectangleLocation{
		"germany": {
			NorthEast: PointLocation{Lat: 55.06, Lon: 15.04},
			SouthWest: PointLocation{Lat: 47.27, Lon: 5.87},
		},
	}
	ActiveShapeProvider = shapefile.NewShapeProvider(nil)
	defer func() { ActiveShapeProvider = nil }()

	t.Run("germany resolves to polygon", func(t *testing.T) {
		loc, err := ParseLocation("germany")
		require.NoError(t, err)
		assert.Equal(t, LocationTypePolygon, loc.Type)
		poly, ok := loc.Payload.(PolygonLocation)
		require.True(t, ok)
		assert.NotEmpty(t, poly.Rings, "should have at least one ring")
		assert.NotEmpty(t, poly.Rings[0], "outer ring should have points")
	})

	t.Run("Germany (PascalCase) resolves to polygon via case-insensitive lookup", func(t *testing.T) {
		loc, err := ParseLocation("Germany")
		require.NoError(t, err)
		assert.Equal(t, LocationTypePolygon, loc.Type)
	})

	t.Run("GERMANY (uppercase) resolves to polygon via case-insensitive lookup", func(t *testing.T) {
		loc, err := ParseLocation("GERMANY")
		require.NoError(t, err)
		assert.Equal(t, LocationTypePolygon, loc.Type)
	})

	t.Run("de resolves to polygon via ISO_A2", func(t *testing.T) {
		LocationShortcutsMap["de"] = RectangleLocation{
			NorthEast: PointLocation{Lat: 55.06, Lon: 15.04},
			SouthWest: PointLocation{Lat: 47.27, Lon: 5.87},
		}
		defer delete(LocationShortcutsMap, "de")

		loc, err := ParseLocation("DE")
		require.NoError(t, err)
		assert.Equal(t, LocationTypePolygon, loc.Type)
	})

	t.Run("france resolves to polygon with multiple rings", func(t *testing.T) {
		LocationShortcutsMap["france"] = RectangleLocation{
			NorthEast: PointLocation{Lat: 51.09, Lon: 9.56},
			SouthWest: PointLocation{Lat: 41.34, Lon: -5.14},
		}
		defer delete(LocationShortcutsMap, "france")

		loc, err := ParseLocation("France")
		require.NoError(t, err)
		assert.Equal(t, LocationTypePolygon, loc.Type)
		poly, ok := loc.Payload.(PolygonLocation)
		require.True(t, ok)
		assert.Greater(t, len(poly.Rings), 1, "France MultiPolygon should have multiple rings")
	})
}

// TestParseLocation_PolygonFallback tests 4: shortcuts with no shape match
// fall back to RectangleLocation silently.
func TestParseLocation_PolygonFallback(t *testing.T) {
	LocationShortcutsMap = map[string]RectangleLocation{
		"alps": {
			NorthEast: PointLocation{Lat: 48.00, Lon: 16.00},
			SouthWest: PointLocation{Lat: 43.50, Lon: 6.00},
		},
	}
	ActiveShapeProvider = shapefile.NewShapeProvider(nil)
	defer func() { ActiveShapeProvider = nil }()

	// "Alps" is a geographic region, not a country in the Natural Earth dataset.
	loc, err := ParseLocation("Alps")
	require.NoError(t, err)
	assert.Equal(t, LocationTypeRectangle, loc.Type, "should fall back to rectangle")
	rect, ok := loc.Payload.(RectangleLocation)
	require.True(t, ok)
	assert.Equal(t, 48.00, rect.NorthEast.Lat)
}

// TestParseLocation_PolygonFallbackNilProvider tests that when ActiveShapeProvider
// is nil, shortcuts always resolve to RectangleLocation.
func TestParseLocation_PolygonFallbackNilProvider(t *testing.T) {
	LocationShortcutsMap = map[string]RectangleLocation{
		"germany": {
			NorthEast: PointLocation{Lat: 55.06, Lon: 15.04},
			SouthWest: PointLocation{Lat: 47.27, Lon: 5.87},
		},
	}
	ActiveShapeProvider = nil

	loc, err := ParseLocation("Germany")
	require.NoError(t, err)
	assert.Equal(t, LocationTypeRectangle, loc.Type, "nil provider should fall back to rectangle")
}

// TestExpandLocations_Polygon tests 5: ExpandLocations for polygon returns outer ring.
func TestExpandLocations_Polygon(t *testing.T) {
	outerRing := []PointLocation{
		{Lat: 55.0, Lon: 15.0},
		{Lat: 47.0, Lon: 15.0},
		{Lat: 47.0, Lon: 6.0},
		{Lat: 55.0, Lon: 6.0},
		{Lat: 55.0, Lon: 15.0},
	}
	innerRing := []PointLocation{
		{Lat: 50.0, Lon: 10.0},
		{Lat: 49.0, Lon: 10.0},
		{Lat: 49.0, Lon: 11.0},
		{Lat: 50.0, Lon: 10.0},
	}

	t.Run("polygon value returns outer ring", func(t *testing.T) {
		loc := Location{
			Type:    LocationTypePolygon,
			Payload: PolygonLocation{Rings: [][]PointLocation{outerRing, innerRing}},
		}
		pts := ExpandLocations(loc)
		assert.Equal(t, outerRing, pts, "should return only the outer ring")
	})

	t.Run("polygon pointer returns outer ring", func(t *testing.T) {
		loc := Location{
			Type:    LocationTypePolygon,
			Payload: &PolygonLocation{Rings: [][]PointLocation{outerRing}},
		}
		pts := ExpandLocations(loc)
		assert.Equal(t, outerRing, pts)
	})

	t.Run("polygon with no rings returns nil", func(t *testing.T) {
		loc := Location{
			Type:    LocationTypePolygon,
			Payload: PolygonLocation{Rings: [][]PointLocation{}},
		}
		pts := ExpandLocations(loc)
		assert.Nil(t, pts)
	})

	t.Run("nil polygon pointer returns nil", func(t *testing.T) {
		loc := Location{
			Type:    LocationTypePolygon,
			Payload: (*PolygonLocation)(nil),
		}
		pts := ExpandLocations(loc)
		assert.Nil(t, pts)
	})
}

// BUG-020: TestParseLocation_CompoundCountryNames tests that compound country shortcut
// names (e.g. "UnitedKingdom") resolve to polygon via the space-stripped lookup.
func TestParseLocation_CompoundCountryNames(t *testing.T) {
	LocationShortcutsMap = map[string]RectangleLocation{
		"unitedkingdom": {
			NorthEast: PointLocation{Lat: 60.85, Lon: 1.77},
			SouthWest: PointLocation{Lat: 49.86, Lon: -8.65},
		},
		"czechrepublic": {
			NorthEast: PointLocation{Lat: 51.06, Lon: 18.86},
			SouthWest: PointLocation{Lat: 48.55, Lon: 12.09},
		},
	}
	ActiveShapeProvider = shapefile.NewShapeProvider(nil)
	defer func() { ActiveShapeProvider = nil }()

	t.Run("UnitedKingdom resolves to polygon (case-insensitive)", func(t *testing.T) {
		loc, err := ParseLocation("UnitedKingdom")
		require.NoError(t, err)
		assert.Equal(t, LocationTypePolygon, loc.Type, "should resolve to polygon, not rectangle")
		poly, ok := loc.Payload.(PolygonLocation)
		require.True(t, ok)
		assert.NotEmpty(t, poly.Rings, "should have at least one ring")
	})

	t.Run("CzechRepublic resolves to polygon (case-insensitive)", func(t *testing.T) {
		loc, err := ParseLocation("CzechRepublic")
		require.NoError(t, err)
		assert.Equal(t, LocationTypePolygon, loc.Type, "should resolve to polygon, not rectangle")
		poly, ok := loc.Payload.(PolygonLocation)
		require.True(t, ok)
		assert.NotEmpty(t, poly.Rings, "should have at least one ring")
	})
}

// TestExistingShortcutStillWorks ensures 8: station shortcuts are unaffected.
func TestExistingShortcutStillWorks(t *testing.T) {
	StationMap = map[string]PointLocation{
		"wmo10381": {Lat: 52.47, Lon: 13.40},
	}
	ActiveShapeProvider = shapefile.NewShapeProvider(nil)
	defer func() { ActiveShapeProvider = nil }()

	loc, err := ParseLocation("wmo10381")
	require.NoError(t, err)
	assert.Equal(t, LocationTypePoint, loc.Type, "station should still resolve to point")
}

// TestCaseInsensitiveLocationShortcuts verifies that location shortcuts
// and station IDs are matched case-insensitively.
func TestCaseInsensitiveLocationShortcuts(t *testing.T) {
	LocationShortcutsMap = map[string]RectangleLocation{
		"germany": {NorthEast: PointLocation{Lat: 55.06, Lon: 15.04}, SouthWest: PointLocation{Lat: 47.27, Lon: 5.87}},
	}
	StationMap = map[string]PointLocation{
		"wmo10381": {Lat: 52.47, Lon: 13.40},
	}
	ActiveShapeProvider = nil

	t.Run("lowercase shortcut", func(t *testing.T) {
		loc, err := ParseLocation("germany")
		require.NoError(t, err)
		assert.Equal(t, LocationTypeRectangle, loc.Type)
	})

	t.Run("PascalCase shortcut", func(t *testing.T) {
		loc, err := ParseLocation("Germany")
		require.NoError(t, err)
		assert.Equal(t, LocationTypeRectangle, loc.Type)
	})

	t.Run("UPPERCASE shortcut", func(t *testing.T) {
		loc, err := ParseLocation("GERMANY")
		require.NoError(t, err)
		assert.Equal(t, LocationTypeRectangle, loc.Type)
	})

	t.Run("lowercase station ID", func(t *testing.T) {
		loc, err := ParseLocation("wmo10381")
		require.NoError(t, err)
		assert.Equal(t, LocationTypePoint, loc.Type)
	})

	t.Run("uppercase station ID", func(t *testing.T) {
		loc, err := ParseLocation("WMO10381")
		require.NoError(t, err)
		assert.Equal(t, LocationTypePoint, loc.Type)
	})

	t.Run("mixed-case station ID", func(t *testing.T) {
		loc, err := ParseLocation("Wmo10381")
		require.NoError(t, err)
		assert.Equal(t, LocationTypePoint, loc.Type)
	})

	t.Run("numeric coordinates unaffected by lowercasing", func(t *testing.T) {
		loc, err := ParseLocation("50.11,8.68")
		require.NoError(t, err)
		assert.Equal(t, LocationTypePoint, loc.Type)
		pt := loc.Payload.(PointLocation)
		assert.InDelta(t, 50.11, pt.Lat, 0.001)
		assert.InDelta(t, 8.68, pt.Lon, 0.001)
	})
}

// ---- 152: Rectangle Grid Polling Tests ----

// TestExpandRectangleToGrid_TierSelection tests 2: adaptive step selection.
func TestExpandRectangleToGrid_TierSelection(t *testing.T) {
	tests := []struct {
		name         string
		rect         RectangleLocation
		expectedStep float64
		description  string
	}{
		{
			name: "min dim >= 1.0 uses step 1.0",
			rect: RectangleLocation{
				SouthWest: PointLocation{Lat: 48, Lon: 11},
				NorthEast: PointLocation{Lat: 50, Lon: 13},
			},
			expectedStep: 1.0,
			description:  "2x2 degree rectangle, min dim=2.0",
		},
		{
			name: "min dim exactly 1.0 uses step 1.0",
			rect: RectangleLocation{
				SouthWest: PointLocation{Lat: 48, Lon: 11},
				NorthEast: PointLocation{Lat: 49, Lon: 14},
			},
			expectedStep: 1.0,
			description:  "1x3 degree rectangle, min dim=1.0",
		},
		{
			name: "min dim 0.7 uses step 0.5",
			rect: RectangleLocation{
				SouthWest: PointLocation{Lat: 48, Lon: 11},
				NorthEast: PointLocation{Lat: 48.7, Lon: 13},
			},
			expectedStep: 0.5,
			description:  "0.7x2 degree rectangle, min dim=0.7",
		},
		{
			name: "min dim exactly 0.5 uses step 0.5",
			rect: RectangleLocation{
				SouthWest: PointLocation{Lat: 48, Lon: 11},
				NorthEast: PointLocation{Lat: 48.5, Lon: 13},
			},
			expectedStep: 0.5,
			description:  "0.5x2 degree rectangle, min dim=0.5",
		},
		{
			name: "min dim 0.3 uses step 0.25",
			rect: RectangleLocation{
				SouthWest: PointLocation{Lat: 48, Lon: 11},
				NorthEast: PointLocation{Lat: 48.3, Lon: 13},
			},
			expectedStep: 0.25,
			description:  "0.3x2 degree rectangle, min dim=0.3",
		},
		{
			name: "min dim exactly 0.25 uses step 0.25",
			rect: RectangleLocation{
				SouthWest: PointLocation{Lat: 48, Lon: 11},
				NorthEast: PointLocation{Lat: 48.25, Lon: 13},
			},
			expectedStep: 0.25,
			description:  "0.25x2 degree rectangle, min dim=0.25",
		},
		{
			name: "min dim 0.15 uses step 0.1",
			rect: RectangleLocation{
				SouthWest: PointLocation{Lat: 48, Lon: 11},
				NorthEast: PointLocation{Lat: 48.15, Lon: 13},
			},
			expectedStep: 0.1,
			description:  "0.15x2 degree rectangle, min dim=0.15",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			latSpan := tt.rect.NorthEast.Lat - tt.rect.SouthWest.Lat
			lonSpan := tt.rect.NorthEast.Lon - tt.rect.SouthWest.Lon
			minDim := math.Min(latSpan, lonSpan)
			step := chooseGridStep(minDim)
			assert.Equal(t, tt.expectedStep, step, tt.description)
		})
	}
}

// TestExpandRectangleToGrid_BoundarySnap tests 3: NE corner always included.
func TestExpandRectangleToGrid_BoundarySnap(t *testing.T) {
	t.Run("NE corner included when not on step boundary", func(t *testing.T) {
		rect := RectangleLocation{
			SouthWest: PointLocation{Lat: 48, Lon: 11},
			NorthEast: PointLocation{Lat: 49.7, Lon: 12.3},
		}
		pts := expandRectangleToGrid(rect)

		// Verify SW corner is first.
		assert.Equal(t, 48.0, pts[0].Lat)
		assert.Equal(t, 11.0, pts[0].Lon)

		// Verify NE corner is last.
		last := pts[len(pts)-1]
		assert.Equal(t, 49.7, last.Lat)
		assert.Equal(t, 12.3, last.Lon)
	})

	t.Run("NE corner included when on step boundary", func(t *testing.T) {
		rect := RectangleLocation{
			SouthWest: PointLocation{Lat: 48, Lon: 11},
			NorthEast: PointLocation{Lat: 50, Lon: 13},
		}
		pts := expandRectangleToGrid(rect)

		// 2x2 at step 1.0 -> 3x3 = 9 points
		assert.Len(t, pts, 9)

		last := pts[len(pts)-1]
		assert.Equal(t, 50.0, last.Lat)
		assert.Equal(t, 13.0, last.Lon)
	})
}

// TestExpandRectangleToGrid_SilentCoarsening tests 5/6: coarsening when > 250 points.
func TestExpandRectangleToGrid_SilentCoarsening(t *testing.T) {
	t.Run("large rectangle coarsened to <= 250 points", func(t *testing.T) {
		// 50x50 degree rectangle. At step 1.0: 51x51 = 2601 > 250.
		// Doubled to 2.0: 26x26 = 676 > 250.
		// Doubled to 4.0: 13x13 = 169 <= 250.
		rect := RectangleLocation{
			SouthWest: PointLocation{Lat: 0, Lon: 0},
			NorthEast: PointLocation{Lat: 50, Lon: 50},
		}
		pts := expandRectangleToGrid(rect)
		assert.LessOrEqual(t, len(pts), maxGridPoints, "should be coarsened to <= 250 points")
		assert.Greater(t, len(pts), 0, "should have at least one point")

		// Verify SW and NE corners are present.
		assert.Equal(t, 0.0, pts[0].Lat)
		assert.Equal(t, 0.0, pts[0].Lon)
		assert.Equal(t, 50.0, pts[len(pts)-1].Lat)
		assert.Equal(t, 50.0, pts[len(pts)-1].Lon)
	})

	t.Run("medium rectangle no coarsening needed", func(t *testing.T) {
		// 5x5 at step 1.0: 6x6 = 36 <= 250.
		rect := RectangleLocation{
			SouthWest: PointLocation{Lat: 48, Lon: 11},
			NorthEast: PointLocation{Lat: 53, Lon: 16},
		}
		pts := expandRectangleToGrid(rect)
		assert.Len(t, pts, 36)
	})
}

// TestExpandRectangleToGrid_DegenerateCases tests edge cases (13, degenerate rectangles).
func TestExpandRectangleToGrid_DegenerateCases(t *testing.T) {
	t.Run("single point (lat_span=0, lon_span=0)", func(t *testing.T) {
		rect := RectangleLocation{
			SouthWest: PointLocation{Lat: 48.5, Lon: 11.5},
			NorthEast: PointLocation{Lat: 48.5, Lon: 11.5},
		}
		pts := expandRectangleToGrid(rect)
		assert.Len(t, pts, 1)
		assert.Equal(t, 48.5, pts[0].Lat)
		assert.Equal(t, 11.5, pts[0].Lon)
	})

	t.Run("horizontal line (lat_span=0)", func(t *testing.T) {
		rect := RectangleLocation{
			SouthWest: PointLocation{Lat: 48.0, Lon: 11.0},
			NorthEast: PointLocation{Lat: 48.0, Lon: 13.0},
		}
		pts := expandRectangleToGrid(rect)
		// lon_span=2.0, lat_span=0 -> min_dim=0 -> step 0.1
		assert.Greater(t, len(pts), 1, "should produce multiple points along the line")
		for _, p := range pts {
			assert.Equal(t, 48.0, p.Lat, "all points should have same lat")
		}
		assert.Equal(t, 11.0, pts[0].Lon)
		assert.Equal(t, 13.0, pts[len(pts)-1].Lon)
	})

	t.Run("vertical line (lon_span=0)", func(t *testing.T) {
		rect := RectangleLocation{
			SouthWest: PointLocation{Lat: 48.0, Lon: 11.0},
			NorthEast: PointLocation{Lat: 50.0, Lon: 11.0},
		}
		pts := expandRectangleToGrid(rect)
		assert.Greater(t, len(pts), 1, "should produce multiple points along the line")
		for _, p := range pts {
			assert.Equal(t, 11.0, p.Lon, "all points should have same lon")
		}
		assert.Equal(t, 48.0, pts[0].Lat)
		assert.Equal(t, 50.0, pts[len(pts)-1].Lat)
	})

	t.Run("tiny rectangle smaller than step", func(t *testing.T) {
		// 0.05x0.05 at step 0.1 -> still includes both corners via boundary snap.
		rect := RectangleLocation{
			SouthWest: PointLocation{Lat: 48.0, Lon: 11.0},
			NorthEast: PointLocation{Lat: 48.05, Lon: 11.05},
		}
		pts := expandRectangleToGrid(rect)
		// lats: [48.0, 48.05], lons: [11.0, 11.05] -> 4 points
		assert.Len(t, pts, 4)
	})
}

// TestChooseGridStep tests the tier table directly.
func TestChooseGridStep(t *testing.T) {
	assert.Equal(t, 1.0, chooseGridStep(10.0))
	assert.Equal(t, 1.0, chooseGridStep(1.0))
	assert.Equal(t, 0.5, chooseGridStep(0.99))
	assert.Equal(t, 0.5, chooseGridStep(0.5))
	assert.Equal(t, 0.25, chooseGridStep(0.49))
	assert.Equal(t, 0.25, chooseGridStep(0.25))
	assert.Equal(t, 0.1, chooseGridStep(0.24))
	assert.Equal(t, 0.1, chooseGridStep(0.05))
	assert.Equal(t, 0.1, chooseGridStep(0.0))
}

// TestGridPointCount tests the point count estimator.
func TestGridPointCount(t *testing.T) {
	assert.Equal(t, 9, gridPointCount(2.0, 2.0, 1.0))
	assert.Equal(t, 2601, gridPointCount(50.0, 50.0, 1.0))
	assert.Equal(t, 196, gridPointCount(50.0, 50.0, 4.0))
}

// TestSteppedValues tests the stepped value generation with boundary snap.
func TestSteppedValues(t *testing.T) {
	t.Run("exact steps", func(t *testing.T) {
		vals := steppedValues(0, 3, 1.0)
		assert.Equal(t, []float64{0, 1, 2, 3}, vals)
	})

	t.Run("boundary snap when max not on step", func(t *testing.T) {
		vals := steppedValues(0, 2.5, 1.0)
		assert.Equal(t, []float64{0, 1, 2, 2.5}, vals)
	})

	t.Run("single value when min equals max", func(t *testing.T) {
		vals := steppedValues(5.0, 5.0, 1.0)
		assert.Equal(t, []float64{5.0}, vals)
	})
}

// ---------------------------------------------------------------------------
// Bbox tests (MMA-163)
// ---------------------------------------------------------------------------

// TestParseBbox tests the parseBbox function that parses "west,south,east,north" strings.
func TestParseBbox(t *testing.T) {
	t.Run("valid input", func(t *testing.T) {
		bbox, err := parseBbox("5.0,47.0,15.0,55.0")
		require.NoError(t, err)
		assert.Equal(t, 5.0, bbox.West)
		assert.Equal(t, 47.0, bbox.South)
		assert.Equal(t, 15.0, bbox.East)
		assert.Equal(t, 55.0, bbox.North)
	})

	t.Run("missing values", func(t *testing.T) {
		_, err := parseBbox("5.0,47.0")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires exactly 4 values")
	})

	t.Run("south >= north", func(t *testing.T) {
		_, err := parseBbox("5.0,55.0,15.0,47.0")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "south")
		assert.Contains(t, err.Error(), "north")
	})

	t.Run("west >= east", func(t *testing.T) {
		_, err := parseBbox("15.0,47.0,5.0,55.0")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "west")
		assert.Contains(t, err.Error(), "east")
	})

	t.Run("non-numeric west value", func(t *testing.T) {
		_, err := parseBbox("abc,47.0,15.0,55.0")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "west")
	})

	t.Run("empty string", func(t *testing.T) {
		_, err := parseBbox("")
		require.Error(t, err)
	})
}

// TestExpandBboxToGrid tests the ExpandBboxToGrid function that expands a bbox to a grid.
func TestExpandBboxToGrid(t *testing.T) {
	t.Run("normal bbox 10x8 at density 1", func(t *testing.T) {
		bbox := BboxLocation{West: 5, South: 47, East: 15, North: 55}
		pts, rows, cols := ExpandBboxToGrid(bbox, 1)

		assert.Equal(t, 9, rows)
		assert.Equal(t, 11, cols)
		assert.Equal(t, 99, len(pts))
	})

	t.Run("first point is NW corner last point is SE corner", func(t *testing.T) {
		bbox := BboxLocation{West: 5, South: 47, East: 15, North: 55}
		pts, _, _ := ExpandBboxToGrid(bbox, 1)

		assert.Equal(t, 55.0, pts[0].Lat)
		assert.Equal(t, 5.0, pts[0].Lon)

		last := pts[len(pts)-1]
		assert.Equal(t, 47.0, last.Lat)
		assert.Equal(t, 15.0, last.Lon)
	})

	t.Run("density 1 on 1x1 degree bbox", func(t *testing.T) {
		bbox := BboxLocation{West: 10, South: 50, East: 11, North: 51}
		pts, rows, cols := ExpandBboxToGrid(bbox, 1)

		assert.Equal(t, 2, rows)
		assert.Equal(t, 2, cols)
		assert.Equal(t, 4, len(pts))
	})

	t.Run("small bbox 0.5x0.5 at density 10", func(t *testing.T) {
		bbox := BboxLocation{West: 10, South: 50, East: 10.5, North: 50.5}
		pts, rows, cols := ExpandBboxToGrid(bbox, 10)

		assert.Equal(t, 6, rows)
		assert.Equal(t, 6, cols)
		assert.Equal(t, 36, len(pts))
	})
}

// TestParseLocationBbox tests ParseLocation with the "bbox:" prefix.
func TestParseLocationBbox(t *testing.T) {
	// Clear shortcuts/stations to avoid interference.
	StationMap = map[string]PointLocation{}
	LocationShortcutsMap = map[string]RectangleLocation{}

	t.Run("lowercase bbox prefix", func(t *testing.T) {
		loc, err := ParseLocation("bbox:5.0,47.0,15.0,55.0")
		require.NoError(t, err)
		assert.Equal(t, LocationTypeBbox, loc.Type)
		bbox, ok := loc.Payload.(BboxLocation)
		require.True(t, ok, "payload should be BboxLocation")
		assert.Equal(t, 5.0, bbox.West)
		assert.Equal(t, 47.0, bbox.South)
		assert.Equal(t, 15.0, bbox.East)
		assert.Equal(t, 55.0, bbox.North)
	})

	t.Run("uppercase BBOX prefix is case-insensitive", func(t *testing.T) {
		loc, err := ParseLocation("BBOX:5.0,47.0,15.0,55.0")
		require.NoError(t, err)
		assert.Equal(t, LocationTypeBbox, loc.Type)
		bbox, ok := loc.Payload.(BboxLocation)
		require.True(t, ok, "payload should be BboxLocation")
		assert.Equal(t, 5.0, bbox.West)
		assert.Equal(t, 47.0, bbox.South)
		assert.Equal(t, 15.0, bbox.East)
		assert.Equal(t, 55.0, bbox.North)
	})

	t.Run("bbox prefix with invalid content", func(t *testing.T) {
		_, err := ParseLocation("bbox:invalid")
		require.Error(t, err)
	})
}
