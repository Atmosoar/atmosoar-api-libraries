package time

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTimeParsing tests the ParseTime function
func TestTimeParsing(t *testing.T) {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	yesterday := today.AddDate(0, 0, -1)
	tomorrow := today.AddDate(0, 0, 1)

	validTime := "2023-06-15T12:00:00Z"
	parsedTime, _ := time.Parse(time.RFC3339, validTime)

	h0 := 0
	hm24 := -24
	hp24 := 24

	TimeShortcutsMap = BuildTimeShortcuts(map[string]TimeShortcut{
		"now":       {Type: "now"},
		"today":     {Type: "offset", Hours: &h0},
		"yesterday": {Type: "offset", Hours: &hm24},
		"tomorrow":  {Type: "offset", Hours: &hp24},
		"release":   {Type: "fixed", Value: "2025-08-01T00:00:00Z"},
	})

	tests := []struct {
		name        string
		input       string
		expected    *TimeRange
		expectedErr string
	}{
		{
			name:  "valid RFC3339 time",
			input: validTime,
			expected: &TimeRange{
				Type:    TimeTypeSingle,
				Payload: SingleTime{Timestamp: parsedTime},
			},
		},
		{
			name:  "now shortcut",
			input: "now",
			expected: &TimeRange{
				Type:    TimeTypeSingle,
				Payload: SingleTime{Timestamp: now},
			},
		},
		{
			name:  "today shortcut",
			input: "today",
			expected: &TimeRange{
				Type:    TimeTypeSingle,
				Payload: SingleTime{Timestamp: today},
			},
		},
		{
			name:  "yesterday shortcut",
			input: "yesterday",
			expected: &TimeRange{
				Type:    TimeTypeSingle,
				Payload: SingleTime{Timestamp: yesterday},
			},
		},
		{
			name:  "tomorrow shortcut",
			input: "tomorrow",
			expected: &TimeRange{
				Type:    TimeTypeSingle,
				Payload: SingleTime{Timestamp: tomorrow},
			},
		},
		{
			name:  "time range with resolution",
			input: "2023-01-01T00:00:00Z,2023-01-02T00:00:00Z,1",
			expected: &TimeRange{
				Type: TimeTypeList,
				Payload: ListTime{
					Timestamps: []time.Time{
						time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
						time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC),
					},
				},
			},
		},
		{
			name:  "time list",
			input: "2023-01-01T00:00:00Z|2023-01-01T06:00:00Z|2023-01-01T12:00:00Z",
			expected: &TimeRange{
				Type: TimeTypeList,
				Payload: ListTime{
					Timestamps: []time.Time{
						time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
						time.Date(2023, 1, 1, 6, 0, 0, 0, time.UTC),
						time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
			},
		},
		{
			name:        "invalid time format",
			input:       "invalid-time",
			expectedErr: "invalid time format, expected RFC3339",
		},
		{
			name:        "invalid range format",
			input:       "invalid/invalid",
			expectedErr: "unable to parse invalid to RFC3339 time",
		},
		{
			name:        "invalid list format",
			input:       "invalid,invalid",
			expectedErr: "unable to parse invalid to RFC3339 time",
		},
		{
			name:        "unknown shortcut",
			input:       "nextweek",
			expectedErr: "invalid time format, expected RFC3339",
		},
		{
			name:        "empty time",
			input:       "",
			expectedErr: "invalid time format, expected RFC3339",
		},
		{
			name:        "invalid resolution in time range",
			input:       "2023-01-01T00:00:00Z,2023-01-02T00:00:00Z,invalid",
			expectedErr: "resolution invalid has to be an integer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseTime(tt.input)

			if tt.expectedErr != "" {
				assert.ErrorContains(t, err, tt.expectedErr)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected.Type, result.Type)

				switch expected := tt.expected.Payload.(type) {
				case SingleTime:
					actual := result.Payload.(SingleTime).Timestamp.UTC()
					assert.LessOrEqual(t, expected.Timestamp.UTC().Sub(actual).Seconds(), 10.0)
				case RangeTime:
					actual := result.Payload.(RangeTime)
					assert.True(t, expected.Start.Equal(actual.Start))
					assert.True(t, expected.End.Equal(actual.End))
					assert.Equal(t, expected.Resolution, actual.Resolution)
				case ListTime:
					actual := result.Payload.(ListTime)
					assert.Len(t, actual.Timestamps, len(expected.Timestamps))
					for i, ts := range expected.Timestamps {
						assert.True(t, ts.Equal(actual.Timestamps[i]))
					}
				}
			}
		})
	}
}

