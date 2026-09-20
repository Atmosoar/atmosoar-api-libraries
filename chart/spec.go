package chart

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// Kind selects the chart form. The form follows the job the data does: a
// magnitude over time is a timeseries, several parameters over the same time
// window are a meteogram, and a direction-plus-speed distribution is a windrose.
type Kind string

// The supported chart forms.
const (
	KindTimeSeries Kind = "timeseries"
	KindMeteogram  Kind = "meteogram"
	KindWindrose   Kind = "windrose"
)

// Mode is the colour mode. Both modes are selected: the dark steps are their own
// steps from the same ramps, never an inversion of the light ones.
type Mode string

// The supported colour modes.
const (
	ModeLight Mode = "light"
	ModeDark  Mode = "dark"
)

// Mark is how one series is drawn.
type Mark string

// The supported marks.
const (
	// MarkLine is a 2px line with round joins; the default.
	MarkLine Mark = "line"
	// MarkStep holds each value until the next sample. Use it for values that
	// are constant over an interval (accumulations, categorical states).
	MarkStep Mark = "step"
	// MarkBand fills between Point.Low and Point.High, for spreads and
	// ensemble envelopes. Drawn as a 10% wash of the series hue.
	MarkBand Mark = "band"
	// MarkBar grows every value from the panel baseline, for precipitation.
	MarkBar Mark = "bar"
)

// Point is one sample of one series.
//
// Value is a pointer because a missing reading is not a zero: calm wind and "no
// wind reported" are different facts, and the renderers draw the second as a gap.
type Point struct {
	Time  time.Time `json:"t"`
	Value *float64  `json:"v"`
	// Low and High bound a MarkBand sample.
	Low  *float64 `json:"lo,omitempty"`
	High *float64 `json:"hi,omitempty"`
	// Direction is the direction the wind blows from, in degrees true. Only a
	// windrose reads it.
	Direction *float64 `json:"dir,omitempty"`
}

// Series is one line, band or bar of one parameter.
type Series struct {
	// Name is what the legend and tooltips show.
	Name string `json:"name"`
	// Parameter is a key into the weather-parameter registry. Setting it fills
	// the unit, the value format and the threshold bands automatically.
	Parameter string `json:"parameter,omitempty"`
	Mark      Mark   `json:"mark,omitempty"`
	Unit      string `json:"unit,omitempty"`
	// Slot pins the series to a categorical palette slot (1..8). Zero assigns
	// slots in order of appearance, which is what keeps a colour bound to an
	// entity rather than to its rank.
	Slot int `json:"slot,omitempty"`
	// Color is resolved from Slot and the mode by Resolved; callers do not set it.
	Color  string  `json:"color,omitempty"`
	Points []Point `json:"points"`
}

// Tick is one resolved axis tick: where it sits in data space and what it reads.
type Tick struct {
	Value float64 `json:"value"`
	Label string  `json:"label"`
	// Time carries the instant for a time axis, so a browser can relabel in the
	// viewer's locale instead of parsing Label back.
	Time *time.Time `json:"time,omitempty"`
}

// Axis is one scale. There is deliberately no second y-axis: two parameters of
// different scale become two panels, never two scales on one plot.
type Axis struct {
	Label string `json:"label,omitempty"`
	Unit  string `json:"unit,omitempty"`
	// Min and Max pin the domain. Left nil, the domain is the data extent
	// widened to clean numbers.
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
	// Ticks is filled by Resolved.
	Ticks []Tick `json:"ticks,omitempty"`
	// Zero forces the domain to include zero. Accumulations want it; a
	// temperature trace does not.
	Zero bool `json:"zero,omitempty"`
}

// Band shades a value range behind the marks — the "caution above 20 m/s" stripe.
// It wears a status colour, never a categorical slot, so a threshold can never be
// mistaken for a series.
type Band struct {
	// From and To bound the band in data space. A nil bound is open-ended.
	From   *float64 `json:"from,omitempty"`
	To     *float64 `json:"to,omitempty"`
	Status Status   `json:"status"`
	// Label names the band. A status band is always labelled: the colour alone
	// never carries the meaning.
	Label string `json:"label"`
	// Color is resolved from Status by Resolved.
	Color string `json:"color,omitempty"`
}

