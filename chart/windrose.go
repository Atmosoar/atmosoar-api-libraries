package chart

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

// WindroseOptions configures the polar form. The resolved fields below the
// divider are filled by Resolved, so a browser can draw the same rose without
// re-binning anything.
type WindroseOptions struct {
	// Sectors is how many compass sectors the circle is cut into. Must divide
	// 360. Default 16, the conventional rose.
	Sectors int `json:"sectors,omitempty"`
	// SpeedBands are the upper edges of the speed classes, ascending, in the
	// series' unit. Everything above the last edge forms the open top band.
	SpeedBands []float64 `json:"speed_bands,omitempty"`
	// CalmBelow is the speed under which a reading is counted as calm rather
	// than binned by direction — a direction reported at 0.1 m/s is noise.
	CalmBelow float64 `json:"calm_below,omitempty"`

	// --- resolved ---

	// Bins is the distribution: one entry per non-empty sector/band pair.
	Bins []RoseBin `json:"bins,omitempty"`
	// BandLabels and BandColors describe the speed classes. The colours are
	// steps of the one-hue sequential ramp, because speed is a magnitude —
	// never the categorical slots, which encode identity.
	BandLabels []string `json:"band_labels,omitempty"`
	BandColors []string `json:"band_colors,omitempty"`
	// Rings are the frequency gridlines, in percent.
	Rings []Tick `json:"rings,omitempty"`
	// CalmFraction is the share of readings counted as calm, 0..1.
	CalmFraction float64 `json:"calm_fraction,omitempty"`
	// Total is how many readings the rose summarises.
	Total int `json:"total,omitempty"`
	// MaxFraction is the fullest sector's share, 0..1 — the outer ring.
	MaxFraction float64 `json:"max_fraction,omitempty"`
}

// RoseBin is one sector/speed-band cell of the distribution.
type RoseBin struct {
	Sector int `json:"sector"`
	Band   int `json:"band"`
	Count  int `json:"count"`
	// Fraction is Count over Total, 0..1.
	Fraction float64 `json:"fraction"`
}

// Windrose defaults.
const (
	defaultSectors   = 16
	defaultCalmBelow = 0.5
	// roseGap is the share of each sector's angular width given up to surface,
	// so neighbouring wedges read as separate without a stroke around them.
	roseGap = 0.12
	// arcStep is the angular resolution wedge arcs are flattened at.
	arcStep = 3.0
)

// defaultSpeedBands are the Beaufort-ish m/s classes the platform's other wind
// products already use for their thresholds (calm, light, moderate, fresh,
// strong, gale).
var defaultSpeedBands = []float64{2, 4, 6, 10, 15}

func validateWindroseSpec(s *Spec) error {
	if len(s.Panels) != 1 || len(s.Panels[0].Series) != 1 {
		return errors.New("chart: a windrose takes exactly one series")
	}
	opts := s.Windrose
	if opts == nil {
		opts = &WindroseOptions{}
	}
	if opts.Sectors != 0 && (opts.Sectors < 4 || opts.Sectors > 72 || 360%opts.Sectors != 0) {
		return fmt.Errorf("chart: windrose sectors %d must divide 360 and sit in 4..72", opts.Sectors)
	}
	for i := 1; i < len(opts.SpeedBands); i++ {
		if opts.SpeedBands[i] <= opts.SpeedBands[i-1] {
			return errors.New("chart: windrose speed bands must ascend")
		}
	}
	if len(opts.SpeedBands) > 0 && opts.SpeedBands[0] <= 0 {
		return errors.New("chart: windrose speed bands must be positive")
	}

	for _, pt := range s.Panels[0].Series[0].Points {
		if pt.Direction != nil && pt.Value != nil {
			return nil
		}
	}
	return errors.New("chart: a windrose needs at least one point with both a direction and a speed")
}

