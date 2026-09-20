package chart

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roseOf builds a rose from direction/speed pairs.
func roseOf(t *testing.T, pairs [][2]float64, opts *WindroseOptions) Spec {
	t.Helper()
	start := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	points := make([]Point, 0, len(pairs))
	for i, p := range pairs {
		points = append(points, Point{
			Time:      start.Add(time.Duration(i) * time.Minute),
			Direction: Float(p[0]),
			Value:     Float(p[1]),
		})
	}
	spec := Windrose("rose", SeriesOf("wind", "wind_speed", points), opts)
	resolved, err := spec.Resolved()
	require.NoError(t, err)
	return resolved
}

func TestRoseSectorsAreCentredOnTheirBearing(t *testing.T) {
	t.Parallel()
	// With 16 sectors each spans 22.5°, so the north sector runs 348.75..11.25.
	// A wind from 349° is northerly, not north-north-westerly.
	for _, dir := range []float64{0, 5, 355, 349, 11} {
		resolved := roseOf(t, [][2]float64{{dir, 5}}, nil)
		require.Len(t, resolved.Windrose.Bins, 1, "direction %v", dir)
		assert.Equal(t, 0, resolved.Windrose.Bins[0].Sector, "direction %v should be north", dir)
	}

	// And the neighbours land where a compass says they should.
	for dir, sector := range map[float64]int{90: 4, 180: 8, 270: 12, 22.5: 1, 337.5: 15} {
		resolved := roseOf(t, [][2]float64{{dir, 5}}, nil)
		require.Len(t, resolved.Windrose.Bins, 1)
		assert.Equal(t, sector, resolved.Windrose.Bins[0].Sector, "direction %v", dir)
	}
}

func TestRoseNormalisesOutOfRangeBearings(t *testing.T) {
	t.Parallel()
	for _, dir := range []float64{360, 720, -90, 450} {
		resolved := roseOf(t, [][2]float64{{dir, 5}}, nil)
		require.Len(t, resolved.Windrose.Bins, 1, "direction %v", dir)
		assert.GreaterOrEqual(t, resolved.Windrose.Bins[0].Sector, 0)
		assert.Less(t, resolved.Windrose.Bins[0].Sector, defaultSectors)
	}
}

func TestRoseBinsSpeedsIntoClasses(t *testing.T) {
	t.Parallel()
	// Default edges are 2, 4, 6, 10, 15, so six classes including the open top.
	for speed, band := range map[float64]int{1.5: 0, 3: 1, 5: 2, 8: 3, 12: 4, 30: 5} {
		resolved := roseOf(t, [][2]float64{{270, speed}}, nil)
		require.Len(t, resolved.Windrose.Bins, 1, "speed %v", speed)
		assert.Equal(t, band, resolved.Windrose.Bins[0].Band, "speed %v", speed)
	}
	// An edge belongs to the class above it: 4 m/s is "4–6", not "2–4".
	resolved := roseOf(t, [][2]float64{{270, 4}}, nil)
	assert.Equal(t, 2, resolved.Windrose.Bins[0].Band)
}

func TestRoseCountsCalmSeparately(t *testing.T) {
	t.Parallel()
	// A direction reported at 0.1 m/s is noise, so calm readings go to the
	// centre rather than inventing a prevailing wind.
	resolved := roseOf(t, [][2]float64{
		{270, 0.1}, {270, 0.2}, {90, 5}, {90, 5},
	}, nil)

	assert.Equal(t, 4, resolved.Windrose.Total)
	assert.InDelta(t, 0.5, resolved.Windrose.CalmFraction, 1e-9)
	require.Len(t, resolved.Windrose.Bins, 1, "only the two moving readings are binned")
	assert.Equal(t, 2, resolved.Windrose.Bins[0].Count)
	assert.InDelta(t, 0.5, resolved.Windrose.Bins[0].Fraction, 1e-9)
}

func TestRoseFractionsSumToOne(t *testing.T) {
	t.Parallel()
	resolved := roseOf(t, [][2]float64{
		{0, 3}, {45, 5}, {90, 7}, {135, 12}, {180, 20}, {225, 1}, {270, 8}, {315, 4},
	}, nil)

	total := resolved.Windrose.CalmFraction
	for _, bin := range resolved.Windrose.Bins {
		total += bin.Fraction
	}
	assert.InDelta(t, 1.0, total, 1e-9, "every reading is accounted for exactly once")
	assert.LessOrEqual(t, resolved.Windrose.MaxFraction, 1.0)
	assert.Positive(t, resolved.Windrose.MaxFraction)
}

