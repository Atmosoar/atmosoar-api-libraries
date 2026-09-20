package chart

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvedAssignsSlotsInOrderAndNeverCycles(t *testing.T) {
	t.Parallel()
	spec := TimeSeries("t",
		SeriesOf("a", "temperature_2m", pointsAt(Float(1))),
		SeriesOf("b", "dewpoint", pointsAt(Float(2))),
		SeriesOf("c", "dewpoint", pointsAt(Float(3))),
	)
	resolved, err := spec.Resolved()
	require.NoError(t, err)

	series := resolved.Panels[0].Series
	assert.Equal(t, []int{1, 2, 3}, []int{series[0].Slot, series[1].Slot, series[2].Slot})
	assert.Equal(t, "#2a78d6", series[0].Color)
	assert.Equal(t, "#eb6834", series[1].Color)
	assert.Equal(t, "#1baf7a", series[2].Color)
}

func TestResolvedHonoursAPinnedSlot(t *testing.T) {
	t.Parallel()
	// A pinned slot keeps a colour bound to an entity, so a filter that drops
	// one series never repaints the survivors.
	spec := TimeSeries("t",
		Series{Name: "a", Parameter: "temperature_2m", Slot: 5, Points: pointsAt(Float(1))},
		SeriesOf("b", "dewpoint", pointsAt(Float(2))),
	)
	resolved, err := spec.Resolved()
	require.NoError(t, err)
	assert.Equal(t, "#e87ba4", resolved.Panels[0].Series[0].Color)
	assert.Equal(t, 1, resolved.Panels[0].Series[1].Slot)
}

func TestResolvedDoesNotMutateTheInput(t *testing.T) {
	t.Parallel()
	spec := TimeSeries("t", SeriesOf("a", "temperature_2m", pointsAt(Float(1), Float(2))))
	spec.Panels[0].ShowThresholds = true

	_, err := spec.Resolved()
	require.NoError(t, err)

	assert.Empty(t, spec.Panels[0].Series[0].Color, "the caller's spec is left alone")
	assert.Empty(t, spec.Panels[0].Bands)
	assert.Nil(t, spec.Panels[0].Y.Ticks)
	assert.Nil(t, spec.Palette)
	assert.Zero(t, spec.Width)
}

func TestResolvedFillsDefaults(t *testing.T) {
	t.Parallel()
	spec := TimeSeries("t", SeriesOf("rain", "total_precipitation", pointsAt(Float(1))))
	resolved, err := spec.Resolved()
	require.NoError(t, err)

	assert.Equal(t, ModeLight, resolved.Mode)
	assert.Equal(t, DefaultWidth, resolved.Width)
	assert.Equal(t, DefaultHeight, resolved.Height)
	require.NotNil(t, resolved.Palette)

	ser := resolved.Panels[0].Series[0]
	assert.Equal(t, "mm", ser.Unit, "the unit comes from the registry")
	assert.Equal(t, MarkBar, ser.Mark, "an accumulation is bars")
	assert.Equal(t, "Precipitation", resolved.Panels[0].Y.Label)
	assert.True(t, resolved.Panels[0].Y.Zero, "an accumulation includes zero")
	require.NotNil(t, resolved.Panels[0].Y.Min)
	assert.LessOrEqual(t, *resolved.Panels[0].Y.Min, 0.0)
}

func TestResolvedMaterialisesThresholdBandsOnlyWhenAsked(t *testing.T) {
	t.Parallel()
	base := TimeSeries("t", SeriesOf("wind", "wind_speed", pointsAt(Float(4), Float(6))))

	off, err := base.Resolved()
	require.NoError(t, err)
	assert.Empty(t, off.Panels[0].Bands, "shading is opt-in")

	on := base
	on.Panels = []Panel{{Series: base.Panels[0].Series, ShowThresholds: true}}
	resolved, err := on.Resolved()
	require.NoError(t, err)
	require.Len(t, resolved.Panels[0].Bands, 2)
	assert.Equal(t, "#fab219", resolved.Panels[0].Bands[0].Color, "bands wear status, not series, colours")
	assert.Equal(t, "#d03b3b", resolved.Panels[0].Bands[1].Color)

	// Bands clip to the data's domain rather than stretching it: a 4..6 m/s
	// trace is not rescaled to 20 m/s so an unreached band can be shown.
	require.NotNil(t, resolved.Panels[0].Y.Max)
	assert.Less(t, *resolved.Panels[0].Y.Max, 20.0)
}