// resolveRose bins the readings and resolves the ramp, labels and rings.
func (s *Spec) resolveRose() {
	opts := WindroseOptions{}
	if s.Windrose != nil {
		opts = *s.Windrose
	}
	if opts.Sectors == 0 {
		opts.Sectors = defaultSectors
	}
	if len(opts.SpeedBands) == 0 {
		opts.SpeedBands = defaultSpeedBands
	}
	if opts.CalmBelow == 0 {
		opts.CalmBelow = defaultCalmBelow
	}

	ser := &s.Panels[0].Series[0]
	nBands := len(opts.SpeedBands) + 1
	counts := make([][]int, opts.Sectors)
	for i := range counts {
		counts[i] = make([]int, nBands)
	}

	total, calm := 0, 0
	sectorWidth := 360.0 / float64(opts.Sectors)
	for i := range ser.Points {
		pt := &ser.Points[i]
		if pt.Direction == nil || pt.Value == nil {
			continue
		}
		total++
		if *pt.Value < opts.CalmBelow {
			calm++
			continue
		}
		// Sectors are centred on their compass bearing, so a wind from 349°
		// lands in the north sector rather than in the one before it.
		deg := math.Mod(math.Mod(*pt.Direction, 360)+360, 360)
		sector := int(math.Floor((deg+sectorWidth/2)/sectorWidth)) % opts.Sectors
		counts[sector][speedBand(opts.SpeedBands, *pt.Value)]++
	}

	opts.Total = total
	if total > 0 {
		opts.CalmFraction = float64(calm) / float64(total)
	}
	opts.Bins = nil
	maxFraction := 0.0
	for sec := range counts {
		sectorTotal := 0
		for band := range counts[sec] {
			if counts[sec][band] == 0 {
				continue
			}
			frac := 0.0
			if total > 0 {
				frac = float64(counts[sec][band]) / float64(total)
			}
			opts.Bins = append(opts.Bins, RoseBin{
				Sector: sec, Band: band, Count: counts[sec][band], Fraction: frac,
			})
			sectorTotal += counts[sec][band]
		}
		if total > 0 {
			maxFraction = math.Max(maxFraction, float64(sectorTotal)/float64(total))
		}
	}
	opts.MaxFraction = maxFraction

	unit := ser.Unit
	opts.BandLabels = speedBandLabels(opts.SpeedBands, unit)
	opts.BandColors = make([]string, nBands)
	for i := range opts.BandColors {
		// Light → dark with speed: the ramp itself carries the magnitude.
		opts.BandColors[i] = s.Palette.SequentialColor(float64(i+1) / float64(nBands))
	}
	opts.Rings = roseRings(maxFraction)
	s.Windrose = &opts
}

// speedBand is the index of the class a speed falls in.
func speedBand(edges []float64, v float64) int {
	for i, e := range edges {
		if v < e {
			return i
		}
	}
	return len(edges)
}

func speedBandLabels(edges []float64, unit string) []string {
	if unit != "" {
		unit = " " + unit
	}
	out := make([]string, 0, len(edges)+1)
	prev := 0.0
	for _, e := range edges {
		out = append(out, trimFloat(prev)+"–"+trimFloat(e)+unit)
		prev = e
	}
	return append(out, "≥ "+trimFloat(prev)+unit)
}

func trimFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// roseRings puts frequency gridlines on clean percentages.
func roseRings(maxFraction float64) []Tick {
	hi := math.Max(maxFraction*100, 1)
	_, nhi, step := niceDomain(0, hi, true)
	out := make([]Tick, 0, 6)
	for v := step; v <= nhi+step/1e6; v += step {
		out = append(out, Tick{Value: v, Label: formatTickValue(v, step, 0) + "%"})
	}
	return out
}

