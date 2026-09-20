// Package chart is the platform's weather-data visualization layer: one
// definition of a chart, rendered three ways.
//
// A [Spec] says what to plot. [Spec.Resolved] derives everything else — palette
// slots, units, marks, domains, ticks and threshold bands — and the three
// encoders emit it as JSON (for a browser to draw), as SVG, or as PNG for the
// image endpoints the platform already serves. SVG and PNG are transcribed from
// one shared draw list, so the same spec is the same picture in either format.
//
// Colours are not a per-service choice. The categorical slots, the reserved
// status scale and the sequential ramp live in one validated palette, and the
// per-parameter threshold bands live in one registry, so a wind speed that is
// amber on a forecast map is amber on an observation chart too.
//
// The smallest useful call:
//
//	spec := chart.TimeSeries("Brenner axis",
//	    chart.SeriesOf("Δp", "pressure_difference", points))
//	png, err := chart.PNG(spec)
package chart

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Content types for the three encodings.
const (
	MIMEPNG  = "image/png"
	MIMESVG  = "image/svg+xml"
	MIMEJSON = "application/json; charset=utf-8"
)

// Format is one of the three encodings a service can serve.
type Format string

// The supported encodings. The names double as query-parameter values, so
// every service spells them the same way.
const (
	FormatPNG  Format = "chart_png"
	FormatSVG  Format = "chart_svg"
	FormatSpec Format = "chartspec"
)

// ParseFormat recognises a format name, with or without the chart_ prefix, in
// any case. The second return is false for anything else.
func ParseFormat(s string) (Format, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "chart_png", "chartpng", "png":
		return FormatPNG, true
	case "chart_svg", "chartsvg", "svg":
		return FormatSVG, true
	case "chartspec", "chart_spec", "spec":
		return FormatSpec, true
	default:
		return "", false
	}
}

// ParseMode recognises a colour-mode name, in any case, so every service reads
// the same `theme` query parameter. The second return is false for anything
// else, which callers treat as "leave the spec's mode alone".
func ParseMode(s string) (Mode, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "light":
		return ModeLight, true
	case "dark":
		return ModeDark, true
	default:
		return "", false
	}
}

// Encode renders a spec in one format and reports the content type to serve it
// with. It is the whole integration surface a service needs.
func Encode(s Spec, f Format) (data []byte, contentType string, err error) {
	switch f {
	case FormatPNG:
		data, err = PNG(s)
		return data, MIMEPNG, err
	case FormatSVG:
		data, err = SVG(s)
		return data, MIMESVG, err
	case FormatSpec:
		data, err = JSON(s)
		return data, MIMEJSON, err
	default:
		return nil, "", fmt.Errorf("chart: unknown format %q", f)
	}
}

// PNG renders a spec as a PNG.
func PNG(s Spec) ([]byte, error) {
	d, err := Render(s)
	if err != nil {
		return nil, err
	}
	return renderPNG(d)
}

// SVG renders a spec as SVG.
func SVG(s Spec) ([]byte, error) {
	d, err := Render(s)
	if err != nil {
		return nil, err
	}
	return renderSVG(d), nil
}

// JSON marshals the resolved spec — the contract a browser draws from.
func JSON(s Spec) ([]byte, error) {
	resolved, err := s.Resolved()
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(resolved)
	if err != nil {
		return nil, fmt.Errorf("chart: marshal spec: %w", err)
	}
	return out, nil
}

// Render resolves a spec and lays it out. Callers with a renderer of their own
// consume the returned draw list directly.
func Render(s Spec) (*Drawing, error) {
	resolved, err := s.Resolved()
	if err != nil {
		return nil, err
	}
	return Build(&resolved), nil
}

// SeriesOf is the common series constructor: a name, a registry parameter and
// its samples. Unit and mark come from the registry.
func SeriesOf(name, parameter string, points []Point) Series {
	if name == "" {
		name = ParameterLabel(parameter)
	}
	return Series{Name: name, Parameter: parameter, Points: points}
}

