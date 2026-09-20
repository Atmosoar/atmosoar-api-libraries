package chart

import (
	"errors"
	"fmt"
)

// Status is the reserved state scale. It is deliberately not part of the
// categorical palette: a colour that means "bad" must never also mean "series 4".
type Status string

// The four status steps, worst last.
const (
	StatusGood     Status = "good"
	StatusWarning  Status = "warning"
	StatusSerious  Status = "serious"
	StatusCritical Status = "critical"
	// StatusNone marks a value that could not be classified — no threshold
	// table for the parameter, or nothing reported.
	StatusNone Status = "none"
)

func (s Status) validate() error {
	switch s {
	case StatusGood, StatusWarning, StatusSerious, StatusCritical, StatusNone:
		return nil
	case "":
		return errors.New("status is required")
	default:
		return fmt.Errorf("unknown status %q", s)
	}
}

// Severity orders the statuses so callers can take the worst of several without
// re-deriving the ranking. StatusNone sorts below good: unknown is not bad.
func (s Status) Severity() int {
	switch s {
	case StatusNone:
		return 0
	case StatusGood:
		return 1
	case StatusWarning:
		return 2
	case StatusSerious:
		return 3
	case StatusCritical:
		return 4
	default:
		return 0
	}
}

// The categorical palette: eight hues in a fixed order, per mode.
//
// The order is the colourblind-safety mechanism, not a preference. This set
// clears, in both modes, the lightness band, the chroma floor, adjacent-pair
// CVD separation (ΔE ≥ 8, protanopia and deuteranopia at severity 1.0) and the
// normal-vision floor (ΔE ≥ 15). Do not reorder or restep a slot without
// re-running the platform palette validator over the whole set: the gates are
// properties of the sequence, not of one colour.
//
// Slots are assigned in order of appearance and never cycled — past eight
// series Validate errors rather than letting two identities share a hue.
var (
	seriesLight = [maxSeries]string{
		"#2a78d6", // 1 blue
		"#eb6834", // 2 orange
		"#1baf7a", // 3 aqua
		"#eda100", // 4 yellow
		"#e87ba4", // 5 magenta
		"#008300", // 6 green
		"#4a3aa7", // 7 violet
		"#e34948", // 8 red
	}
	seriesDark = [maxSeries]string{
		"#3987e5", // 1 blue
		"#d95926", // 2 orange
		"#199e70", // 3 aqua
		"#c98500", // 4 yellow
		"#d55181", // 5 magenta
		"#008300", // 6 green
		"#9085e9", // 7 violet
		"#e66767", // 8 red
	}

	// The status steps are mode-invariant: all four clear 3:1 on the dark
	// surface, and on the light surface warning and serious sit below 3:1 by
	// design — the label that always accompanies them is the mitigation.
	statusSteps = map[Status]string{
		StatusGood:     "#0ca30c",
		StatusWarning:  "#fab219",
		StatusSerious:  "#ec835a",
		StatusCritical: "#d03b3b",
		StatusNone:     "#898781",
	}

	// The sequential ramp, light → dark, for magnitude encoding (a windrose's
	// speed bands). One hue: never a rainbow.
	sequentialLight = []string{"#cde2fb", "#9ec5f4", "#6da7ec", "#3987e5", "#256abf", "#184f95", "#0d366b"}
	sequentialDark  = []string{"#0d366b", "#184f95", "#256abf", "#3987e5", "#6da7ec", "#9ec5f4", "#cde2fb"}
)

// ResolvedPalette is every colour a renderer or a browser needs, already picked
// for one mode. It travels inside the marshalled Spec so a frontend drawing the
// same data with recharts or Chart.js needs no hardcoded hexes.
type ResolvedPalette struct {
	Mode Mode `json:"mode"`
	// Surface is the chart's own background.
	Surface string `json:"surface"`
	// Page is the plane the chart sits on, one step off the surface.
	Page string `json:"page"`
	// InkPrimary, InkSecondary and InkMuted are the only colours text ever
	// wears. A label never takes its series' hue: identity comes from the
	// swatch beside it.
	InkPrimary   string `json:"ink_primary"`
	InkSecondary string `json:"ink_secondary"`
	InkMuted     string `json:"ink_muted"`
	// Gridline is a hairline one step off the surface; Baseline is the axis.
	Gridline string `json:"gridline"`
	Baseline string `json:"baseline"`
	// Series holds the eight categorical slots in fixed order.
	Series []string `json:"series"`
	// Status holds the reserved state scale.
	Status map[Status]string `json:"status"`
	// Sequential is the one-hue magnitude ramp, from "near zero" to "most".
	Sequential []string `json:"sequential"`
}

// PaletteFor returns the resolved tokens for a mode. An unknown or empty mode
// resolves light.
func PaletteFor(mode Mode) ResolvedPalette {
	p := ResolvedPalette{
		Mode:       ModeLight,
		Surface:    "#fcfcfb",
		Page:       "#f9f9f7",
		InkPrimary: "#0b0b0b", InkSecondary: "#52514e", InkMuted: "#898781",
		Gridline: "#e1e0d9", Baseline: "#c3c2b7",
		Series:     seriesLight[:],
		Sequential: sequentialLight,
	}
	if mode == ModeDark {
		p = ResolvedPalette{
			Mode:       ModeDark,
			Surface:    "#1a1a19",
			Page:       "#0d0d0d",
			InkPrimary: "#ffffff", InkSecondary: "#c3c2b7", InkMuted: "#898781",
			Gridline: "#2c2c2a", Baseline: "#383835",
			Series:     seriesDark[:],
			Sequential: sequentialDark,
		}
	}
	p.Status = make(map[Status]string, len(statusSteps))
	for k, v := range statusSteps {
		p.Status[k] = v
	}
	return p
}

// SeriesColor returns the hex for a 1-based categorical slot. Slot 0 or out of
// range falls back to slot 1 rather than inventing a hue.
func (p *ResolvedPalette) SeriesColor(slot int) string {
	if slot < 1 || slot > len(p.Series) {
		return p.Series[0]
	}
	return p.Series[slot-1]
}

// StatusColor returns the hex for a status step.
func (p *ResolvedPalette) StatusColor(s Status) string {
	if hex, ok := p.Status[s]; ok {
		return hex
	}
	return p.Status[StatusNone]
}

// SequentialColor returns the ramp step for a 0..1 position, clamped.
func (p *ResolvedPalette) SequentialColor(t float64) string {
	n := len(p.Sequential)
	switch {
	case t <= 0:
		return p.Sequential[0]
	case t >= 1:
		return p.Sequential[n-1]
	default:
		return p.Sequential[int(t*float64(n-1)+0.5)]
	}
}