// buildRose lays the polar form out. Wedges stack outward from the centre in
// speed order, which is why the ramp has to be ordered too.
func (d *Drawing) buildRose(s *Spec, l *layout) {
	opts := s.Windrose
	c, maxR := l.roseCentre, l.roseRadius
	outer := 0.0
	if n := len(opts.Rings); n > 0 {
		outer = opts.Rings[n-1].Value / 100
	}
	if outer <= 0 {
		outer = 1
	}
	scale := maxR / outer

	d.roseGrid(s, c, maxR, scale)

	// Stack each sector's bands, innermost first.
	sectorWidth := 360.0 / float64(opts.Sectors)
	cum := make([]float64, opts.Sectors)
	for i := range cum {
		cum[i] = opts.CalmFraction
	}
	for _, bin := range opts.Bins {
		r0 := cum[bin.Sector] * scale
		r1 := (cum[bin.Sector] + bin.Fraction) * scale
		cum[bin.Sector] += bin.Fraction
		centre := float64(bin.Sector) * sectorWidth
		half := sectorWidth / 2 * (1 - roseGap)
		d.polygon(wedge(c, r0, r1, centre-half, centre+half), opts.BandColors[bin.Band], 1, "", 0)
	}

	if opts.CalmFraction > 0 {
		// Calm readings have no direction, so they sit in the middle and every
		// wedge starts outside them. The disc wears chrome colours: it is a
		// reference, not a measurement.
		r := opts.CalmFraction * scale
		d.circle(c.X, c.Y, r, d.Palette.Gridline, hairline, d.Palette.Baseline)
		// The bounding-box corner is always outside the outer ring, so a label
		// there can never land on a wedge or on the S compass point.
		d.text("calm "+strconv.FormatFloat(opts.CalmFraction*100, 'f', 0, 64)+"%",
			c.X-maxR, c.Y-maxR, sizeFooter, false, d.Palette.InkMuted, 0, 0)
	}
}

// roseGrid draws the frequency rings, their labels and the compass points.
func (d *Drawing) roseGrid(s *Spec, c Pt, maxR, scale float64) {
	for _, ring := range s.Windrose.Rings {
		r := ring.Value / 100 * scale
		if r > maxR+0.5 {
			continue
		}
		d.strokedCircle(c.X, c.Y, r, d.Palette.Gridline, hairline)
		// Ring labels ride the north-north-east diagonal, where they cross the
		// fewest wedges.
		d.text(ring.Label, c.X+r*0.36, c.Y-r*0.93, sizeFooter, false, d.Palette.InkMuted, 0.5, 0.5)
	}
	for i, name := range [...]string{"N", "E", "S", "W"} {
		angle := float64(i) * 90
		p := polar(c, maxR+textHeight(sizeAxis)*0.9, angle)
		d.text(name, p.X, p.Y, sizeAxis, true, d.Palette.InkSecondary, 0.5, 0.5)
	}
}

// roseLegend is an ordinal legend: the speed classes in ramp order.
func (d *Drawing) roseLegend(s *Spec, l *layout) {
	const swatch = 10.0
	opts := s.Windrose
	x := l.legendAt.X
	limit := l.legendAt.X + l.legendW
	for i, label := range opts.BandLabels {
		w := swatch + 5 + measureText(label, sizeLegend, false)
		if x+w > limit {
			return
		}
		cy := l.legendAt.Y - textHeight(sizeLegend)/2
		d.rect(x, cy-swatch/2, swatch, swatch, opts.BandColors[i], 1)
		d.text(label, x+swatch+5, l.legendAt.Y, sizeLegend, false, d.Palette.InkSecondary, 0, 1)
		x += w + 12
	}
}

// polar converts a compass bearing and radius to device space: north is up and
// bearings run clockwise, as a compass does.
func polar(c Pt, r, bearingDeg float64) Pt {
	rad := bearingDeg * math.Pi / 180
	return Pt{X: c.X + r*math.Sin(rad), Y: c.Y - r*math.Cos(rad)}
}

// wedge flattens an annular sector into a polygon, so the renderers need no arc
// primitive.
func wedge(c Pt, r0, r1, fromDeg, toDeg float64) []Pt {
	steps := int(math.Max(2, math.Ceil((toDeg-fromDeg)/arcStep)))
	pts := make([]Pt, 0, (steps+1)*2)
	for i := 0; i <= steps; i++ {
		a := fromDeg + (toDeg-fromDeg)*float64(i)/float64(steps)
		pts = append(pts, polar(c, r1, a))
	}
	for i := steps; i >= 0; i-- {
		a := fromDeg + (toDeg-fromDeg)*float64(i)/float64(steps)
		pts = append(pts, polar(c, r0, a))
	}
	return pts
}
