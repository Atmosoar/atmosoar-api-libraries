package location

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseLocationPrefixedForms covers the explicit `type:value` location
// strings accepted by ParseLocation alongside the bare forms.
func TestParseLocationPrefixedForms(t *testing.T) {
	t.Run("point prefix", func(t *testing.T) {
		loc, err := ParseLocation("point:55.75,37.6")
		require.NoError(t, err)
		assert.Equal(t, LocationTypePoint, loc.Type)
		assert.Equal(t, PointLocation{Lat: 55.75, Lon: 37.6}, loc.Payload)
	})

	t.Run("point prefix is case-insensitive", func(t *testing.T) {
		loc, err := ParseLocation("POINT:1.5,2.5")
		require.NoError(t, err)
		assert.Equal(t, LocationTypePoint, loc.Type)
		assert.Equal(t, PointLocation{Lat: 1.5, Lon: 2.5}, loc.Payload)
	})

	t.Run("polyline prefix with semicolons", func(t *testing.T) {
		loc, err := ParseLocation("polyline:55.7,37.6;55.8,37.7")
		require.NoError(t, err)
		assert.Equal(t, LocationTypePolyline, loc.Type)
		pl, ok := loc.Payload.(PolylineLocation)
		require.True(t, ok)
		assert.Len(t, pl.Coordinates, 2)
	})

	t.Run("rectangle prefix four-value form", func(t *testing.T) {
		loc, err := ParseLocation("rectangle:44.5,11.6,55.2,33.4")
		require.NoError(t, err)
		assert.Equal(t, LocationTypeRectangle, loc.Type)
		rect, ok := loc.Payload.(RectangleLocation)
		require.True(t, ok)
		assert.Equal(t, PointLocation{Lat: 44.5, Lon: 11.6}, rect.SouthWest)
		assert.Equal(t, PointLocation{Lat: 55.2, Lon: 33.4}, rect.NorthEast)
	})

	t.Run("radius prefix", func(t *testing.T) {
		loc, err := ParseLocation("radius:50.0,8.0|25")
		require.NoError(t, err)
		assert.Equal(t, LocationTypeRadius, loc.Type)
		rad, ok := loc.Payload.(RadiusLocation)
		require.True(t, ok)
		assert.Equal(t, 25.0, rad.Radius)
	})

	t.Run("point prefix rejects a malformed value", func(t *testing.T) {
		_, err := ParseLocation("point:nope")
		require.Error(t, err)
	})

	t.Run("point prefix rejects multiple points", func(t *testing.T) {
		_, err := ParseLocation("point:1,2|3,4")
		require.Error(t, err)
	})
}

// TestLocationUnmarshalJSON covers both wire forms accepted by the custom
// Location JSON decoder.
func TestLocationUnmarshalJSON(t *testing.T) {
	t.Run("string form", func(t *testing.T) {
		var loc Location
		require.NoError(t, json.Unmarshal([]byte(`"point:47.37,8.55"`), &loc))
		assert.Equal(t, LocationTypePoint, loc.Type)
		assert.Equal(t, PointLocation{Lat: 47.37, Lon: 8.55}, loc.Payload)
	})

	t.Run("string form bbox", func(t *testing.T) {
		var loc Location
		require.NoError(t, json.Unmarshal([]byte(`"bbox:-5,50,2,55"`), &loc))
		assert.Equal(t, LocationTypeBbox, loc.Type)
		_, ok := loc.Payload.(BboxLocation)
		assert.True(t, ok)
	})

	t.Run("object form decodes into a typed payload", func(t *testing.T) {
		var loc Location
		body := `{"type":"point","payload":{"lat":47.37,"lon":8.55}}`
		require.NoError(t, json.Unmarshal([]byte(body), &loc))
		assert.Equal(t, LocationTypePoint, loc.Type)
		// The payload must be the concrete struct, not a map, so downstream
		// type assertions in ExpandLocations succeed.
		pt, ok := loc.Payload.(PointLocation)
		require.True(t, ok)
		assert.Equal(t, PointLocation{Lat: 47.37, Lon: 8.55}, pt)
	})

	t.Run("object form bbox", func(t *testing.T) {
		var loc Location
		body := `{"type":"bbox","payload":{"west":-5,"south":50,"east":2,"north":55}}`
		require.NoError(t, json.Unmarshal([]byte(body), &loc))
		bbox, ok := loc.Payload.(BboxLocation)
		require.True(t, ok)
		assert.Equal(t, 55.0, bbox.North)
	})

	t.Run("both forms expand to the same point", func(t *testing.T) {
		var fromString, fromObject Location
		require.NoError(t, json.Unmarshal([]byte(`"point:1.5,2.5"`), &fromString))
		require.NoError(t, json.Unmarshal(
			[]byte(`{"type":"point","payload":{"lat":1.5,"lon":2.5}}`), &fromObject))
		assert.Equal(t, ExpandLocations(fromString), ExpandLocations(fromObject))
	})

	t.Run("round-trips through the default marshaller", func(t *testing.T) {
		var original Location
		require.NoError(t, json.Unmarshal([]byte(`"point:9,9"`), &original))
		encoded, err := json.Marshal(original)
		require.NoError(t, err)
		var decoded Location
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		assert.Equal(t, original, decoded)
	})

	t.Run("rejects null", func(t *testing.T) {
		var loc Location
		require.Error(t, json.Unmarshal([]byte(`null`), &loc))
	})

	t.Run("rejects an unparseable string", func(t *testing.T) {
		var loc Location
		require.Error(t, json.Unmarshal([]byte(`"point:nope"`), &loc))
	})

	t.Run("rejects an unknown object type", func(t *testing.T) {
		var loc Location
		require.Error(t, json.Unmarshal([]byte(`{"type":"galaxy","payload":{}}`), &loc))
	})

	t.Run("rejects an object missing its payload", func(t *testing.T) {
		var loc Location
		require.Error(t, json.Unmarshal([]byte(`{"type":"point"}`), &loc))
	})
}
