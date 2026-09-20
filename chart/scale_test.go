package chart

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNiceStepSnapsTo125(t *testing.T) {
	t.Parallel()
	for raw, want := range map[float64]float64{
		0.07: 0.1, 0.3: 0.5, 0.9: 1, 1: 1, 1.5: 2, 3: 5, 6: 10,
		17: 20, 23: 50, 60: 100, 4200: 5000,
	} {
		assert.InDelta(t, want, niceStep(raw), 1e-9, "raw %v", raw)
	}
	// A degenerate interval must not produce a zero step and hang tick
	// generation.
	for _, bad := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		assert.InDelta(t, 1.0, niceStep(bad), 1e-9)
	}
}

func TestNiceDomainAlwaysContainsTheData(t *testing.T) {
	t.Parallel()
	cases := []struct{ lo, hi float64 }{
		{4.2, 18.7}, {-12.3, 5.5}, {0, 0.004}, {981.2, 1032.8}, {-0.5, -0.1},
		{1e-6, 2e-6}, {0, 1e6},
	}
	for _, tc := range cases {
		lo, hi, step := niceDomain(tc.lo, tc.hi, false)
		assert.LessOrEqual(t, lo, tc.lo, "domain must contain the low value")
		assert.GreaterOrEqual(t, hi, tc.hi, "domain must contain the high value")
		assert.Positive(t, step)
	}
}

func TestNiceDomainIncludesZeroWhenAsked(t *testing.T) {
	t.Parallel()
	lo, hi, _ := niceDomain(4, 18, true)
	assert.LessOrEqual(t, lo, 0.0)
	assert.GreaterOrEqual(t, hi, 18.0)

	// Without the flag a temperature trace keeps its own window.
	lo, _, _ = niceDomain(12, 18, false)
	assert.Greater(t, lo, 0.0)
}

func TestNiceDomainGivesAFlatSeriesAWindow(t *testing.T) {
	t.Parallel()
	lo, hi, step := niceDomain(7, 7, false)
	assert.Less(t, lo, 7.0)
	assert.Greater(t, hi, 7.0)
	assert.Positive(t, step)
}

func TestNumericTicksReadAsCleanNumbers(t *testing.T) {
	t.Parallel()
	lo, hi, step := niceDomain(0, 23, false)
	ticks := numericTicks(lo, hi, step, 1)
	require.NotEmpty(t, ticks)

	assert.Equal(t, "0", ticks[0].Label, "a zero tick never reads as -0")
	for _, tick := range ticks {
		assert.GreaterOrEqual(t, tick.Value, lo)
		assert.LessOrEqual(t, tick.Value, hi+step/1e6)
		assert.NotContains(t, tick.Label, ".", "a step of 5 needs no decimals")
	}
	assert.Nil(t, numericTicks(0, 10, 0, 1), "a zero step yields no ticks")
}

func TestTickLabelsGroupThousands(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]string{
		"0": "0", "999": "999", "1000": "1000", "9999": "9999",
		"10000": "10,000", "1234567": "1,234,567", "-45000": "-45,000",
		"12345.5": "12,345.5",
	} {
		assert.Equal(t, want, groupThousands(input), input)
	}
}

func TestTimeTicksSitOnWholeUnits(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)

	for _, span := range []time.Duration{
		30 * time.Minute, 6 * time.Hour, 24 * time.Hour,
		7 * 24 * time.Hour, 90 * 24 * time.Hour,
	} {
		ticks := timeTicks(t0, t0.Add(span))
		require.NotEmpty(t, ticks, span.String())
		assert.LessOrEqual(t, len(ticks), 12, "span %s produced %d ticks", span, len(ticks))
		for _, tick := range ticks {
			require.NotNil(t, tick.Time, "a time tick carries its instant for relabelling")
			assert.False(t, tick.Time.Before(t0))
			assert.False(t, tick.Time.After(t0.Add(span)))
		}
	}

	// A zero-width domain still labels its single instant rather than looping.
	ticks := timeTicks(t0, t0)
	require.Len(t, ticks, 1)
}

func TestTimeTickLabelsMatchTheSpan(t *testing.T) {
	t.Parallel()
	noon := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	midnight := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)

	assert.Equal(t, "12:00", formatTickTime(noon, 6*time.Hour))
	// Midnight carries the date: a run of bare "00:00" labels is how a
	// multi-day axis becomes unreadable.
	assert.Equal(t, "19 Sep", formatTickTime(midnight, 24*time.Hour))
	assert.Equal(t, "Sat 19 Sep", formatTickTime(noon, 5*24*time.Hour))
	assert.Equal(t, "Sep 2026", formatTickTime(noon, 200*24*time.Hour))
}

func TestLinearScaleHandlesInvertedAndFlatRanges(t *testing.T) {
	t.Parallel()
	// A y-axis maps a growing domain onto shrinking pixels.
	s := linearScale{d0: 0, d1: 10, r0: 100, r1: 0}
	assert.InDelta(t, 100.0, s.at(0), 1e-9)
	assert.InDelta(t, 0.0, s.at(10), 1e-9)
	assert.InDelta(t, 50.0, s.at(5), 1e-9)

	flat := linearScale{d0: 5, d1: 5, r0: 0, r1: 100}
	assert.InDelta(t, 50.0, flat.at(5), 1e-9, "a flat domain centres rather than dividing by zero")
}

func TestTimeScaleHandlesAFlatDomain(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	s := timeScale{t0: t0, t1: t0.Add(2 * time.Hour), r0: 0, r1: 200}
	assert.InDelta(t, 100.0, s.at(t0.Add(time.Hour)), 1e-9)

	flat := timeScale{t0: t0, t1: t0, r0: 0, r1: 200}
	assert.InDelta(t, 100.0, flat.at(t0), 1e-9)
}

func TestSeriesExtentIgnoresGapsAndCoversBandBounds(t *testing.T) {
	t.Parallel()
	p := Panel{Series: []Series{{
		Name: "spread", Mark: MarkBand,
		Points: []Point{
			{Time: time.Now(), Value: Float(5), Low: Float(2), High: Float(9)},
			{Time: time.Now().Add(time.Hour)},
		},
	}}}
	lo, hi, ok := seriesExtent(&p)
	require.True(t, ok)
	assert.InDelta(t, 2.0, lo, 1e-9)
	assert.InDelta(t, 9.0, hi, 1e-9)

	empty := Panel{Series: []Series{{Name: "none", Points: []Point{{Time: time.Now()}}}}}
	_, _, ok = seriesExtent(&empty)
	assert.False(t, ok, "a series of nothing but gaps has no extent")
}