func TestRoseSpeedBandsWearTheSequentialRamp(t *testing.T) {
	t.Parallel()
	resolved := roseOf(t, [][2]float64{{270, 5}}, nil)
	opts := resolved.Windrose

	require.Len(t, opts.BandColors, len(defaultSpeedBands)+1)
	require.Len(t, opts.BandLabels, len(defaultSpeedBands)+1)
	assert.Equal(t, "0–2 m/s", opts.BandLabels[0])
	assert.Equal(t, "≥ 15 m/s", opts.BandLabels[len(opts.BandLabels)-1])

	// Speed is a magnitude, so the classes take one hue's ramp — never the
	// categorical slots, which encode identity.
	for _, hex := range opts.BandColors {
		assert.Contains(t, PaletteFor(ModeLight).Sequential, hex)
		assert.NotContains(t, PaletteFor(ModeLight).Series, hex)
	}
}

func TestRoseRingsAreCleanPercentages(t *testing.T) {
	t.Parallel()
	resolved := roseOf(t, [][2]float64{{270, 5}, {270, 6}, {90, 5}, {0, 5}}, nil)
	require.NotEmpty(t, resolved.Windrose.Rings)

	last := resolved.Windrose.Rings[len(resolved.Windrose.Rings)-1]
	assert.GreaterOrEqual(t, last.Value/100, resolved.Windrose.MaxFraction,
		"the outer ring must contain the fullest sector")
	for _, ring := range resolved.Windrose.Rings {
		assert.Contains(t, ring.Label, "%")
	}
}

func TestRoseHonoursCustomSectorsAndBands(t *testing.T) {
	t.Parallel()
	resolved := roseOf(t, [][2]float64{{90, 12}}, &WindroseOptions{
		Sectors:    8,
		SpeedBands: []float64{5, 10},
		CalmBelow:  1,
	})
	assert.Equal(t, 8, resolved.Windrose.Sectors)
	require.Len(t, resolved.Windrose.BandLabels, 3)
	assert.Equal(t, 2, resolved.Windrose.Bins[0].Band, "12 m/s is above the last edge")
	assert.Equal(t, 2, resolved.Windrose.Bins[0].Sector, "east of eight sectors")
}

func TestWindroseValidation(t *testing.T) {
	t.Parallel()
	now := time.Now()
	good := []Point{{Time: now, Value: Float(5), Direction: Float(270)}}

	cases := map[string]struct {
		spec Spec
		want string
	}{
		"no direction": {
			Windrose("r", Series{Name: "w", Points: []Point{{Time: now, Value: Float(5)}}}, nil),
			"direction and a speed",
		},
		"no speed": {
			Windrose("r", Series{Name: "w", Points: []Point{{Time: now, Direction: Float(90)}}}, nil),
			"direction and a speed",
		},
		"sectors do not divide 360": {
			Windrose("r", Series{Name: "w", Points: good}, &WindroseOptions{Sectors: 7}),
			"must divide 360",
		},
		"too few sectors": {
			Windrose("r", Series{Name: "w", Points: good}, &WindroseOptions{Sectors: 2}),
			"must divide 360",
		},
		"descending bands": {
			Windrose("r", Series{Name: "w", Points: good}, &WindroseOptions{SpeedBands: []float64{5, 3}}),
			"must ascend",
		},
		"negative band": {
			Windrose("r", Series{Name: "w", Points: good}, &WindroseOptions{SpeedBands: []float64{-1, 3}}),
			"must be positive",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := tc.spec.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}

	twoSeries := Spec{Kind: KindWindrose, Panels: []Panel{{Series: []Series{
		{Name: "a", Points: good}, {Name: "b", Points: good},
	}}}}
	err := twoSeries.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one series")
}

func TestPolarPutsNorthUpAndRunsClockwise(t *testing.T) {
	t.Parallel()
	c := Pt{X: 100, Y: 100}
	north := polar(c, 50, 0)
	east := polar(c, 50, 90)
	south := polar(c, 50, 180)
	west := polar(c, 50, 270)

	assert.InDelta(t, 100.0, north.X, 1e-9)
	assert.InDelta(t, 50.0, north.Y, 1e-9, "north is up, and up is a smaller y")
	assert.InDelta(t, 150.0, east.X, 1e-9)
	assert.InDelta(t, 150.0, south.Y, 1e-9)
	assert.InDelta(t, 50.0, west.X, 1e-9)
}

func TestWedgeIsAClosedAnnularSector(t *testing.T) {
	t.Parallel()
	c := Pt{X: 0, Y: 0}
	pts := wedge(c, 10, 20, -10, 10)
	require.NotEmpty(t, pts)
	assert.Equal(t, 0, len(pts)%2, "the outer arc and the inner arc have equal point counts")

	for i, p := range pts {
		r := math.Hypot(p.X, p.Y)
		if i < len(pts)/2 {
			assert.InDelta(t, 20.0, r, 1e-6, "outer arc")
			continue
		}
		assert.InDelta(t, 10.0, r, 1e-6, "inner arc")
	}
}

func TestSpeedBandLabelsWithoutAUnit(t *testing.T) {
	t.Parallel()
	labels := speedBandLabels([]float64{2, 4}, "")
	assert.Equal(t, []string{"0–2", "2–4", "≥ 4"}, labels)
}
