package time

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseTimeSlashRange covers the slash-delimited range grammar added for
// the structured v2 request body.
func TestParseTimeSlashRange(t *testing.T) {
	t.Run("three-part range with an ISO-8601 step", func(t *testing.T) {
		tr, err := ParseTime("2026-04-26T00:00:00Z/2026-04-27T00:00:00Z/PT1H")
		require.NoError(t, err)
		assert.Equal(t, TimeTypeList, tr.Type)
		expanded := ExpandTimes(*tr)
		assert.Len(t, expanded, 25) // 24h inclusive of both endpoints
		assert.True(t, expanded[0].Equal(time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)))
		assert.True(t, expanded[len(expanded)-1].Equal(time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)))
	})

	t.Run("three-part range with an integer-hour step", func(t *testing.T) {
		tr, err := ParseTime("2026-04-26T00:00:00Z/2026-04-26T12:00:00Z/6")
		require.NoError(t, err)
		assert.Len(t, ExpandTimes(*tr), 3) // 00:00, 06:00, 12:00
	})

	t.Run("step that does not divide the span still includes the end", func(t *testing.T) {
		tr, err := ParseTime("2026-04-26T00:00:00Z/2026-04-26T05:00:00Z/PT2H")
		require.NoError(t, err)
		expanded := ExpandTimes(*tr)
		last := expanded[len(expanded)-1]
		assert.True(t, last.Equal(time.Date(2026, 4, 26, 5, 0, 0, 0, time.UTC)))
	})

	t.Run("two-part range", func(t *testing.T) {
		tr, err := ParseTime("2026-04-26T00:00:00Z/2026-04-27T00:00:00Z")
		require.NoError(t, err)
		assert.Equal(t, TimeTypeRange, tr.Type)
	})

	t.Run("rejects end before start", func(t *testing.T) {
		_, err := ParseTime("2026-04-27T00:00:00Z/2026-04-26T00:00:00Z/PT1H")
		require.Error(t, err)
	})

	t.Run("rejects a non-RFC3339 bound", func(t *testing.T) {
		_, err := ParseTime("yesterday/today/PT1H")
		require.Error(t, err)
	})
}

// TestParseISO8601Duration covers the duration subset used for range steps.
func TestParseISO8601Duration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"PT1H", time.Hour},
		{"PT30M", 30 * time.Minute},
		{"PT1H30M", 90 * time.Minute},
		{"P1D", 24 * time.Hour},
		{"P1DT12H", 36 * time.Hour},
		{"P1W", 7 * 24 * time.Hour},
		{"PT45S", 45 * time.Second},
	}
	for _, tc := range cases {
		got, err := parseISO8601Duration(tc.in)
		require.NoError(t, err, tc.in)
		assert.Equal(t, tc.want, got, tc.in)
	}

	for _, bad := range []string{"P", "PT", "P1M", "PT0H", "1H", "PTH", "PT1X"} {
		_, err := parseISO8601Duration(bad)
		assert.Error(t, err, bad)
	}
}

// TestTimeRangeUnmarshalJSON covers both wire forms accepted by the custom
// TimeRange JSON decoder.
func TestTimeRangeUnmarshalJSON(t *testing.T) {
	t.Run("string form single timestamp", func(t *testing.T) {
		var tr TimeRange
		require.NoError(t, json.Unmarshal([]byte(`"2026-05-22T00:00:00Z"`), &tr))
		assert.Equal(t, TimeTypeSingle, tr.Type)
		single, ok := tr.Payload.(SingleTime)
		require.True(t, ok)
		assert.True(t, single.Timestamp.Equal(time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)))
	})

	t.Run("string form slash range", func(t *testing.T) {
		var tr TimeRange
		body := `"2026-04-26T00:00:00Z/2026-04-27T00:00:00Z/PT1H"`
		require.NoError(t, json.Unmarshal([]byte(body), &tr))
		assert.Equal(t, TimeTypeList, tr.Type)
		assert.Len(t, ExpandTimes(tr), 25)
	})

	t.Run("object form single", func(t *testing.T) {
		var tr TimeRange
		body := `{"type":"single","payload":{"timestamp":"2026-05-22T00:00:00Z"}}`
		require.NoError(t, json.Unmarshal([]byte(body), &tr))
		single, ok := tr.Payload.(SingleTime)
		require.True(t, ok)
		assert.True(t, single.Timestamp.Equal(time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)))
	})

	t.Run("object form range decodes into a typed payload", func(t *testing.T) {
		var tr TimeRange
		body := `{"type":"range","payload":{"start":"2026-04-26T00:00:00Z",` +
			`"end":"2026-04-27T00:00:00Z","resolution":6}}`
		require.NoError(t, json.Unmarshal([]byte(body), &tr))
		rng, ok := tr.Payload.(RangeTime)
		require.True(t, ok)
		assert.Equal(t, 6, rng.Resolution)
	})

	t.Run("object form list", func(t *testing.T) {
		var tr TimeRange
		body := `{"type":"list","payload":{"timestamps":` +
			`["2026-04-26T00:00:00Z","2026-04-27T00:00:00Z"]}}`
		require.NoError(t, json.Unmarshal([]byte(body), &tr))
		list, ok := tr.Payload.(ListTime)
		require.True(t, ok)
		assert.Len(t, list.Timestamps, 2)
	})

	t.Run("rejects null", func(t *testing.T) {
		var tr TimeRange
		require.Error(t, json.Unmarshal([]byte(`null`), &tr))
	})

	t.Run("rejects an unknown object type", func(t *testing.T) {
		var tr TimeRange
		require.Error(t, json.Unmarshal([]byte(`{"type":"epoch","payload":{}}`), &tr))
	})

	t.Run("rejects an object missing its payload", func(t *testing.T) {
		var tr TimeRange
		require.Error(t, json.Unmarshal([]byte(`{"type":"single"}`), &tr))
	})
}
