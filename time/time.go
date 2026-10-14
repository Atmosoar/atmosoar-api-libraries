// Package time provides parsers and expansion helpers for time query
// parameters used by Atmosoar services (single timestamp, range with
// resolution, list, and named time shortcuts).
//
// NOTE: This package is named "time" which shadows the standard library
// package of the same name. Consumers that also need stdlib time should
// alias this import, e.g.:
//
//	import (
//	    "time"
//	    atmotime "atmosoar.io/atmosoar-api-libraries/time"
//	)
//
// Inside this package itself, the stdlib "time" is imported normally and
// referenced as `time.Time` without conflict — the package's own name is
// not used to qualify identifiers from within the package.
package time

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// TimeRange represents different time parameter formats.
// Name preserved from MMA-171's in-tree implementation so the cutover is a pure import swap.
//
//nolint:revive // parity: renaming would break the MMA-171 drop-in swap contract.
type TimeRange struct {
	Type    TimeRangeType `json:"type"`
	Payload interface{}   `json:"payload"`
}

// TimeRangeType is a type to not use a raw string for readeablity.
//
//nolint:revive // parity: renaming would break the MMA-171 drop-in swap contract.
type TimeRangeType string

const (
	// TimeTypeSingle used for single time points
	TimeTypeSingle TimeRangeType = "single"
	// TimeTypeRange used for a range of points using start, end and a resolution
	TimeTypeRange TimeRangeType = "range"
	// TimeTypeList used for a list of time points
	TimeTypeList TimeRangeType = "list"
)

// SingleTime just the time
type SingleTime struct {
	Timestamp time.Time `json:"timestamp" validate:"required"`
}

// RangeTime holds end and start time and a resolution, one of the possible types of time input in the url query
type RangeTime struct {
	Start      time.Time `json:"start"      validate:"required"`
	End        time.Time `json:"end"        validate:"required"`
	Resolution int       `json:"resolution" validate:"required"` // in hours
}

// ListTime holds a list of times, one of the possible types of time input in the url query
type ListTime struct {
	Timestamps []time.Time `json:"timestamps" validate:"required"`
}

// MaxTimestamps caps how many timestamps a single time parameter may expand
// to. Like the location package's expansion caps it is enforced at PARSE
// time, so an oversized range is a clean 400-style error instead of an
// allocation the process cannot survive.
//
// Memory math (a time.Time is 24 bytes — wall, ext, loc):
//
//   - "start,end,hours" and "start/end/PT1S" step the span at a caller-chosen
//     step regardless of its length: 1980→2050 at one second is ~2.2e9
//     timestamps ≈ 53 GB, hourly still ~613,000.
//   - ExpandTimes walks a JSON-decoded RangeTime.Resolution that only
//     decodeTimePayload validated; a hand-built one must still not be able
//     to ask for an unbounded slice.
//
// 10,000 is over a year of hourly steps (8,760) and ~240 KB expanded — far
// beyond /impact's days=10 (241 hourly slots) yet cheap in both memory and
// downstream per-timestamp work.
const MaxTimestamps = 10000

// MaxResolutionHours bounds the hour step of a range ("start,end,hours" and
// RangeTime.Resolution) at 100 years. That is far below the point where
// time.Duration(hours)*time.Hour overflows (about 292 years), and any larger
// value is a typo, not a step.
const MaxResolutionHours = 24 * 366 * 100

// registryMu guards concurrent access to the package-level shortcut
// registries (TimeShortcutsMap, TimeRangeShortcutsMap) via the
// Register*/Lookup* accessors.
//
// BUG-001 / ADR-001: direct writes to the deprecated exported vars below do
// NOT take this lock — they remain race-prone for backwards compatibility and
// will be removed in v2.
var registryMu sync.RWMutex

// TimeShortcutsMap holds the time shortcuts defined in the config.toml.
//
// Deprecated: use RegisterTimeShortcut / LookupTimeShortcut instead. Direct
// access to this map is not thread-safe and will be removed in v2.
var TimeShortcutsMap = map[string]func() time.Time{}

// TimeRangeShortcutsMap holds the range_offset time shortcuts defined in the config.toml.
//
// Deprecated: use RegisterTimeRangeShortcut / LookupTimeRangeShortcut instead.
// Direct access to this map is not thread-safe and will be removed in v2.
var TimeRangeShortcutsMap = map[string]func() RangeTime{}