// TestParseTimeShortcutHelper tests the unexported parseTimeShortcut helper.
func TestParseTimeShortcutHelper(t *testing.T) {
	h0 := 0
	TimeShortcutsMap = BuildTimeShortcuts(map[string]TimeShortcut{
		"today": {Type: "offset", Hours: &h0},
	})

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	result, err := parseTimeShortcut("today")
	assert.NoError(t, err)
	assert.True(t, result.Equal(today))

	_, err = parseTimeShortcut("invalid")
	assert.Error(t, err)
}

// TestExpandTimes tests the ExpandTimes function for all time range types.
func TestExpandTimes(t *testing.T) {
	now := time.Now().UTC()

	t.Run("single value", func(t *testing.T) {
		tr := TimeRange{Type: TimeTypeSingle, Payload: SingleTime{Timestamp: now}}
		times := ExpandTimes(tr)
		assert.Len(t, times, 1)
	})

	t.Run("single pointer", func(t *testing.T) {
		tr := TimeRange{Type: TimeTypeSingle, Payload: &SingleTime{Timestamp: now}}
		times := ExpandTimes(tr)
		assert.Len(t, times, 1)
	})

	t.Run("list value", func(t *testing.T) {
		tr := TimeRange{
			Type:    TimeTypeList,
			Payload: ListTime{Timestamps: []time.Time{now, now.Add(time.Hour)}},
		}
		times := ExpandTimes(tr)
		assert.Len(t, times, 2)
	})

	t.Run("list pointer", func(t *testing.T) {
		tr := TimeRange{
			Type:    TimeTypeList,
			Payload: &ListTime{Timestamps: []time.Time{now}},
		}
		times := ExpandTimes(tr)
		assert.Len(t, times, 1)
	})

	t.Run("range with resolution", func(t *testing.T) {
		start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2025, 1, 1, 6, 0, 0, 0, time.UTC)
		tr := TimeRange{
			Type:    TimeTypeRange,
			Payload: RangeTime{Start: start, End: end, Resolution: 3},
		}
		times := ExpandTimes(tr)
		assert.Len(t, times, 4) // 3+1 inclusive
		assert.True(t, times[0].Equal(start))
		assert.True(t, times[3].Equal(end))
	})

	t.Run("range zero resolution", func(t *testing.T) {
		start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2025, 1, 1, 6, 0, 0, 0, time.UTC)
		tr := TimeRange{
			Type:    TimeTypeRange,
			Payload: RangeTime{Start: start, End: end, Resolution: 0},
		}
		times := ExpandTimes(tr)
		assert.Len(t, times, 2) // just start and end
	})

	t.Run("range inverted span", func(t *testing.T) {
		start := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		end := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		tr := TimeRange{
			Type:    TimeTypeRange,
			Payload: RangeTime{Start: start, End: end, Resolution: 2},
		}
		times := ExpandTimes(tr)
		assert.Len(t, times, 1) // only start when inverted
	})

	t.Run("range pointer", func(t *testing.T) {
		start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2025, 1, 1, 6, 0, 0, 0, time.UTC)
		tr := TimeRange{
			Type:    TimeTypeRange,
			Payload: &RangeTime{Start: start, End: end, Resolution: 2},
		}
		times := ExpandTimes(tr)
		assert.Len(t, times, 3) // 2+1 inclusive
	})

	t.Run("range with ListTime payload", func(t *testing.T) {
		tr := TimeRange{
			Type:    TimeTypeRange,
			Payload: ListTime{Timestamps: []time.Time{now, now.Add(time.Hour)}},
		}
		times := ExpandTimes(tr)
		assert.Len(t, times, 2)
	})

	t.Run("unknown type returns nil", func(t *testing.T) {
		tr := TimeRange{Type: "unknown", Payload: nil}
		times := ExpandTimes(tr)
		assert.Nil(t, times)
	})
}

