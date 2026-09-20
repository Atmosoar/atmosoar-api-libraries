package chart

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// linearScale maps a data domain onto a pixel range. r0 may be greater than r1,
// which is the normal case for a y-axis: data grows upward, pixels grow downward.
type linearScale struct {
	d0, d1 float64
	r0, r1 float64
}

func (s linearScale) at(v float64) float64 {
	if s.d1 == s.d0 {
		return (s.r0 + s.r1) / 2
	}
	return s.r0 + (v-s.d0)/(s.d1-s.d0)*(s.r1-s.r0)
}

// timeScale maps an instant onto a pixel range.
type timeScale struct {
	t0, t1 time.Time
	r0, r1 float64
}

func (s timeScale) at(t time.Time) float64 {
	span := s.t1.Sub(s.t0)
	if span <= 0 {
		return (s.r0 + s.r1) / 2
	}
	return s.r0 + float64(t.Sub(s.t0))/float64(span)*(s.r1-s.r0)
}

// targetTicks is the tick count both axes aim at. Few enough to stay quiet,
// enough to carry the values that are not directly labelled.
const targetTicks = 6

// niceDomain widens [lo, hi] to round numbers and returns the domain plus the
// step between ticks. A flat series (lo == hi) gets a symmetric window around
// the value rather than a zero-height plot.
func niceDomain(lo, hi float64, includeZero bool) (nlo, nhi, step float64) {
	if includeZero {
		lo = math.Min(lo, 0)
		hi = math.Max(hi, 0)
	}
	if hi < lo {
		lo, hi = hi, lo
	}
	if hi == lo {
		pad := math.Max(math.Abs(hi)*0.1, 1)
		lo, hi = lo-pad, hi+pad
	}

	step = niceStep((hi - lo) / targetTicks)
	nlo = math.Floor(lo/step) * step
	nhi = math.Ceil(hi/step) * step
	// Floating-point floor/ceil can leave the domain a hair inside the data.
	if nlo > lo {
		nlo -= step
	}
	if nhi < hi {
		nhi += step
	}
	return nlo, nhi, step
}

// niceStep snaps a raw interval to the next 1-2-5 multiple of a power of ten,
// so ticks read 0 / 5 / 10 rather than 0 / 4.7 / 9.4.
func niceStep(raw float64) float64 {
	if raw <= 0 || math.IsInf(raw, 0) || math.IsNaN(raw) {
		return 1
	}
	exp := math.Floor(math.Log10(raw))
	pow := math.Pow(10, exp)
	switch f := raw / pow; {
	case f <= 1:
		return pow
	case f <= 2:
		return 2 * pow
	case f <= 5:
		return 5 * pow
	default:
		return 10 * pow
	}
}

// numericTicks lays clean ticks across a domain.
func numericTicks(lo, hi, step float64, decimals int) []Tick {
	if step <= 0 {
		return nil
	}
	out := make([]Tick, 0, targetTicks+2)
	// Accumulating by multiplication rather than repeated addition keeps the
	// last tick from drifting off the domain edge.
	for i := 0; ; i++ {
		v := lo + float64(i)*step
		if v > hi+step/1e6 {
			break
		}
		out = append(out, Tick{Value: v, Label: formatTickValue(v, step, decimals)})
		if len(out) > 64 {
			break
		}
	}
	return out
}

// formatTickValue rounds a tick to the precision the step justifies — a step of
// 5 never needs decimals — and groups thousands.
func formatTickValue(v, step float64, decimals int) string {
	places := decimals
	if step >= 1 {
		places = 0
	} else if p := int(math.Ceil(-math.Log10(step))); p < places || places == 0 {
		places = p
	}
	// -0 reads as an error to anybody looking at an axis.
	if v == 0 {
		v = 0
	}
	return groupThousands(strconv.FormatFloat(v, 'f', places, 64))
}

func groupThousands(s string) string {
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i:]
	}
	if len(intPart) > 4 {
		var b strings.Builder
		for i, r := range intPart {
			if i > 0 && (len(intPart)-i)%3 == 0 {
				b.WriteByte(',')
			}
			b.WriteRune(r)
		}
		intPart = b.String()
	}
	out := intPart + frac
	if neg {
		return "-" + out
	}
	return out
}

// timeTickSteps are the intervals a time axis is allowed to tick on. Anything
// else (a 7-minute tick) reads as an accident.
var timeTickSteps = []time.Duration{
	time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute,
	time.Hour, 3 * time.Hour, 6 * time.Hour, 12 * time.Hour,
	24 * time.Hour, 48 * time.Hour, 7 * 24 * time.Hour, 14 * 24 * time.Hour,
	30 * 24 * time.Hour,
}

// timeTicks lays ticks on whole units of a sensible interval, in the location
// of t0 — so "00:00" means local midnight to whoever set the location, and a
// UTC series ticks on UTC midnight.
func timeTicks(t0, t1 time.Time) []Tick {
	span := t1.Sub(t0)
	if span <= 0 {
		return []Tick{{Value: float64(t0.UnixMilli()), Label: formatTickTime(t0, 0), Time: &t0}}
	}

	step := timeTickSteps[len(timeTickSteps)-1]
	for _, cand := range timeTickSteps {
		if span/cand <= targetTicks {
			step = cand
			break
		}
	}

	out := make([]Tick, 0, targetTicks+2)
	for t := t0.Truncate(step); !t.After(t1); t = t.Add(step) {
		if t.Before(t0) {
			continue
		}
		at := t
		out = append(out, Tick{Value: float64(at.UnixMilli()), Label: formatTickTime(at, span), Time: &at})
		if len(out) > 64 {
			break
		}
	}
	return out
}

// Span thresholds for choosing a tick label's resolution.
const (
	twoDays   = 48 * time.Hour
	sixtyDays = 60 * 24 * time.Hour
)

// formatTickTime labels a tick at the resolution the span justifies: clock time
// inside two days, weekday and date inside two months, month and year beyond.
func formatTickTime(t time.Time, span time.Duration) string {
	switch {
	case span >= sixtyDays:
		return t.Format("Jan 2006")
	case span >= twoDays:
		return t.Format("Mon 2 Jan")
	case t.Hour() == 0 && t.Minute() == 0:
		// Midnight carries the date: a run of "00:00" labels with no day is
		// the commonest way a multi-day axis becomes unreadable.
		return t.Format("2 Jan")
	default:
		return t.Format("15:04")
	}
}

// seriesExtent is the finite value range across a panel's series, including the
// bounds of any band marks.
func seriesExtent(p *Panel) (lo, hi float64, ok bool) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for i := range p.Series {
		for j := range p.Series[i].Points {
			pt := &p.Series[i].Points[j]
			for _, v := range []*float64{pt.Value, pt.Low, pt.High} {
				if v == nil {
					continue
				}
				lo, hi = math.Min(lo, *v), math.Max(hi, *v)
				ok = true
			}
		}
	}
	return lo, hi, ok
}

// timeExtent is the instant range across every series of every panel, so a
// meteogram's panels share one x domain.
func timeExtent(s *Spec) (t0, t1 time.Time, ok bool) {
	for i := range s.Panels {
		for j := range s.Panels[i].Series {
			pts := s.Panels[i].Series[j].Points
			if len(pts) == 0 {
				continue
			}
			first, last := pts[0].Time, pts[len(pts)-1].Time
			if !ok {
				t0, t1, ok = first, last, true
				continue
			}
			if first.Before(t0) {
				t0 = first
			}
			if last.After(t1) {
				t1 = last
			}
		}
	}
	return t0, t1, ok
}