func TestThresholdBandsClipToTheDomain(t *testing.T) {
	t.Parallel()
	// Temperature bands start at 30 °C. A trace that peaks at 17 gets no
	// shading at all, and keeps its own scale.
	cool := TimeSeries("t", SeriesOf("temp", "temperature_2m", pointsAt(Float(2), Float(17))))
	cool.Panels[0].ShowThresholds = true

	resolved, err := cool.Resolved()
	require.NoError(t, err)
	require.NotNil(t, resolved.Panels[0].Y.Max)
	assert.Less(t, *resolved.Panels[0].Y.Max, 30.0, "the trace keeps its own scale")

	d, err := Render(cool)
	require.NoError(t, err)
	for _, op := range d.Ops {
		if op.Kind == OpRect && op.Alpha == thresholdAlpha {
			t.Fatal("a band the data does not reach must not be drawn")
		}
	}

	// A trace that does reach the bands gets them shaded.
	hot := TimeSeries("t", SeriesOf("temp", "temperature_2m", pointsAt(Float(28), Float(37))))
	hot.Panels[0].ShowThresholds = true
	d, err = Render(hot)
	require.NoError(t, err)

	shaded := 0
	for _, op := range d.Ops {
		if op.Kind == OpRect && op.Alpha == thresholdAlpha {
			shaded++
		}
	}
	assert.Equal(t, 2, shaded, "caution and unfavourable both fall inside this domain")
}

func TestResolvedSharesOneTimeDomainAcrossPanels(t *testing.T) {
	t.Parallel()
	early := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	late := early.Add(12 * time.Hour)

	spec := Meteogram("m",
		PanelOf(Series{Name: "a", Parameter: "temperature_2m", Points: []Point{
			{Time: early, Value: Float(1)}, {Time: early.Add(time.Hour), Value: Float(2)},
		}}),
		PanelOf(Series{Name: "b", Parameter: "wind_speed", Points: []Point{
			{Time: early.Add(6 * time.Hour), Value: Float(3)}, {Time: late, Value: Float(4)},
		}}),
	)
	resolved, err := spec.Resolved()
	require.NoError(t, err)

	require.NotNil(t, resolved.X.Min)
	require.NotNil(t, resolved.X.Max)
	assert.Equal(t, early.UnixMilli(), int64(*resolved.X.Min))
	assert.Equal(t, late.UnixMilli(), int64(*resolved.X.Max))
	assert.NotEmpty(t, resolved.X.Ticks)
}

func TestSpecRoundTripsThroughJSON(t *testing.T) {
	t.Parallel()
	spec := TimeSeries("Wind speed", SeriesOf("wind", "wind_speed", pointsAt(Float(4), nil, Float(6))))
	spec.Panels[0].ShowThresholds = true
	resolved, err := spec.Resolved()
	require.NoError(t, err)

	encoded, err := json.Marshal(resolved)
	require.NoError(t, err)

	var decoded Spec
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	reencoded, err := json.Marshal(decoded)
	require.NoError(t, err)
	assert.JSONEq(t, string(encoded), string(reencoded))

	// The marshalled form carries the resolved palette, so a browser drawing
	// the same spec needs no hardcoded hexes.
	require.NotNil(t, decoded.Palette)
	assert.Equal(t, "#fcfcfb", decoded.Palette.Surface)
	assert.Len(t, decoded.Palette.Series, maxSeries)
	assert.Equal(t, "#fab219", decoded.Palette.Status[StatusWarning])
	assert.Equal(t, "#2a78d6", decoded.Panels[0].Series[0].Color)

	// A gap survives the round trip as a gap, not as a zero.
	assert.Nil(t, decoded.Panels[0].Series[0].Points[1].Value)
}

func TestRunsSplitOnMissingValues(t *testing.T) {
	t.Parallel()
	pts := pointsAt(Float(1), Float(2), nil, nil, Float(5), nil, Float(7))
	assert.Equal(t, [][]int{{0, 1}, {4}, {6}}, runs(pts))

	assert.Empty(t, runs(pointsAt(nil, nil)))
	assert.Equal(t, [][]int{{0, 1, 2}}, runs(pointsAt(Float(1), Float(2), Float(3))))
}