// TestBuildTimeShortcuts tests the BuildTimeShortcuts function.
func TestBuildTimeShortcuts(t *testing.T) {
	h0 := 0
	hm24 := -24

	shortcuts := map[string]TimeShortcut{
		"now":       {Type: "now"},
		"today":     {Type: "offset", Hours: &h0},
		"yesterday": {Type: "offset", Hours: &hm24},
		"release":   {Type: "fixed", Value: "2025-08-01T00:00:00Z"},
		"unknown":   {Type: "bogus"},
		"noHours":   {Type: "offset"},             // offset with no Hours
		"badFixed":  {Type: "fixed", Value: "XY"}, // invalid fixed
	}

	result := BuildTimeShortcuts(shortcuts)

	t.Run("now returns current time", func(t *testing.T) {
		ts := result["now"]()
		assert.WithinDuration(t, time.Now().UTC(), ts, 2*time.Second)
	})

	t.Run("today returns midnight", func(t *testing.T) {
		ts := result["today"]()
		now := time.Now().UTC()
		expected := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		assert.True(t, ts.Equal(expected))
	})

	t.Run("yesterday returns previous midnight", func(t *testing.T) {
		ts := result["yesterday"]()
		now := time.Now().UTC()
		expected := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
		assert.True(t, ts.Equal(expected))
	})

	t.Run("fixed shortcut", func(t *testing.T) {
		ts := result["release"]()
		expected := time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC)
		assert.True(t, ts.Equal(expected))
	})

	t.Run("unknown type falls back to now", func(t *testing.T) {
		ts := result["unknown"]()
		assert.WithinDuration(t, time.Now().UTC(), ts, 2*time.Second)
	})

	t.Run("offset with nil hours falls back to now", func(t *testing.T) {
		ts := result["noHours"]()
		assert.WithinDuration(t, time.Now().UTC(), ts, 2*time.Second)
	})

	t.Run("invalid fixed falls back to now", func(t *testing.T) {
		ts := result["badFixed"]()
		assert.WithinDuration(t, time.Now().UTC(), ts, 2*time.Second)
	})
}

// TestBuildRangeTimeShortcuts tests the BuildRangeTimeShortcuts function.
func TestBuildRangeTimeShortcuts(t *testing.T) {
	h72 := 72
	h168 := 168
	h0 := 0
	hNeg := -24

	shortcuts := map[string]TimeShortcut{
		"3day":      {Type: "range_offset", Hours: &h72},
		"7day":      {Type: "range_offset", Hours: &h168},
		"zeroHours": {Type: "range_offset", Hours: &h0},
		"negHours":  {Type: "range_offset", Hours: &hNeg},
		"nilHours":  {Type: "range_offset"},
		"today":     {Type: "offset", Hours: &h0}, // should be skipped (not range_offset)
		"now":       {Type: "now"},                // should be skipped
	}

	result := BuildRangeTimeShortcuts(shortcuts, nil)

	t.Run("only range_offset entries included", func(t *testing.T) {
		assert.Len(t, result, 2) // only 3day and 7day (valid hours > 0)
		assert.Contains(t, result, "3day")
		assert.Contains(t, result, "7day")
		assert.NotContains(t, result, "zeroHours")
		assert.NotContains(t, result, "negHours")
		assert.NotContains(t, result, "nilHours")
		assert.NotContains(t, result, "today")
		assert.NotContains(t, result, "now")
	})

	t.Run("3day returns correct range", func(t *testing.T) {
		rt := result["3day"]()
		now := time.Now().UTC()
		expectedStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		expectedEnd := expectedStart.Add(72 * time.Hour)
		assert.True(t, rt.Start.Equal(expectedStart))
		assert.True(t, rt.End.Equal(expectedEnd))
		assert.Equal(t, 0, rt.Resolution)
	})

	t.Run("7day returns correct range", func(t *testing.T) {
		rt := result["7day"]()
		now := time.Now().UTC()
		expectedStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		expectedEnd := expectedStart.Add(168 * time.Hour)
		assert.True(t, rt.Start.Equal(expectedStart))
		assert.True(t, rt.End.Equal(expectedEnd))
	})
}

// TestParseTime_RangeShortcuts tests that range shortcuts like 3day/7day/10day work in ParseTime.
func TestParseTime_RangeShortcuts(t *testing.T) {
	h72 := 72
	h240 := 240

	TimeRangeShortcutsMap = BuildRangeTimeShortcuts(map[string]TimeShortcut{
		"3day":  {Type: "range_offset", Hours: &h72},
		"10day": {Type: "range_offset", Hours: &h240},
	}, nil)

	t.Run("3day shortcut returns range", func(t *testing.T) {
		result, err := ParseTime("3day")
		assert.NoError(t, err)
		assert.Equal(t, TimeTypeRange, result.Type)
		rt, ok := result.Payload.(RangeTime)
		assert.True(t, ok)
		now := time.Now().UTC()
		expectedStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		expectedEnd := expectedStart.Add(72 * time.Hour)
		assert.True(t, rt.Start.Equal(expectedStart))
		assert.True(t, rt.End.Equal(expectedEnd))
		assert.Equal(t, 0, rt.Resolution)
	})

	t.Run("10day shortcut returns range", func(t *testing.T) {
		result, err := ParseTime("10day")
		assert.NoError(t, err)
		assert.Equal(t, TimeTypeRange, result.Type)
		rt, ok := result.Payload.(RangeTime)
		assert.True(t, ok)
		now := time.Now().UTC()
		expectedStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		expectedEnd := expectedStart.Add(240 * time.Hour)
		assert.True(t, rt.Start.Equal(expectedStart))
		assert.True(t, rt.End.Equal(expectedEnd))
	})

	t.Run("unknown shortcut still fails", func(t *testing.T) {
		_, err := ParseTime("99day")
		assert.Error(t, err)
	})

	// Reset to avoid polluting other tests
	TimeRangeShortcutsMap = map[string]func() RangeTime{}
}

