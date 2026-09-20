package chart

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleSeries builds a deterministic day of hourly readings with two gaps, so
// every render test exercises the missing-data path.
func sampleSeries(name, parameter string, base, amplitude float64) Series {
	start := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	points := make([]Point, 0, 24)
	for h := 0; h < 24; h++ {
		pt := Point{Time: start.Add(time.Duration(h) * time.Hour)}
		if h != 7 && h != 8 {
			pt.Value = Float(base + amplitude*math.Sin(float64(h)/24*2*math.Pi))
		}
		points = append(points, pt)
	}
	return SeriesOf(name, parameter, points)
}

func windSamples() []Sample {
	start := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	rows := make([]Sample, 0, 96)
	for i := 0; i < 96; i++ {
		// A prevailing westerly with a spread, plus a calm spell.
		dir := math.Mod(250+40*math.Sin(float64(i)/7), 360)
		speed := 3 + 6*math.Abs(math.Sin(float64(i)/11))
		if i%17 == 0 {
			speed = 0.2
		}
		rows = append(rows, Sample{
			Time: start.Add(time.Duration(i) * 15 * time.Minute),
			Values: map[string]*float64{
				"wind_speed":     Float(speed),
				"wind_direction": Float(dir),
			},
		})
	}
	return rows
}

// specFixtures is every form, in both modes — the set the renderers are held to.
func specFixtures() map[string]Spec {
	temp := sampleSeries("Temperature", "temperature_2m", 14, 7)
	dew := sampleSeries("Dewpoint", "dewpoint", 9, 4)
	wind := sampleSeries("Wind speed", "wind_speed", 12, 9)
	rain := sampleSeries("Precipitation", "precipitation", 1.2, 1.2)
	generated := time.Date(2026, 9, 19, 23, 0, 0, 0, time.UTC)

	single := TimeSeries("Föhn axis — Brenner", sampleSeries("Δp", "pressure_difference", 4, 6))
	single.Subtitle = "Pressure difference across the axis, hourly"
	single.Source = "Atmosoar · foehn-api"
	single.Generated = &generated

	two := TimeSeries("Temperature and dewpoint — Innsbruck", temp, dew)
	two.Subtitle = "Same unit, so one panel and one scale"

	thresholds := TimeSeries("Wind speed — Brenner", wind)
	thresholds.Panels[0].ShowThresholds = true
	thresholds.Subtitle = "Operational thresholds shaded behind the trace"

	meteo := Meteogram("Meteogram — Innsbruck",
		PanelOf(temp), PanelOf(wind), PanelOf(rain))
	meteo.Source = "Atmosoar MMA · ICON-D2"
	meteo.Generated = &generated

	rose := FromSamples(KindWindrose, "Wind rose — Innsbruck", windSamples(), nil)
	rose.Subtitle = "24 h, 15-minute readings"

	out := map[string]Spec{}
	for name, spec := range map[string]Spec{
		"timeseries-single":     single,
		"timeseries-two":        two,
		"timeseries-thresholds": thresholds,
		"meteogram":             meteo,
		"windrose":              rose,
	} {
		light, dark := spec, spec
		light.Mode = ModeLight
		dark.Mode = ModeDark
		out[name+"-light"] = light
		out[name+"-dark"] = dark
	}
	return out
}

func TestRenderEveryFormAndMode(t *testing.T) {
	t.Parallel()
	// CHART_DUMP_DIR writes the rendered fixtures out, so a change to the
	// visual language can be looked at rather than argued about.
	dump := os.Getenv("CHART_DUMP_DIR")

	for name, spec := range specFixtures() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pngBytes, err := PNG(spec)
			require.NoError(t, err)
			img, err := png.Decode(bytes.NewReader(pngBytes))
			require.NoError(t, err, "png must decode")
			assert.Equal(t, image.Rect(0, 0, DefaultWidth, DefaultHeight), img.Bounds())

			svgBytes, err := SVG(spec)
			require.NoError(t, err)
			assert.Contains(t, string(svgBytes), "<svg")
			assert.Contains(t, string(svgBytes), "</svg>")

			specJSON, err := JSON(spec)
			require.NoError(t, err)
			assert.Contains(t, string(specJSON), `"palette"`)

			if dump == "" {
				return
			}
			require.NoError(t, os.WriteFile(filepath.Join(dump, name+".png"), pngBytes, 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dump, name+".svg"), svgBytes, 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dump, name+".json"), specJSON, 0o600))
		})
	}
}

