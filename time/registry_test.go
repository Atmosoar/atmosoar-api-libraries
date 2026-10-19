package time

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestRegistryAccessors_Concurrent exercises the BUG-001 locked accessors
// from many goroutines. Run with `go test -race` to catch any unguarded
// access.
func TestRegistryAccessors_Concurrent(t *testing.T) {
	// Reset registries under the lock so we start clean.
	registryMu.Lock()
	TimeShortcutsMap = map[string]func() time.Time{}
	TimeRangeShortcutsMap = map[string]func() RangeTime{}
	registryMu.Unlock()

	const goroutines = 64
	const ops = 200

	now := func() time.Time { return time.Unix(0, 0).UTC() }
	rangeFn := func() RangeTime {
		t0 := time.Unix(0, 0).UTC()
		return RangeTime{Start: t0, End: t0.Add(time.Hour), Resolution: 0}
	}

	var wg sync.WaitGroup
	wg.Add(goroutines * 4)

	for range goroutines {
		go func() {
			defer wg.Done()
			for range ops {
				RegisterTimeShortcut("now", now)
			}
		}()
		go func() {
			defer wg.Done()
			for range ops {
				_, _ = LookupTimeShortcut("now")
			}
		}()
		go func() {
			defer wg.Done()
			for range ops {
				RegisterTimeRangeShortcut("3day", rangeFn)
			}
		}()
		go func() {
			defer wg.Done()
			for range ops {
				_, _ = LookupTimeRangeShortcut("3day")
			}
		}()
	}

	wg.Wait()

	fn, ok := LookupTimeShortcut("now")
	assert.True(t, ok)
	assert.NotNil(t, fn)
	rfn, ok := LookupTimeRangeShortcut("3day")
	assert.True(t, ok)
	assert.NotNil(t, rfn)

	// Cleanup.
	registryMu.Lock()
	TimeShortcutsMap = map[string]func() time.Time{}
	TimeRangeShortcutsMap = map[string]func() RangeTime{}
	registryMu.Unlock()
}

// TestFormatTimeDataToRFC3339_FallbackOnError verifies BUG-002: a non-zero
// input must round-trip to a non-zero value (specifically the RFC3339-truncated
// version of the input), and never the zero time.
func TestFormatTimeDataToRFC3339_FallbackOnError(t *testing.T) {
	in := time.Date(2026, 4, 19, 12, 34, 56, 789000000, time.UTC)
	got := FormatTimeDataToRFC3339(in)

	// Expected: input truncated to RFC3339 precision (whole seconds, UTC).
	want := time.Date(2026, 4, 19, 12, 34, 56, 0, time.UTC)
	assert.Equal(t, want, got)
	assert.False(t, got.IsZero(), "must never silently return the zero time")
}

// roundTripRFC3339 is the original FormatTimeDataToRFC3339 body, kept as the
// reference the UTC fast path must match exactly.
func roundTripRFC3339(t time.Time) time.Time {
	parsed, err := time.Parse(time.RFC3339, t.Format(time.RFC3339))
	if err != nil {
		return t
	}
	return parsed
}

// TestFormatTimeDataToRFC3339_MatchesRoundTrip pins the truncate fast path to
// the old Format/Parse output, value and Location, for UTC and other zones.
func TestFormatTimeDataToRFC3339_MatchesRoundTrip(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	inputs := []time.Time{
		time.Now().UTC(),
		time.Date(2026, 4, 19, 12, 34, 56, 999999999, time.UTC),
		time.Date(1969, 12, 31, 23, 59, 59, 500000000, time.UTC),
		time.Date(10000, 1, 1, 0, 0, 0, 5, time.UTC),
		time.Date(2026, 4, 19, 12, 34, 56, 789, berlin),
		time.Date(2026, 4, 19, 12, 34, 56, 789, time.FixedZone("x", -3*3600)),
	}
	for _, in := range inputs {
		got, want := FormatTimeDataToRFC3339(in), roundTripRFC3339(in)
		assert.True(t, got.Equal(want), "%v: got %v, want %v", in, got, want)
		assert.Equal(t, want.Location().String(), got.Location().String(), "%v: location", in)
	}
}