// TestFormatTimeDataToRFC3339 tests the time formatting helper.
func TestFormatTimeDataToRFC3339(t *testing.T) {
	input := time.Date(2025, 3, 1, 12, 30, 45, 123456789, time.UTC)
	result := FormatTimeDataToRFC3339(input)
	// Sub-second precision is truncated
	assert.Equal(t, 0, result.Nanosecond())
	assert.Equal(t, 2025, result.Year())
	assert.Equal(t, time.March, result.Month())
}

// TestParseTimeRange_NoResolution tests time range without resolution.
func TestParseTimeRange_NoResolution(t *testing.T) {
	result, err := ParseTime("2023-01-01T00:00:00Z,2023-01-02T00:00:00Z")
	assert.NoError(t, err)
	assert.Equal(t, TimeTypeRange, result.Type)
	rt, ok := result.Payload.(RangeTime)
	assert.True(t, ok)
	assert.Equal(t, 0, rt.Resolution)
}

// TestParseTimeRange_InvalidEndTime tests invalid end time in range.
func TestParseTimeRange_InvalidEndTime(t *testing.T) {
	_, err := ParseTime("2023-01-01T00:00:00Z,invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to parse")
}

// TestParseTimeRange_NegativeResolution tests negative resolution.
func TestParseTimeRange_NegativeResolution(t *testing.T) {
	_, err := ParseTime("2023-01-01T00:00:00Z,2023-01-02T00:00:00Z,-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "positive integer")
}

// TestParseTimeRange_EndBeforeStart tests end time before start time.
func TestParseTimeRange_EndBeforeStart(t *testing.T) {
	_, err := ParseTime("2023-01-02T00:00:00Z,2023-01-01T00:00:00Z,3")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "end time must be after start time")
}

// TestParseTimeList_InvalidItem tests invalid item in pipe-separated list.
func TestParseTimeList_InvalidItem(t *testing.T) {
	_, err := ParseTime("2023-01-01T00:00:00Z|invalid")
	assert.Error(t, err)
}

// TestCaseInsensitiveTimeShortcuts verifies that time shortcuts
// are matched case-insensitively.
func TestCaseInsensitiveTimeShortcuts(t *testing.T) {
	h0 := 0
	h72 := 72

	TimeShortcutsMap = BuildTimeShortcuts(map[string]TimeShortcut{
		"now":   {Type: "now"},
		"today": {Type: "offset", Hours: &h0},
	})
	TimeRangeShortcutsMap = BuildRangeTimeShortcuts(map[string]TimeShortcut{
		"3day": {Type: "range_offset", Hours: &h72},
	}, nil)

	t.Run("NOW resolves same as now", func(t *testing.T) {
		res, err := ParseTime("NOW")
		require.NoError(t, err)
		assert.Equal(t, TimeTypeSingle, res.Type)
	})

	t.Run("Now resolves same as now", func(t *testing.T) {
		res, err := ParseTime("Now")
		require.NoError(t, err)
		assert.Equal(t, TimeTypeSingle, res.Type)
	})

	t.Run("TODAY resolves same as today", func(t *testing.T) {
		res, err := ParseTime("TODAY")
		require.NoError(t, err)
		assert.Equal(t, TimeTypeSingle, res.Type)
	})

	t.Run("3DAY resolves same as 3day", func(t *testing.T) {
		res, err := ParseTime("3DAY")
		require.NoError(t, err)
		assert.Equal(t, TimeTypeRange, res.Type)
	})

	t.Run("3Day resolves same as 3day", func(t *testing.T) {
		res, err := ParseTime("3Day")
		require.NoError(t, err)
		assert.Equal(t, TimeTypeRange, res.Type)
	})

	t.Run("RFC3339 timestamps still work", func(t *testing.T) {
		res, err := ParseTime("2025-07-23T17:56:48Z")
		require.NoError(t, err)
		assert.Equal(t, TimeTypeSingle, res.Type)
	})

	// Reset
	TimeRangeShortcutsMap = map[string]func() RangeTime{}
}