// RegisterTimeShortcut adds or replaces a single-time shortcut entry under
// the package-level lock.
func RegisterTimeShortcut(name string, fn func() time.Time) {
	registryMu.Lock()
	defer registryMu.Unlock()
	TimeShortcutsMap[name] = fn
}

// LookupTimeShortcut returns the time-shortcut function registered under name
// and whether it exists. Safe for concurrent use.
func LookupTimeShortcut(name string) (func() time.Time, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	fn, ok := TimeShortcutsMap[name]
	return fn, ok
}

// RegisterTimeRangeShortcut adds or replaces a range-shortcut entry under
// the package-level lock.
func RegisterTimeRangeShortcut(name string, fn func() RangeTime) {
	registryMu.Lock()
	defer registryMu.Unlock()
	TimeRangeShortcutsMap[name] = fn
}

// LookupTimeRangeShortcut returns the range-shortcut function registered
// under name and whether it exists. Safe for concurrent use.
func LookupTimeRangeShortcut(name string) (func() RangeTime, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	fn, ok := TimeRangeShortcutsMap[name]
	return fn, ok
}

// ParseTime takes in the raw time string from the querie and tries parsing it to one of the TimeRangeTypes.
// 150: The input is lowercased before shortcut map lookups so that
// e.g. "Now", "NOW", "now" and "3Day", "3DAY", "3day" all resolve identically.
func ParseTime(timeStr string) (*TimeRange, error) {
	// 150: Lowercase for case-insensitive shortcut matching.
	timeLower := strings.ToLower(timeStr)

	// Check range shortcuts (e.g. 3day, 7day, 10day) before delimiter-based parsing
	// BUG-001: read via locked accessor.
	if fn, exists := LookupTimeRangeShortcut(timeLower); exists {
		rt := fn()
		return &TimeRange{
			Type:    TimeTypeRange,
			Payload: rt,
		}, nil
	}

	// Try parsing as time list (pipe separated)
	if strings.Contains(timeStr, "|") {
		return parseTimeList(timeStr)
	}

	// Try parsing as a slash-delimited range (start/end or start/end/step).
	// RFC3339 timestamps never contain '/', so the separator is unambiguous.
	if strings.Contains(timeStr, "/") {
		return parseTimeRangeSlash(timeStr)
	}

	// Try parsing as time range (start/end)
	if strings.Contains(timeStr, ",") {
		return parseTimeRange(timeStr)
	}

	// Try parsing as single timestamp
	return parseSingleTime(timeStr)
}

func parseSingleTime(timeStr string) (*TimeRange, error) {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err == nil {
		return &TimeRange{
			Type:    TimeTypeSingle,
			Payload: SingleTime{Timestamp: t},
		}, nil
	}

	t, err = parseTimeShortcut(timeStr)
	if err == nil {
		return &TimeRange{
			Type:    TimeTypeSingle,
			Payload: SingleTime{Timestamp: t},
		}, nil
	}
	return nil, fmt.Errorf("invalid time format, expected RFC3339")
}

func parseTimeRange(rangeStr string) (*TimeRange, error) {
	parts := strings.Split(rangeStr, ",")
	if len(parts) != 2 && len(parts) != 3 {
		return nil, fmt.Errorf("invalid time range format")
	}

	start, err := time.Parse(time.RFC3339, parts[0])
	if err != nil {
		return nil, fmt.Errorf("unable to parse %s to RFC3339 time", parts[0])
	}

	end, err := time.Parse(time.RFC3339, parts[1])
	if err != nil {
		return nil, fmt.Errorf("unable to parse %s to RFC3339 time", parts[1])
	}

	// If no resolution is provided, return as a simple range
	if len(parts) < 3 {
		return &TimeRange{
			Type: TimeTypeRange,
			Payload: RangeTime{
				Start:      start,
				End:        end,
				Resolution: 0,
			},
		}, nil
	}

	hours, err := strconv.Atoi(parts[2])
	if err != nil {
		return nil, fmt.Errorf("resolution %s has to be an integer", parts[2])
	}
	if hours <= 0 {
		return nil, fmt.Errorf("resolution must be a positive integer")
	}
	if hours > MaxResolutionHours {
		return nil, fmt.Errorf("resolution must be <= %d hours, got %d", MaxResolutionHours, hours)
	}
	if !end.After(start) {
		return nil, fmt.Errorf("end time must be after start time")
	}

	// The third element is the step in whole hours, the same meaning as the
	// slash form's bare-integer step: "T00:00Z,T12:00Z,3" is 00, 03, 06, 09
	// and 12. Until ATMO-311 it was read as a number of segments, which the
	// public docs never said.
	timestamps, err := stepTimestamps(rangeStr, start, end, time.Duration(hours)*time.Hour)
	if err != nil {
		return nil, err
	}
	return &TimeRange{
		Type:    TimeTypeList,
		Payload: ListTime{Timestamps: timestamps},
	}, nil
}