// TestGapsAreNeverBridged is the missing-data guarantee, checked at the draw
// level: two reported stretches either side of a gap produce two polylines, not
// one line across the hole.
func TestGapsAreNeverBridged(t *testing.T) {
	t.Parallel()
	spec := TimeSeries("t", SeriesOf("a", "temperature_2m",
		pointsAt(Float(1), Float(2), nil, nil, Float(5), Float(6))))

	d, err := Render(spec)
	require.NoError(t, err)

	polylines := 0
	for _, op := range d.Ops {
		if op.Kind == OpPolyline && op.StrokeWidth == lineWidth {
			polylines++
		}
	}
	assert.Equal(t, 2, polylines, "one polyline per reported stretch")
}

func TestLoneReadingBecomesADot(t *testing.T) {
	t.Parallel()
	// A single reading between two gaps would vanish as a zero-length line.
	spec := TimeSeries("t", SeriesOf("a", "temperature_2m", pointsAt(nil, Float(2), nil)))
	d, err := Render(spec)
	require.NoError(t, err)

	dots := 0
	for _, op := range d.Ops {
		if op.Kind == OpCircle && op.Fill == "#2a78d6" {
			dots++
		}
	}
	assert.Positive(t, dots)
}

func TestBarsKeepASurfaceGapAndRoundOneEnd(t *testing.T) {
	t.Parallel()
	spec := TimeSeries("t", SeriesOf("rain", "precipitation",
		pointsAt(Float(1), Float(2), Float(3), Float(4))))
	d, err := Render(spec)
	require.NoError(t, err)

	bars := make([]Op, 0, 4)
	for _, op := range d.Ops {
		if op.Kind == OpRect && op.Radius > 0 {
			bars = append(bars, op)
		}
	}
	require.Len(t, bars, 4)
	for _, bar := range bars {
		assert.LessOrEqual(t, bar.W, barCap, "a sparse series must not draw slabs")
		assert.Equal(t, barRadius, bar.Radius, "rounded at the data end, square at the baseline")
		assert.Empty(t, bar.Stroke, "white does the separating, never a stroke")
	}
	assert.GreaterOrEqual(t, bars[1].X-(bars[0].X+bars[0].W), 2.0, "bars never touch")

	// Densely sampled, the slot narrows until the 2px surface gap is all that
	// is left between neighbours — and it is still there.
	dense := make([]*float64, 0, 64)
	for i := 0; i < 64; i++ {
		dense = append(dense, Float(float64(i%5)))
	}
	spec = TimeSeries("t", SeriesOf("rain", "precipitation", pointsAt(dense...)))
	d, err = Render(spec)
	require.NoError(t, err)

	bars = bars[:0]
	for _, op := range d.Ops {
		if op.Kind == OpRect && op.Radius > 0 {
			bars = append(bars, op)
		}
	}
	require.Greater(t, len(bars), 2)
	// The first bar is trimmed to the plot edge, so measure a pair from the
	// middle of the run.
	assert.InDelta(t, 2.0, bars[2].X-(bars[1].X+bars[1].W), 0.01)
}

func TestMarkersCarryASurfaceRing(t *testing.T) {
	t.Parallel()
	spec := TimeSeries("t", SeriesOf("a", "temperature_2m", pointsAt(Float(1), Float(2))))
	d, err := Render(spec)
	require.NoError(t, err)

	found := false
	for _, op := range d.Ops {
		if op.Kind == OpCircle && op.Ring > 0 {
			assert.Equal(t, d.Palette.Surface, op.RingColor)
			assert.GreaterOrEqual(t, op.Radius, 4.0, "a marker is at least 8px across")
			found = true
		}
	}
	assert.True(t, found)
}

func TestTextNeverWearsASeriesColour(t *testing.T) {
	t.Parallel()
	for _, spec := range specFixtures() {
		d, err := Render(spec)
		require.NoError(t, err)

		inks := map[string]bool{
			d.Palette.InkPrimary: true, d.Palette.InkSecondary: true, d.Palette.InkMuted: true,
		}
		for _, op := range d.Ops {
			if op.Kind != OpText {
				continue
			}
			assert.True(t, inks[op.Fill],
				"%q is drawn in %s, which is not an ink token", op.Text, op.Fill)
		}
	}
}