// Panel is one plot area with one y-axis. A timeseries has one; a meteogram
// stacks several over a shared time axis.
type Panel struct {
	Title  string   `json:"title,omitempty"`
	Series []Series `json:"series"`
	Y      Axis     `json:"y"`
	Bands  []Band   `json:"bands,omitempty"`
	// ShowThresholds shades the parameter's registry threshold bands behind the
	// marks. Opt-in: a chart of a parameter that has bands does not
	// automatically want them, and shading every panel spends the reader's
	// attention on the background.
	ShowThresholds bool `json:"show_thresholds,omitempty"`
	// Weight is this panel's share of the plot height, relative to its
	// siblings. Zero means one share.
	Weight float64 `json:"weight,omitempty"`
}

// Spec is the whole chart, and the contract between the Go renderers and any
// browser that draws the same data: resolve it once and every consumer gets the
// same colours, bands, ticks and labels.
type Spec struct {
	Kind     Kind    `json:"kind"`
	Title    string  `json:"title,omitempty"`
	Subtitle string  `json:"subtitle,omitempty"`
	Mode     Mode    `json:"mode,omitempty"`
	Width    int     `json:"width,omitempty"`
	Height   int     `json:"height,omitempty"`
	Panels   []Panel `json:"panels"`
	// X is the shared horizontal axis. Every kind but the windrose uses it, and
	// its domain is time.
	X Axis `json:"x"`
	// Windrose configures the polar form; ignored by the other kinds.
	Windrose *WindroseOptions `json:"windrose,omitempty"`
	// Source is the attribution line in the footer, e.g. "Atmosoar MMA · ICON-D2".
	Source string `json:"source,omitempty"`
	// Generated timestamps the render; drawn in the footer when set.
	Generated *time.Time `json:"generated,omitempty"`
	// Palette is filled by Resolved so a browser needs no hardcoded hexes.
	Palette *ResolvedPalette `json:"palette,omitempty"`
}

// Default chart geometry, in pixels. 800x600 matches the existing MMA PNG
// output so a switch of formatter does not resize anybody's embed.
const (
	DefaultWidth  = 800
	DefaultHeight = 600

	minWidth  = 240
	minHeight = 160
	maxWidth  = 4096
	maxHeight = 4096
)

// maxSeries is the categorical palette's length. Past it a chart would have to
// reuse a hue, which silently merges two identities, so Validate errors instead.
const maxSeries = 8

// ErrEmptySpec is returned for a spec with nothing to draw.
var ErrEmptySpec = errors.New("chart: spec has no panels")

// Validate reports the first structural problem with the spec. It enforces the
// rules that are easy to break and expensive to notice: one unit per panel, no
// recycled colours, sorted samples, finite values.
func (s *Spec) Validate() error {
	switch s.Kind {
	case KindTimeSeries, KindMeteogram, KindWindrose:
	case "":
		return errors.New("chart: kind is required")
	default:
		return fmt.Errorf("chart: unknown kind %q", s.Kind)
	}

	switch s.Mode {
	case ModeLight, ModeDark, "":
	default:
		return fmt.Errorf("chart: unknown mode %q", s.Mode)
	}

	if len(s.Panels) == 0 {
		return ErrEmptySpec
	}
	if s.Kind != KindMeteogram && len(s.Panels) > 1 {
		return fmt.Errorf("chart: kind %q takes one panel, got %d", s.Kind, len(s.Panels))
	}
	if err := s.validateSize(); err != nil {
		return err
	}

	total := 0
	for i := range s.Panels {
		p := &s.Panels[i]
		if len(p.Series) == 0 {
			return fmt.Errorf("chart: panel %d has no series", i)
		}
		total += len(p.Series)
		if err := validatePanel(i, p); err != nil {
			return err
		}
	}
	if total > maxSeries {
		return fmt.Errorf("chart: %d series exceeds the %d categorical slots; fold to \"other\" or facet",
			total, maxSeries)
	}
	if s.Kind == KindWindrose {
		return validateWindroseSpec(s)
	}
	return nil
}