// parseTimeRangeSlash parses a slash-delimited range: "start/end" or
// "start/end/step". start and end are RFC3339 timestamps; step is either an
// ISO-8601 duration (e.g. "PT1H") or a bare integer number of hours. A
// three-part range is expanded into a TimeTypeList of stepped timestamps
// (end always included); a two-part range becomes a TimeTypeRange.
func parseTimeRangeSlash(rangeStr string) (*TimeRange, error) {
	parts := strings.Split(rangeStr, "/")
	if len(parts) != 2 && len(parts) != 3 {
		return nil, fmt.Errorf("invalid time range %q, expected start/end or start/end/step", rangeStr)
	}

	start, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, fmt.Errorf("unable to parse %s to RFC3339 time", parts[0])
	}
	end, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, fmt.Errorf("unable to parse %s to RFC3339 time", parts[1])
	}
	if !end.After(start) {
		return nil, fmt.Errorf("end time must be after start time")
	}

	if len(parts) == 2 {
		return &TimeRange{
			Type:    TimeTypeRange,
			Payload: RangeTime{Start: start, End: end, Resolution: 0},
		}, nil
	}

	step, err := parseStepDuration(strings.TrimSpace(parts[2]))
	if err != nil {
		return nil, err
	}

	timestamps, err := stepTimestamps(rangeStr, start, end, step)
	if err != nil {
		return nil, err
	}

	return &TimeRange{
		Type:    TimeTypeList,
		Payload: ListTime{Timestamps: timestamps},
	}, nil
}

// stepCount is the number of points stepTimestamps emits for span at step:
// one per step from the start, plus the end boundary when the step does not
// divide the span.
func stepCount(span, step time.Duration) int64 {
	count := int64(span/step) + 1
	if span%step != 0 {
		count++
	}
	return count
}

// stepTimestamps walks start..end at step and returns every point, the end
// boundary always included. The step is caller-supplied and the span is
// unbounded ("1980-01-01/2050-01-01/PT1S" is ~2.2e9 points), so the
// expansion is sized up front and refused beyond MaxTimestamps instead of
// appending until the process dies.
func stepTimestamps(rangeStr string, start, end time.Time, step time.Duration) ([]time.Time, error) {
	if step <= 0 {
		return nil, fmt.Errorf("range step must be positive")
	}
	count := stepCount(end.Sub(start), step)
	if count > MaxTimestamps {
		return nil, fmt.Errorf(
			"time range %q expands to %d timestamps, more than the %d maximum; use a coarser step",
			rangeStr, count, MaxTimestamps,
		)
	}

	timestamps := make([]time.Time, 0, count)
	for t := start; !t.After(end); t = t.Add(step) {
		timestamps = append(timestamps, t.UTC())
	}
	// Include the end boundary when the step does not divide the span evenly.
	if len(timestamps) == 0 || !timestamps[len(timestamps)-1].Equal(end.UTC()) {
		timestamps = append(timestamps, end.UTC())
	}
	return timestamps, nil
}

// parseStepDuration parses a range step expressed either as an ISO-8601
// duration ("PT1H", "PT30M", "P1DT12H") or as a bare positive integer number
// of hours ("1", "6"). The returned duration is always strictly positive.
func parseStepDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("missing range step")
	}
	if strings.HasPrefix(strings.ToUpper(s), "P") {
		return parseISO8601Duration(strings.ToUpper(s))
	}
	hours, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf(
			"invalid range step %q, expected an ISO-8601 duration (e.g. PT1H) or integer hours", s,
		)
	}
	if hours <= 0 {
		return 0, fmt.Errorf("range step hours must be a positive integer")
	}
	return time.Duration(hours) * time.Hour, nil
}

