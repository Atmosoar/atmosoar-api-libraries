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