// TestRenderersShareOneDrawList is the anti-drift check: both encoders consume
// the identical op list, so neither can grow geometry of its own.
func TestRenderersShareOneDrawList(t *testing.T) {
	t.Parallel()
	spec := TimeSeries("Wind speed", sampleSeries("Wind speed", "wind_speed", 12, 6))

	first, err := Render(spec)
	require.NoError(t, err)
	second, err := Render(spec)
	require.NoError(t, err)
	require.Equal(t, first.Ops, second.Ops, "Build must be deterministic")

	svgBytes := renderSVG(first)
	pngBytes, err := renderPNG(second)
	require.NoError(t, err)

	// The op list is untouched by either renderer: rendering twice, in either
	// order, gives the same bytes.
	assert.Equal(t, svgBytes, renderSVG(second))
	again, err := renderPNG(first)
	require.NoError(t, err)
	assert.Equal(t, pngBytes, again)
}

func TestSVGEscapesText(t *testing.T) {
	t.Parallel()
	spec := TimeSeries(`Wind <speed> & "gust"`, sampleSeries("A", "wind_speed", 5, 2))
	out, err := SVG(spec)
	require.NoError(t, err)
	assert.Contains(t, string(out), "&lt;speed&gt; &amp; &quot;gust&quot;")
	assert.NotContains(t, string(out), "<speed>")
}

func TestPNGModesDifferInSurface(t *testing.T) {
	t.Parallel()
	light := TimeSeries("t", sampleSeries("A", "temperature_2m", 10, 2))
	dark := light
	dark.Mode = ModeDark

	lightPNG, err := PNG(light)
	require.NoError(t, err)
	darkPNG, err := PNG(dark)
	require.NoError(t, err)
	assert.NotEqual(t, lightPNG, darkPNG, "dark mode is selected, not derived")

	lightImg, err := png.Decode(bytes.NewReader(lightPNG))
	require.NoError(t, err)
	darkImg, err := png.Decode(bytes.NewReader(darkPNG))
	require.NoError(t, err)

	// The top-left pixel is the chart surface in both modes.
	lr, lg, lb, _ := lightImg.At(1, 1).RGBA()
	dr, dg, db, _ := darkImg.At(1, 1).RGBA()
	assert.Greater(t, lr+lg+lb, dr+dg+db, "the light surface must be lighter than the dark one")
}

func TestRenderRejectsInvalidSpec(t *testing.T) {
	t.Parallel()
	_, err := PNG(Spec{Kind: KindTimeSeries})
	require.ErrorIs(t, err, ErrEmptySpec)

	_, err = SVG(Spec{})
	require.Error(t, err)

	_, err = JSON(Spec{Kind: "sunburst", Panels: []Panel{{Series: []Series{{Name: "a"}}}}})
	require.Error(t, err)
}

func TestParseFormatAndEncode(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input string
		want  Format
	}{
		{"chart_png", FormatPNG}, {"PNG", FormatPNG}, {"chartpng", FormatPNG},
		{"chart_svg", FormatSVG}, {"svg", FormatSVG},
		{"chartspec", FormatSpec}, {"chart_spec", FormatSpec},
		// A query parameter arrives with whatever padding the client sent.
		{"  spec  ", FormatSpec},
	} {
		got, ok := ParseFormat(tc.input)
		assert.True(t, ok, tc.input)
		assert.Equal(t, tc.want, got, tc.input)
	}
	_, ok := ParseFormat("geotiff")
	assert.False(t, ok)

	spec := TimeSeries("t", sampleSeries("A", "temperature_2m", 10, 2))
	for _, tc := range []struct {
		format Format
		mime   string
		prefix string
	}{
		{FormatPNG, MIMEPNG, "\x89PNG"},
		{FormatSVG, MIMESVG, "<svg"},
		{FormatSpec, MIMEJSON, `{"kind"`},
	} {
		data, mime, err := Encode(spec, tc.format)
		require.NoError(t, err)
		assert.Equal(t, tc.mime, mime)
		assert.True(t, bytes.HasPrefix(data, []byte(tc.prefix)), string(tc.format))
	}

	_, _, err := Encode(spec, Format("pdf"))
	require.Error(t, err)
}