// parseISO8601Duration parses the subset of ISO-8601 durations Atmosoar uses
// for range steps: weeks, days, hours, minutes, and seconds. Month and year
// designators are intentionally rejected because their length is ambiguous.
// The input is expected to be upper-cased and to start with 'P'.
func parseISO8601Duration(s string) (time.Duration, error) {
	rest := strings.TrimPrefix(s, "P")
	if rest == "" {
		return 0, fmt.Errorf("invalid ISO-8601 duration %q", s)
	}

	datePart, timePart := rest, ""
	if i := strings.IndexByte(rest, 'T'); i >= 0 {
		datePart, timePart = rest[:i], rest[i+1:]
	}

	var total time.Duration
	accumulate := func(part string, units map[byte]time.Duration) error {
		var num strings.Builder
		for i := 0; i < len(part); i++ {
			ch := part[i]
			if ch >= '0' && ch <= '9' {
				num.WriteByte(ch)
				continue
			}
			unit, ok := units[ch]
			if !ok {
				return fmt.Errorf("invalid ISO-8601 duration %q: unsupported designator %q", s, string(ch))
			}
			if num.Len() == 0 {
				return fmt.Errorf("invalid ISO-8601 duration %q: designator %q has no value", s, string(ch))
			}
			n, err := strconv.Atoi(num.String())
			if err != nil {
				return fmt.Errorf("invalid ISO-8601 duration %q: %w", s, err)
			}
			total += time.Duration(n) * unit
			num.Reset()
		}
		if num.Len() != 0 {
			return fmt.Errorf("invalid ISO-8601 duration %q: trailing value with no designator", s)
		}
		return nil
	}

	if err := accumulate(datePart, map[byte]time.Duration{
		'W': 7 * 24 * time.Hour,
		'D': 24 * time.Hour,
	}); err != nil {
		return 0, err
	}
	if err := accumulate(timePart, map[byte]time.Duration{
		'H': time.Hour,
		'M': time.Minute,
		'S': time.Second,
	}); err != nil {
		return 0, err
	}
	if total <= 0 {
		return 0, fmt.Errorf("ISO-8601 duration %q must be a positive, non-zero duration", s)
	}
	return total, nil
}