func TestLegendAppearsForTwoSeriesAndNotForOne(t *testing.T) {
	t.Parallel()
	single := TimeSeries("t", SeriesOf("only", "temperature_2m", pointsAt(Float(1))))
	assert.Equal(t, 1, legendEntryCount(&single))

	two := TimeSeries("t",
		SeriesOf("a", "temperature_2m", pointsAt(Float(1))),
		SeriesOf("b", "dewpoint", pointsAt(Float(2))),
	)
	assert.Equal(t, 2, legendEntryCount(&two))

	// A meteogram of titled single-series panels names itself; a legend would
	// only restate the panel titles.
	meteo := Meteogram("m",
		PanelOf(SeriesOf("a", "temperature_2m", pointsAt(Float(1)))),
		PanelOf(SeriesOf("b", "wind_speed", pointsAt(Float(2)))),
	)
	assert.Equal(t, 0, legendEntryCount(&meteo))
}

func TestPanelHeaderStatesTheUnitExactlyOnce(t *testing.T) {
	t.Parallel()
	// One series: the panel is titled and carries the unit.
	single := Panel{Title: "Temperature", Y: Axis{Unit: "°C"}}
	assert.Equal(t, "Temperature (°C)", panelTitle(&single))

	// Several series: the legend names them, so the header is the unit alone —
	// never a title claiming the panel plots only its first series.
	shared := Panel{Y: Axis{Unit: "°C"}}
	assert.Equal(t, "°C", panelTitle(&shared))

	assert.Empty(t, panelTitle(&Panel{}))
	assert.Equal(t, "Visibility", panelTitle(&Panel{Title: "Visibility"}))
}

func TestFromSamplesSortsAndGapsMissingParameters(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	samples := []Sample{
		{Time: start.Add(2 * time.Hour), Values: map[string]*float64{"temperature_2m": Float(12)}},
		{Time: start, Values: map[string]*float64{"temperature_2m": Float(10), "wind_speed": Float(3)}},
		{Time: start.Add(time.Hour), Values: map[string]*float64{"wind_speed": Float(4)}},
	}

	spec := FromSamples(KindMeteogram, "m", samples, []string{"temperature_2m", "wind_speed"})
	require.NoError(t, spec.Validate(), "rows arrive in any order and are sorted here")
	require.Len(t, spec.Panels, 2)

	temp := spec.Panels[0].Series[0]
	require.Len(t, temp.Points, 3)
	assert.Equal(t, start, temp.Points[0].Time)
	assert.Nil(t, temp.Points[1].Value, "a parameter missing from a row is a gap")
	require.NotNil(t, temp.Points[2].Value)
	assert.InDelta(t, 12.0, *temp.Points[2].Value, 1e-9)
}

func TestFromSamplesTimeSeriesSharesOnePanel(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	samples := []Sample{{Time: start, Values: map[string]*float64{
		"temperature_2m": Float(10), "dewpoint": Float(6),
	}}}

	spec := FromSamples(KindTimeSeries, "t", samples, []string{"temperature_2m", "dewpoint"})
	require.Len(t, spec.Panels, 1)
	assert.Len(t, spec.Panels[0].Series, 2)
	require.NoError(t, spec.Validate(), "both are °C, so one scale is honest")
}

func TestBuildStaysInsideTheCanvas(t *testing.T) {
	t.Parallel()
	for name, spec := range specFixtures() {
		d, err := Render(spec)
		require.NoError(t, err, name)

		w, h := float64(d.Width), float64(d.Height)
		for _, op := range d.Ops {
			for _, p := range op.Points {
				assert.GreaterOrEqual(t, p.X, -1.0, "%s: %v off the left edge", name, p)
				assert.LessOrEqual(t, p.X, w+1, "%s: %v off the right edge", name, p)
				assert.GreaterOrEqual(t, p.Y, -1.0, "%s: %v above the top edge", name, p)
				assert.LessOrEqual(t, p.Y, h+1, "%s: %v below the bottom edge", name, p)
			}
			if op.Kind == OpRect {
				assert.GreaterOrEqual(t, op.X, -1.0, "%s: rect overflows left", name)
				assert.GreaterOrEqual(t, op.Y, -1.0, "%s: rect overflows top", name)
				assert.LessOrEqual(t, op.X+op.W, w+1, "%s: rect overflows right", name)
				assert.LessOrEqual(t, op.Y+op.H, h+1, "%s: rect overflows bottom", name)
			}
		}
	}
}

