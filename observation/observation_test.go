package observation

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnitConversions(t *testing.T) {
	cases := []struct {
		name string
		got  float64
		want float64
	}{
		{"freezing point", FahrenheitToCelsius(32), 0},
		{"body temperature", FahrenheitToCelsius(98.6), 37},
		{"minus forty", FahrenheitToCelsius(-40), -40},
		{"10 mph", MphToMps(10), 4.4704},
		{"36 km/h", KmhToMps(36), 10},
		{"1 knot", KnotsToMps(1), 0.514444},
		{"standard atmosphere", InHgToHPa(29.92), 1013.21},
		{"pascal", PaToHPa(101325), 1013.25},
		{"one inch", InchToMm(1), 25.4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.InDelta(t, tc.want, tc.got, 0.01)
		})
	}
}

func TestDewpointMagnus(t *testing.T) {
	assert.InDelta(t, 20, DewpointMagnus(20, 100), 0.01, "saturated air: dewpoint equals temperature")
	assert.InDelta(t, 9.26, DewpointMagnus(20, 50), 0.05)
	assert.True(t, math.IsNaN(DewpointMagnus(20, 0)))
	assert.True(t, math.IsNaN(DewpointMagnus(20, 101)))
}

func TestRound(t *testing.T) {
	assert.InDelta(t, 1.23, Round(1.23456, 2), 1e-9)
	assert.InDelta(t, -4.5, Round(-4.46, 1), 1e-9)
}

func TestHasMeasurement(t *testing.T) {
	o := Observation{Lat: Float(47.3), Lon: Float(8.5)}
	assert.False(t, o.HasMeasurement(), "position alone is not a reading")
	o.WindSpeed = Float(0)
	assert.True(t, o.HasMeasurement(), "a reported calm is a reading")
}

func TestSanitize(t *testing.T) {
	o := Observation{
		Temperature2m:    Float(71.6), // a °F value stored as °C
		RelativeHumidity: Float(55),
		SurfacePressure:  Float(math.NaN()),
		WindDirection:    Float(360),
		Extra:            map[string]any{"wh65batt": 0},
	}
	violations := o.Sanitize(DefaultBounds())

	require.Len(t, violations, 2)
	assert.Equal(t, "temperature_2m", violations[0].Field)
	assert.Equal(t, "out of range", violations[0].Reason)
	assert.Equal(t, "surface_pressure", violations[1].Field)
	assert.Equal(t, "not finite", violations[1].Reason)

	assert.Nil(t, o.Temperature2m)
	assert.Nil(t, o.SurfacePressure)
	assert.InDelta(t, 55, *o.RelativeHumidity, 0)
	assert.InDelta(t, 360, *o.WindDirection, 0, "bounds are inclusive")

	_, err := json.Marshal(violations)
	require.NoError(t, err, "violations must stay JSON-encodable")
}

func TestJSONFieldNames(t *testing.T) {
	o := Observation{
		StationID:     "st-1",
		Source:        "ecowitt",
		Timestamp:     time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC),
		Temperature2m: Float(11.4),
		WindGust:      Float(8.2),
	}
	raw, err := json.Marshal(o)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	assert.InDelta(t, 11.4, m["temperature_2m"], 0)
	assert.InDelta(t, 8.2, m["wind_gust"], 0)
	assert.Nil(t, m["relative_humidity"], "unreported fields encode as null")
	assert.NotContains(t, m, "extra", "empty extra is omitted")
}