// UnmarshalJSON decodes a TimeRange from JSON. It accepts two wire forms:
//
//   - a string in the v1 query-parameter grammar (an RFC3339 timestamp, a
//     "start/end/step" or "start,end,resolution" range, a pipe-delimited
//     list, or a configured shortcut such as "now"), parsed via ParseTime;
//     and
//   - the structured object {"type":..,"payload":..} produced by the default
//     marshaller, whose payload is decoded into the concrete payload struct
//     named by type.
//
// Supporting both lets a POST /v2 JSON body accept the same compact strings
// the GET /v1 query string accepts.
func (tr *TimeRange) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return fmt.Errorf("timeRange: missing value")
	}

	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return fmt.Errorf("timeRange: %w", err)
		}
		parsed, err := ParseTime(s)
		if err != nil {
			return fmt.Errorf("timeRange %q: %w", s, err)
		}
		*tr = *parsed
		return nil
	}

	if trimmed[0] == '{' {
		var raw struct {
			Type    TimeRangeType   `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(trimmed, &raw); err != nil {
			return fmt.Errorf("timeRange: %w", err)
		}
		payload, err := decodeTimePayload(raw.Type, raw.Payload)
		if err != nil {
			return err
		}
		tr.Type = raw.Type
		tr.Payload = payload
		return nil
	}

	return fmt.Errorf("timeRange: expected a JSON string or a {type,payload} object")
}

// decodeTimePayload decodes a raw JSON payload into the concrete payload
// struct for the given time range type.
func decodeTimePayload(t TimeRangeType, raw json.RawMessage) (interface{}, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("timeRange: type %q is missing its payload", t)
	}
	switch t {
	case TimeTypeSingle:
		var p SingleTime
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("timeRange: invalid single payload: %w", err)
		}
		return p, nil
	case TimeTypeRange:
		var p RangeTime
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("timeRange: invalid range payload: %w", err)
		}
		// The structured payload bypasses parseTimeRange, so this is the
		// first code to see Resolution (hours): bound the step and the size
		// of the expansion here, so ExpandTimes never has to refuse anything.
		if p.Resolution > MaxResolutionHours {
			return nil, fmt.Errorf(
				"timeRange: resolution must be <= %d hours, got %d", MaxResolutionHours, p.Resolution,
			)
		}
		if p.Resolution > 0 && p.End.After(p.Start) {
			step := time.Duration(p.Resolution) * time.Hour
			if count := stepCount(p.End.Sub(p.Start), step); count > MaxTimestamps {
				return nil, fmt.Errorf(
					"timeRange: range expands to %d timestamps, more than the %d maximum; use a coarser resolution",
					count, MaxTimestamps,
				)
			}
		}
		return p, nil
	case TimeTypeList:
		var p ListTime
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("timeRange: invalid list payload: %w", err)
		}
		if len(p.Timestamps) > MaxTimestamps {
			return nil, fmt.Errorf(
				"timeRange: list has %d entries, more than the %d maximum",
				len(p.Timestamps), MaxTimestamps,
			)
		}
		return p, nil
	default:
		return nil, fmt.Errorf("timeRange: unknown type %q", t)
	}
}

func parseTimeList(listStr string) (*TimeRange, error) {
	timeStrs := strings.Split(listStr, "|")
	if len(timeStrs) > MaxTimestamps {
		return nil, fmt.Errorf(
			"time list has %d entries, more than the %d maximum", len(timeStrs), MaxTimestamps,
		)
	}
	times := make([]time.Time, 0, len(timeStrs))

	for _, ts := range timeStrs {
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return nil, err
		}
		times = append(times, t)
	}

	return &TimeRange{
		Type:    TimeTypeList,
		Payload: ListTime{Timestamps: times},
	}, nil
}

// parseTimeShortcut returns the appropriate time for a given shortcut.
// 150: Lowercases the shortcut before lookup for case-insensitive matching.
func parseTimeShortcut(shortcut string) (time.Time, error) {
	// BUG-001: read via locked accessor.
	if fn, exists := LookupTimeShortcut(strings.ToLower(shortcut)); exists {
		return fn(), nil
	}
	return time.Time{}, fmt.Errorf("input does not map to valid shortcut")
}

// ExpandTimes resolves a TimeRange into a flat slice of time.Time values.
func ExpandTimes(tr TimeRange) []time.Time {
	switch tr.Type {
	case TimeTypeSingle:
		if v, ok := tr.Payload.(SingleTime); ok {
			return []time.Time{v.Timestamp}
		}
		if v, ok := tr.Payload.(*SingleTime); ok && v != nil {
			return []time.Time{v.Timestamp}
		}

	case TimeTypeList:
		if v, ok := tr.Payload.(ListTime); ok {
			return v.Timestamps
		}
		if v, ok := tr.Payload.(*ListTime); ok && v != nil {
			return v.Timestamps
		}

	case TimeTypeRange:
		// Support both: (1) raw RangeTime payload, (2) already-expanded ListTime payload.
		if v, ok := tr.Payload.(ListTime); ok {
			// If parse layer already expanded the range → just return the list
			return v.Timestamps
		}
		if v, ok := tr.Payload.(*ListTime); ok && v != nil {
			return v.Timestamps
		}

		var rt *RangeTime
		if v, ok := tr.Payload.(RangeTime); ok {
			rt = &v
		} else if v, ok := tr.Payload.(*RangeTime); ok && v != nil {
			rt = v
		}
		if rt != nil {
			return expandRangeTime(*rt)
		}
	}
	return nil
}

// expandRangeTime turns a RangeTime into its inclusive list of timestamps:
// start and end only when Resolution is 0, otherwise one point every
// Resolution hours from the start, with the end boundary always included.
// Split out of ExpandTimes so the range arm stays flat and readable.
func expandRangeTime(rt RangeTime) []time.Time {
	start, end, res := rt.Start, rt.End, rt.Resolution
	if !end.After(start) {
		// Non-positive span → return start only
		return []time.Time{start}
	}
	if res <= 0 {
		return []time.Time{start, end}
	}
	// Defence in depth: decodeTimePayload bounds Resolution and the expansion
	// at parse time, but this function has no error return and a hand-built
	// RangeTime must neither overflow the step arithmetic nor walk an
	// unbounded span. Clamp the hours, then coarsen the step until the list
	// fits MaxTimestamps; a coarser expansion beats crashing.
	if res > MaxResolutionHours {
		res = MaxResolutionHours
	}
	span := end.Sub(start)
	step := time.Duration(res) * time.Hour
	if stepCount(span, step) > MaxTimestamps {
		// The smallest whole-hour step that fits: MaxTimestamps-2 full steps
		// leave room for the start and an unaligned end boundary.
		step = span / time.Duration(MaxTimestamps-2)
		step = ((step + time.Hour - 1) / time.Hour) * time.Hour
	}
	out, err := stepTimestamps("", start, end, step)
	if err != nil {
		// Unreachable after the coarsening above; never return nothing.
		return []time.Time{start.UTC(), end.UTC()}
	}
	return out
}

// --- Shortcut builders (formerly middleware/config/shortcut_builder.go) ---

// TimeShortcutRaw holds the raw data of one time shortcut.
//
//nolint:revive // parity: renaming would break the MMA-171 drop-in swap contract.
type TimeShortcutRaw struct {
	Type  string `mapstructure:"type"`            // "now" | "offset" | "fixed"
	Hours *int   `mapstructure:"hours,omitempty"` // for offset
	Value string `mapstructure:"value,omitempty"` // RFC3339 for fixed
}

// TimeShortcut holds the type of the time shortcut "now", "offset" in hours or "fixed"
// based on this Hours and Value, with value being optional.
//
//nolint:revive // parity: renaming would break the MMA-171 drop-in swap contract.
type TimeShortcut struct {
	Type  string
	Hours *int
	Value string `mapstructure:"value,omitempty"`
}

// BuildTimeShortcuts creates the dynamic timeShortcutsMap directly from a map[string]TimeShortcut.
func BuildTimeShortcuts(shortcuts map[string]TimeShortcut) map[string]func() time.Time {
	m := make(map[string]func() time.Time, len(shortcuts))
	for name, sc := range shortcuts {
		// capture loop vars
		nameLocal := name
		scLocal := sc

		m[nameLocal] = func() time.Time {
			now := time.Now().UTC()
			switch scLocal.Type {
			case "now":
				return FormatTimeDataToRFC3339(now)

			case "offset":
				if scLocal.Hours == nil {
					// fallback if hours not provided
					return FormatTimeDataToRFC3339(now)
				}
				base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
				return FormatTimeDataToRFC3339(base.Add(time.Duration(*scLocal.Hours) * time.Hour))

			case "fixed":
				if t, err := time.Parse(time.RFC3339, scLocal.Value); err == nil {
					return FormatTimeDataToRFC3339(t.UTC())
				}
				// invalid fallback
				return FormatTimeDataToRFC3339(now)

			default:
				// unknown type -> fallback
				return FormatTimeDataToRFC3339(now)
			}
		}
	}
	return m
}

// BuildRangeTimeShortcuts creates the dynamic timeRangeShortcutsMap from range_offset entries.
// Each returned func computes: Start = UTC midnight of current day, End = Start + hours,
// at hourly resolution, so "3day" expands to 73 points rather than its two
// boundaries (ATMO-311). Entries with hours <= 0 are logged as warnings and skipped.
func BuildRangeTimeShortcuts(shortcuts map[string]TimeShortcut, log *zap.SugaredLogger) map[string]func() RangeTime {
	m := make(map[string]func() RangeTime)
	for name, sc := range shortcuts {
		if sc.Type != "range_offset" {
			continue
		}

		if sc.Hours == nil || *sc.Hours <= 0 {
			if log != nil {
				log.Warnw("skipping range_offset shortcut with invalid hours",
					"shortcut", name,
					"hours", sc.Hours,
				)
			}
			continue
		}

		hours := *sc.Hours
		m[name] = func() RangeTime {
			now := time.Now().UTC()
			start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			end := start.Add(time.Duration(hours) * time.Hour)
			return RangeTime{
				Start:      FormatTimeDataToRFC3339(start),
				End:        FormatTimeDataToRFC3339(end),
				Resolution: 1,
			}
		}
	}
	return m
}

// FormatTimeDataToRFC3339 does as the name says.
// BUG-002: on parse error, return the original t unchanged rather than the
// zero time, so the function never silently produces a zero value.
func FormatTimeDataToRFC3339(t time.Time) time.Time {
	formattedTime := t.Format(time.RFC3339)
	parsed, err := time.Parse(time.RFC3339, formattedTime)
	if err != nil {
		return t
	}
	return parsed
}