func TestBuildHandlesAnAbsurdlySmallCanvas(t *testing.T) {
	t.Parallel()
	// A caller asking for the minimum size must still get a chart, not a
	// negative-height panel.
	spec := Meteogram("m",
		PanelOf(SeriesOf("a", "temperature_2m", pointsAt(Float(1), Float(2)))),
		PanelOf(SeriesOf("b", "wind_speed", pointsAt(Float(3), Float(4)))),
	)
	spec.Width, spec.Height = minWidth, minHeight

	d, err := Render(spec)
	require.NoError(t, err)
	assert.NotEmpty(t, d.Ops)

	out, err := PNG(spec)
	require.NoError(t, err)
	assert.NotEmpty(t, out)
}

func TestBandMarkFillsBetweenLowAndHigh(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	points := make([]Point, 0, 6)
	for i := range 6 {
		points = append(points, Point{
			Time:  start.Add(time.Duration(i) * time.Hour),
			Value: Float(10 + float64(i)),
			Low:   Float(8 + float64(i)),
			High:  Float(13 + float64(i)),
		})
	}
	spec := TimeSeries("spread", Series{
		Name: "Ensemble", Parameter: "temperature_2m", Mark: MarkBand, Points: points,
	})

	d, err := Render(spec)
	require.NoError(t, err)

	var fills []Op
	for _, op := range d.Ops {
		if op.Kind == OpPolygon && op.Alpha > 0 && op.Alpha < 1 {
			fills = append(fills, op)
		}
	}
	require.Len(t, fills, 1, "one wash per band series")
	assert.InDelta(t, bandAlpha, fills[0].Alpha, 1e-9, "a wash, never a saturated block")
	assert.Len(t, fills[0].Points, 2*len(points), "the polygon walks the highs out and the lows back")
	assert.Empty(t, fills[0].Stroke, "no border around a fill")

	// The mid line still rides on top of the wash.
	lines := 0
	for _, op := range d.Ops {
		if op.Kind == OpPolyline && op.StrokeWidth == lineWidth {
			lines++
		}
	}
	assert.Equal(t, 1, lines)

	// A band sample missing one of its bounds is skipped rather than half-drawn.
	points[2].High = nil
	partial := TimeSeries("spread", Series{
		Name: "Ensemble", Parameter: "temperature_2m", Mark: MarkBand, Points: points,
	})
	d, err = Render(partial)
	require.NoError(t, err)
	for _, op := range d.Ops {
		if op.Kind == OpPolygon && op.Alpha == bandAlpha {
			assert.Len(t, op.Points, 2*(len(points)-1))
		}
	}
}

func TestStepMarkHoldsEachValue(t *testing.T) {
	t.Parallel()
	spec := TimeSeries("state", Series{
		Name: "Ceiling", Parameter: "cloud_base", Mark: MarkStep,
		Points: pointsAt(Float(300), Float(900), Float(900), Float(200)),
	})
	d, err := Render(spec)
	require.NoError(t, err)

	var line Op
	for _, op := range d.Ops {
		if op.Kind == OpPolyline && op.StrokeWidth == lineWidth {
			line = op
		}
	}
	require.NotEmpty(t, line.Points)
	// A step inserts a corner before each new value: n samples, 2n-1 vertices.
	assert.Len(t, line.Points, 7)
	assert.InDelta(t, line.Points[0].Y, line.Points[1].Y, 1e-9, "the value is held, then stepped")
}

func TestLongLabelsAreTruncatedNotClipped(t *testing.T) {
	t.Parallel()
	long := "Pressure difference across the Brenner axis, hourly, from the operational run"
	assert.Equal(t, "short", truncateToWidth("short", sizeAxis, false, 400))

	cut := truncateToWidth(long, sizeSubtitle, false, 120)
	require.NotEmpty(t, cut)
	assert.Less(t, len(cut), len(long))
	assert.Contains(t, cut, "…")
	assert.LessOrEqual(t, measureText(cut, sizeSubtitle, false), 120.0)

	// Where not even an ellipsis fits, the label is dropped: text is never
	// cropped by its own mark.
	assert.Empty(t, truncateToWidth(long, sizeTitle, true, 2))

	// End through the real pipeline: a title too long for the canvas still
	// renders, shortened.
	spec := TimeSeries(long, SeriesOf("a", "pressure_difference", pointsAt(Float(1), Float(2))))
	spec.Subtitle = long
	spec.Source = long
	spec.Width = minWidth
	out, err := SVG(spec)
	require.NoError(t, err)
	assert.Contains(t, string(out), "…")
}