// TimeSeries is a one-panel chart. Every series must share a unit — two units
// would need two scales, and the second scale is the worst chart mistake there
// is, so Validate rejects it.
func TimeSeries(title string, series ...Series) Spec {
	panel := Panel{Series: series}
	if len(series) == 1 {
		panel.Title = ParameterLabel(series[0].Parameter)
	}
	return Spec{Kind: KindTimeSeries, Title: title, Panels: []Panel{panel}}
}

// Meteogram stacks one panel per parameter over a shared time axis. This is
// what a caller reaches for instead of a second y-axis.
func Meteogram(title string, panels ...Panel) Spec {
	return Spec{Kind: KindMeteogram, Title: title, Panels: panels}
}

// PanelOf is one meteogram panel, titled from its first series' parameter.
func PanelOf(series ...Series) Panel {
	p := Panel{Series: series}
	if len(series) == 1 {
		p.Title = ParameterLabel(series[0].Parameter)
	}
	return p
}

// Windrose is a direction-and-speed distribution. The samples carry the speed
// in Value and the direction the wind blows from in Direction.
func Windrose(title string, series Series, opts *WindroseOptions) Spec {
	return Spec{
		Kind:     KindWindrose,
		Title:    title,
		Panels:   []Panel{{Series: []Series{series}}},
		Windrose: opts,
	}
}

// Sample is one reading: an instant plus the values reported at it, keyed by
// parameter name. It is the shape every Atmosoar time-series endpoint already
// has in hand after querying, which is why the builders below take it.
type Sample struct {
	Time   time.Time
	Values map[string]*float64
}

// FromSamples builds a spec out of reading rows.
//
// The parameters are plotted in the order given, one panel each for a
// meteogram, one shared panel for a timeseries. A parameter missing from a row
// becomes a gap in that series, never a zero. Rows arrive in any order; they
// are sorted here, because an unsorted series is rejected by Validate and
// callers should not have to know that.
func FromSamples(kind Kind, title string, samples []Sample, parameters []string) Spec {
	rows := make([]Sample, len(samples))
	copy(rows, samples)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Time.Before(rows[j].Time) })

	if kind == KindWindrose {
		return windroseFromSamples(title, rows, parameters)
	}

	series := make([]Series, 0, len(parameters))
	for _, param := range parameters {
		points := make([]Point, 0, len(rows))
		for i := range rows {
			points = append(points, Point{Time: rows[i].Time, Value: rows[i].Values[param]})
		}
		series = append(series, SeriesOf(ParameterLabel(param), param, points))
	}

	if kind == KindMeteogram {
		panels := make([]Panel, 0, len(series))
		for i := range series {
			panels = append(panels, PanelOf(series[i]))
		}
		return Meteogram(title, panels...)
	}
	return TimeSeries(title, series...)
}

// windroseSpeedParam and windroseDirectionParam are the parameters a rose is
// built from when the caller does not name them.
const (
	windroseSpeedParam     = "wind_speed"
	windroseDirectionParam = "wind_direction"
)

// windroseFromSamples pairs a speed with a direction per row. The parameters
// slice may name them explicitly, speed first.
func windroseFromSamples(title string, rows []Sample, parameters []string) Spec {
	speedParam, dirParam := windroseSpeedParam, windroseDirectionParam
	if len(parameters) > 0 {
		speedParam = parameters[0]
	}
	if len(parameters) > 1 {
		dirParam = parameters[1]
	}

	points := make([]Point, 0, len(rows))
	for i := range rows {
		points = append(points, Point{
			Time:      rows[i].Time,
			Value:     rows[i].Values[speedParam],
			Direction: rows[i].Values[dirParam],
		})
	}
	return Windrose(title, SeriesOf(ParameterLabel(speedParam), speedParam, points), nil)
}