func (s *Spec) validateSize() error {
	if s.Width != 0 && (s.Width < minWidth || s.Width > maxWidth) {
		return fmt.Errorf("chart: width %d outside %d..%d", s.Width, minWidth, maxWidth)
	}
	if s.Height != 0 && (s.Height < minHeight || s.Height > maxHeight) {
		return fmt.Errorf("chart: height %d outside %d..%d", s.Height, minHeight, maxHeight)
	}
	return nil
}

// validatePanel enforces the one-axis rule and the per-series invariants.
func validatePanel(idx int, p *Panel) error {
	unit := ""
	for j := range p.Series {
		ser := &p.Series[j]
		if ser.Name == "" {
			return fmt.Errorf("chart: panel %d series %d has no name", idx, j)
		}
		if ser.Slot < 0 || ser.Slot > maxSeries {
			return fmt.Errorf("chart: panel %d series %q slot %d outside 1..%d", idx, ser.Name, ser.Slot, maxSeries)
		}
		switch ser.Mark {
		case MarkLine, MarkStep, MarkBand, MarkBar, "":
		default:
			return fmt.Errorf("chart: panel %d series %q unknown mark %q", idx, ser.Name, ser.Mark)
		}
		u := ser.EffectiveUnit()
		if j == 0 {
			unit = u
		} else if u != unit {
			// Two units in one plot would need two y-scales, which is the
			// single worst chart mistake. Split the panel instead.
			return fmt.Errorf("chart: panel %d mixes units %q and %q; use one panel per unit", idx, unit, u)
		}
		if err := validatePoints(idx, ser); err != nil {
			return err
		}
	}
	for j := range p.Bands {
		if p.Bands[j].Label == "" {
			return fmt.Errorf("chart: panel %d band %d has no label; a status colour never stands alone", idx, j)
		}
		if err := p.Bands[j].Status.validate(); err != nil {
			return fmt.Errorf("chart: panel %d band %d: %w", idx, j, err)
		}
	}
	return nil
}

func validatePoints(idx int, ser *Series) error {
	var prev time.Time
	for k := range ser.Points {
		pt := &ser.Points[k]
		if pt.Time.IsZero() {
			return fmt.Errorf("chart: panel %d series %q point %d has no time", idx, ser.Name, k)
		}
		if k > 0 && pt.Time.Before(prev) {
			return fmt.Errorf("chart: panel %d series %q points are not sorted by time", idx, ser.Name)
		}
		prev = pt.Time
		for _, v := range []*float64{pt.Value, pt.Low, pt.High, pt.Direction} {
			if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0)) {
				return fmt.Errorf("chart: panel %d series %q point %d is not finite", idx, ser.Name, k)
			}
		}
		if ser.Mark == MarkBand && pt.Low != nil && pt.High != nil && *pt.Low > *pt.High {
			return fmt.Errorf("chart: panel %d series %q point %d has lo > hi", idx, ser.Name, k)
		}
	}
	return nil
}

// EffectiveUnit is the series' unit, falling back to the registry entry for its
// parameter.
func (s *Series) EffectiveUnit() string {
	if s.Unit != "" {
		return s.Unit
	}
	if p, ok := Parameter(s.Parameter); ok {
		return p.Unit
	}
	return ""
}

// EffectiveMark is the series' mark, falling back to the registry's preferred
// mark for its parameter and then to a line.
func (s *Series) EffectiveMark() Mark {
	if s.Mark != "" {
		return s.Mark
	}
	if p, ok := Parameter(s.Parameter); ok && p.Mark != "" {
		return p.Mark
	}
	return MarkLine
}

// Float returns a pointer to v, for building Points inline.
func Float(v float64) *float64 { return &v }
