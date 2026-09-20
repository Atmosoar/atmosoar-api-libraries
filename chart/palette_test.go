package chart

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCategoricalPaletteIsTheValidatedSet pins the hexes and their order.
//
// The order is the colourblind-safety mechanism, not a preference: this
// sequence is what clears the adjacent-pair CVD and normal-vision gates in both
// modes. A reorder or a restep has to be re-validated as a whole set, so it has
// to break a test first.
func TestCategoricalPaletteIsTheValidatedSet(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{
		"#2a78d6", "#eb6834", "#1baf7a", "#eda100",
		"#e87ba4", "#008300", "#4a3aa7", "#e34948",
	}, PaletteFor(ModeLight).Series)

	assert.Equal(t, []string{
		"#3987e5", "#d95926", "#199e70", "#c98500",
		"#d55181", "#008300", "#9085e9", "#e66767",
	}, PaletteFor(ModeDark).Series)
}

func TestStatusScaleIsReservedAndModeInvariant(t *testing.T) {
	t.Parallel()
	light, dark := PaletteFor(ModeLight), PaletteFor(ModeDark)

	for status, hex := range map[Status]string{
		StatusGood:     "#0ca30c",
		StatusWarning:  "#fab219",
		StatusSerious:  "#ec835a",
		StatusCritical: "#d03b3b",
		// Unclassified is the muted ink: "no threshold table for this
		// parameter" must not look like a verdict.
		StatusNone: "#898781",
	} {
		assert.Equal(t, hex, light.StatusColor(status), string(status))
		assert.Equal(t, hex, dark.StatusColor(status), string(status)+" dark")
	}

	// A status step must never be reachable as a series colour, or "bad" and
	// "series 4" would wear the same hue.
	for _, mode := range []Mode{ModeLight, ModeDark} {
		p := PaletteFor(mode)
		for _, series := range p.Series {
			for status, hex := range p.Status {
				if status == StatusNone {
					continue
				}
				assert.NotEqual(t, hex, series, "%s collides with status %s", series, status)
			}
		}
	}
}

func TestModesAreSelectedNotInverted(t *testing.T) {
	t.Parallel()
	light, dark := PaletteFor(ModeLight), PaletteFor(ModeDark)

	assert.Equal(t, "#fcfcfb", light.Surface)
	assert.Equal(t, "#1a1a19", dark.Surface)
	assert.Equal(t, "#0b0b0b", light.InkPrimary)
	assert.Equal(t, "#ffffff", dark.InkPrimary)
	assert.Equal(t, ModeLight, PaletteFor("").Mode, "an unset mode resolves light")

	// The dark steps are their own steps from the same ramps: same hue count,
	// different values, except the green that is deliberately mode-invariant.
	require.Len(t, dark.Series, len(light.Series))
	differing := 0
	for i := range light.Series {
		if light.Series[i] != dark.Series[i] {
			differing++
		}
	}
	assert.Equal(t, len(light.Series)-1, differing)
}

func TestSeriesColorFallsBackRatherThanInventing(t *testing.T) {
	t.Parallel()
	p := PaletteFor(ModeLight)
	assert.Equal(t, "#2a78d6", p.SeriesColor(1))
	assert.Equal(t, "#e34948", p.SeriesColor(8))
	// Out of range never generates a hue — it reuses slot 1, and Validate
	// stops a spec from getting here in the first place.
	assert.Equal(t, p.SeriesColor(1), p.SeriesColor(0))
	assert.Equal(t, p.SeriesColor(1), p.SeriesColor(99))
	assert.Equal(t, p.Status[StatusNone], p.StatusColor("nonsense"))
}

func TestSequentialRampRunsTowardTheSurface(t *testing.T) {
	t.Parallel()
	light := PaletteFor(ModeLight)
	// Light mode: near-zero is the palest step, so it recedes into the surface.
	assert.Equal(t, "#cde2fb", light.SequentialColor(0))
	assert.Equal(t, "#0d366b", light.SequentialColor(1))
	assert.Equal(t, light.SequentialColor(0), light.SequentialColor(-2), "clamped")
	assert.Equal(t, light.SequentialColor(1), light.SequentialColor(2), "clamped")

	// Dark mode flips the anchor: near-zero is the darkest step, for the same
	// reason.
	dark := PaletteFor(ModeDark)
	assert.Equal(t, "#0d366b", dark.SequentialColor(0))
	assert.Equal(t, "#cde2fb", dark.SequentialColor(1))
}

func TestResolvedPaletteIsACopy(t *testing.T) {
	t.Parallel()
	first := PaletteFor(ModeLight)
	first.Status[StatusGood] = "#000000"
	second := PaletteFor(ModeLight)
	assert.Equal(t, "#0ca30c", second.StatusColor(StatusGood),
		"mutating one resolved palette must not poison the next")
}
