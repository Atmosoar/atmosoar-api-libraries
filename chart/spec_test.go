package chart

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pointsAt(values ...*float64) []Point {
	start := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	out := make([]Point, 0, len(values))
	for i, v := range values {
		out = append(out, Point{Time: start.Add(time.Duration(i) * time.Hour), Value: v})
	}
	return out
}

func TestValidateAcceptsEveryForm(t *testing.T) {
	t.Parallel()
	specs := []Spec{
		TimeSeries("t", SeriesOf("a", "temperature_2m", pointsAt(Float(1), Float(2)))),
		Meteogram("m",
			PanelOf(SeriesOf("a", "temperature_2m", pointsAt(Float(1)))),
			PanelOf(SeriesOf("b", "wind_speed", pointsAt(Float(3)))),
		),
		Windrose("w", Series{
			Name: "wind", Parameter: "wind_speed",
			Points: []Point{{Time: time.Now(), Value: Float(5), Direction: Float(270)}},
		}, nil),
	}
	for i := range specs {
		require.NoError(t, specs[i].Validate(), specs[i].Kind)
	}
}

func TestValidateRejectsMixedUnitsInOnePanel(t *testing.T) {
	t.Parallel()
	// Two units in one plot would need two y-scales. The package would rather
	// fail than draw the single worst chart mistake there is.
	spec := TimeSeries("t",
		SeriesOf("temp", "temperature_2m", pointsAt(Float(12))),
		SeriesOf("wind", "wind_speed", pointsAt(Float(4))),
	)
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "one panel per unit")
}

func TestValidateRejectsMoreSeriesThanSlots(t *testing.T) {
	t.Parallel()
	series := make([]Series, 0, maxSeries+1)
	for i := 0; i <= maxSeries; i++ {
		series = append(series, SeriesOf(string(rune('a'+i)), "temperature_2m", pointsAt(Float(1))))
	}
	spec := TimeSeries("t", series...)
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "categorical slots")
}

// panelsOf wraps series into the one-panel shape most specs have, so the case
// table below reads as the mistake it is testing rather than as brace soup.
func panelsOf(series ...Series) []Panel {
	return []Panel{{Series: series}}
}

func TestValidateRejectsStructuralMistakes(t *testing.T) {
	t.Parallel()
	valid := pointsAt(Float(1), Float(2))
	start := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	named := Series{Name: "a", Points: valid}

	cases := map[string]struct {
		spec Spec
		want string
	}{
		"no kind":   {Spec{Panels: panelsOf(named)}, "kind is required"},
		"bad kind":  {Spec{Kind: "sunburst", Panels: panelsOf(named)}, "unknown kind"},
		"bad mode":  {Spec{Kind: KindTimeSeries, Mode: "sepia", Panels: panelsOf(named)}, "unknown mode"},
		"no panels": {Spec{Kind: KindTimeSeries}, "no panels"},
		"no series": {Spec{Kind: KindTimeSeries, Panels: []Panel{{}}}, "no series"},
		"no name": {
			Spec{Kind: KindTimeSeries, Panels: panelsOf(Series{Points: valid})},
			"no name",
		},
		"bad mark": {
			Spec{Kind: KindTimeSeries, Panels: panelsOf(Series{Name: "a", Mark: "spline"})},
			"unknown mark",
		},
		"bad slot": {
			Spec{Kind: KindTimeSeries, Panels: panelsOf(Series{Name: "a", Slot: 99})},
			"outside 1..8",
		},
		"narrow": {
			Spec{Kind: KindTimeSeries, Width: 10, Panels: panelsOf(named)},
			"width 10 outside",
		},
		"short": {
			Spec{Kind: KindTimeSeries, Height: 10, Panels: panelsOf(named)},
			"height 10 outside",
		},
		"two panels": {
			Spec{Kind: KindTimeSeries, Panels: []Panel{
				{Series: []Series{named}},
				{Series: []Series{{Name: "b", Points: valid}}},
			}},
			"takes one panel",
		},
		"unsorted": {
			Spec{Kind: KindTimeSeries, Panels: panelsOf(Series{Name: "a", Points: []Point{
				{Time: start.Add(time.Hour), Value: Float(1)},
				{Time: start, Value: Float(2)},
			}})},
			"not sorted",
		},
		"no time": {
			Spec{Kind: KindTimeSeries, Panels: panelsOf(Series{Name: "a", Points: []Point{
				{Value: Float(1)},
			}})},
			"no time",
		},
		"not finite": {
			Spec{Kind: KindTimeSeries, Panels: panelsOf(Series{Name: "a", Points: []Point{
				{Time: start, Value: Float(math.Inf(1))},
			}})},
			"not finite",
		},
		"nan": {
			Spec{Kind: KindTimeSeries, Panels: panelsOf(Series{Name: "a", Points: []Point{
				{Time: start, Value: Float(math.NaN())},
			}})},
			"not finite",
		},
		"inverted band": {
			Spec{Kind: KindTimeSeries, Panels: panelsOf(Series{
				Name: "a", Mark: MarkBand,
				Points: []Point{{Time: start, Low: Float(5), High: Float(1)}},
			})},
			"lo > hi",
		},
		"unlabelled status band": {
			Spec{Kind: KindTimeSeries, Panels: []Panel{{
				Series: []Series{named},
				Bands:  []Band{{Status: StatusWarning}},
			}}},
			"no label",
		},
		"unknown band status": {
			Spec{Kind: KindTimeSeries, Panels: []Panel{{
				Series: []Series{named},
				Bands:  []Band{{Status: "spicy", Label: "hot"}},
			}}},
			"unknown status",
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
}

func TestEffectiveUnitAndMarkComeFromTheRegistry(t *testing.T) {
	t.Parallel()
	ser := Series{Name: "rain", Parameter: "total_precipitation"}
	assert.Equal(t, "mm", ser.EffectiveUnit())
	assert.Equal(t, MarkBar, ser.EffectiveMark(), "an accumulation is bars")

	explicit := Series{Name: "rain", Parameter: "precipitation", Unit: "in", Mark: MarkLine}
	assert.Equal(t, "in", explicit.EffectiveUnit(), "an explicit unit wins")
	assert.Equal(t, MarkLine, explicit.EffectiveMark())

	unknown := Series{Name: "x", Parameter: "teacups"}
	assert.Empty(t, unknown.EffectiveUnit())
	assert.Equal(t, MarkLine, unknown.EffectiveMark(), "a line is the default")
}
