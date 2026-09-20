package chart

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mmaThreshold is the Multi-Model API's marker table as it stood before this
// package existed: two edges per parameter, green / yellow / red, with
// visibility and cloud base inverted.
type mmaThreshold struct {
	greenMax  float64
	yellowMax float64
	inverted  bool
}

var mmaThresholds = map[string]mmaThreshold{
	"10m_wind_speed":      {greenMax: 10, yellowMax: 20},
	"wind_speed":          {greenMax: 10, yellowMax: 20},
	"wind_speed_10m":      {greenMax: 10, yellowMax: 20},
	"temperature_2m":      {greenMax: 30, yellowMax: 35},
	"2m_t":                {greenMax: 30, yellowMax: 35},
	"precip":              {greenMax: 1, yellowMax: 5},
	"precipitation":       {greenMax: 1, yellowMax: 5},
	"total_precipitation": {greenMax: 1, yellowMax: 5},
	"visibility":          {greenMax: 10, yellowMax: 5, inverted: true},
	"cloud_base":          {greenMax: 1000, yellowMax: 500, inverted: true},
	"2m_rel_humidity":     {greenMax: 80, yellowMax: 95},
	"icing_risk":          {greenMax: 20, yellowMax: 50},
}

// mmaClassify reproduces the old marker logic exactly.
func mmaClassify(th mmaThreshold, v float64) Status {
	if th.inverted {
		switch {
		case v >= th.greenMax:
			return StatusGood
		case v >= th.yellowMax:
			return StatusWarning
		default:
			return StatusCritical
		}
	}
	switch {
	case v <= th.greenMax:
		return StatusGood
	case v <= th.yellowMax:
		return StatusWarning
	default:
		return StatusCritical
	}
}

// TestClassifyMatchesMMATable is the parity gate: a value that was amber on a
// forecast map must be amber on a chart. Only the hexes changed when the table
// moved here, never the banding.
func TestClassifyMatchesMMATable(t *testing.T) {
	t.Parallel()
	for name, th := range mmaThresholds {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Sweep across, through and past both edges, including the exact
			// boundaries, where an off-by-one in the comparison would hide.
			for _, v := range []float64{
				-50, -1, 0, 0.5, 1, 4.9, 5, 5.1, 9.9, 10, 10.1, 19.9, 20, 20.1,
				29.9, 30, 30.1, 34.9, 35, 35.1, 49.9, 50, 50.1, 79.9, 80, 80.1,
				94.9, 95, 95.1, 499, 500, 501, 999, 1000, 1001, 5000,
			} {
				assert.Equal(t, mmaClassify(th, v), Classify(name, v),
					"%s at %v", name, v)
			}
		})
	}
}

func TestClassifyUnknownAndUnbanded(t *testing.T) {
	t.Parallel()
	// An unregistered parameter is not an error, and pressure is honestly
	// neither good nor bad.
	assert.Equal(t, StatusNone, Classify("teacups_per_hour", 3))
	assert.Equal(t, StatusNone, Classify("mslp", 1013))
	assert.Equal(t, StatusNone, Classify("dewpoint", 9))
}

func TestParameterAliasesResolveToCanonical(t *testing.T) {
	t.Parallel()
	for alias, canonical := range map[string]string{
		"2m_t":                "temperature_2m",
		"TEMPERATURE":         "temperature_2m",
		"  10m_wind_speed  ":  "wind_speed",
		"wind_speed_10m":      "wind_speed",
		"2m_rel_humidity":     "relative_humidity",
		"total_precipitation": "precipitation",
		"pressure_msl":        "mslp",
		"ceiling":             "cloud_base",
		"delta_p":             "pressure_difference",
	} {
		def, ok := Parameter(alias)
		require.True(t, ok, alias)
		assert.Equal(t, canonical, def.Key, alias)
	}

	_, ok := Parameter("")
	assert.False(t, ok)
	_, ok = Parameter("teacups")
	assert.False(t, ok)
}

func TestParameterUnitsAreSI(t *testing.T) {
	t.Parallel()
	// The registry speaks the observation package's vocabulary; a service that
	// converts on the way in gets the same axis label as one that does not.
	for param, unit := range map[string]string{
		"temperature_2m":     "°C",
		"relative_humidity":  "%",
		"wind_speed":         "m/s",
		"wind_direction":     "°",
		"mslp":               "hPa",
		"precipitation":      "mm",
		"precipitation_rate": "mm/h",
		"solar_radiation":    "W/m²",
		"uv_index":           "",
	} {
		def, ok := Parameter(param)
		require.True(t, ok, param)
		assert.Equal(t, unit, def.Unit, param)
	}
}

func TestBandsForShadesOnlyTheWarnings(t *testing.T) {
	t.Parallel()
	bands := BandsFor("wind_speed")
	require.Len(t, bands, 2, "the comfortable range is not shaded")

	assert.Equal(t, StatusWarning, bands[0].Status)
	require.NotNil(t, bands[0].From)
	require.NotNil(t, bands[0].To)
	assert.InDelta(t, 10.0, *bands[0].From, 1e-9)
	assert.InDelta(t, 20.0, *bands[0].To, 1e-9)
	assert.Contains(t, bands[0].Label, "caution")
	assert.Contains(t, bands[0].Label, "m/s")

	assert.Equal(t, StatusCritical, bands[1].Status)
	require.NotNil(t, bands[1].From)
	assert.Nil(t, bands[1].To, "the worst band is open-ended")
	assert.InDelta(t, 20.0, *bands[1].From, 1e-9)

	assert.Nil(t, BandsFor("mslp"), "a parameter with no thresholds gets no bands")
	assert.Nil(t, BandsFor("teacups"))
}

// TestBandsForInvertedParameterOpensDownward covers visibility and cloud base,
// where the open-ended worst band runs toward zero rather than upward.
func TestBandsForInvertedParameterOpensDownward(t *testing.T) {
	t.Parallel()
	bands := BandsFor("visibility")
	require.Len(t, bands, 2)

	assert.Equal(t, StatusWarning, bands[0].Status)
	assert.InDelta(t, 5.0, *bands[0].From, 1e-9)
	assert.InDelta(t, 10.0, *bands[0].To, 1e-9)

	assert.Equal(t, StatusCritical, bands[1].Status)
	assert.Nil(t, bands[1].From)
	require.NotNil(t, bands[1].To)
	assert.InDelta(t, 5.0, *bands[1].To, 1e-9)
	assert.Contains(t, bands[1].Label, "≤")
}

func TestFormatValueUsesParameterPrecision(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "12.3", FormatValue("temperature_2m", 12.34))
	assert.Equal(t, "83", FormatValue("relative_humidity", 82.6))
	assert.Equal(t, "1013.2", FormatValue("mslp", 1013.24))
	assert.Equal(t, "3.5", FormatValue("teacups", 3.45), "unknown falls back to one place")
}

func TestParameterLabelFallsBackToTheName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "Wind speed", ParameterLabel("10m_wind_speed"))
	assert.Equal(t, "teacups", ParameterLabel("teacups"))
}

func TestStatusSeverityRanksUnknownBelowGood(t *testing.T) {
	t.Parallel()
	assert.Less(t, StatusNone.Severity(), StatusGood.Severity())
	assert.Less(t, StatusGood.Severity(), StatusWarning.Severity())
	assert.Less(t, StatusWarning.Severity(), StatusSerious.Severity())
	assert.Less(t, StatusSerious.Severity(), StatusCritical.Severity())
	assert.Equal(t, 0, Status("nonsense").Severity())
}
