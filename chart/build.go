package chart

import (
	"math"
	"time"
)

// Resolved returns a copy of the spec with every derived value filled in:
// defaults applied, palette slots assigned, units and marks settled, domains
// widened to clean numbers, ticks laid, threshold bands materialised and the
// resolved palette attached.
//
// It is the single derivation step. The Go renderers call it, and services hand
// its JSON to browsers, so a chart drawn by recharts and the same chart drawn as
// a PNG carry identical colours, bands, ticks and labels.
func (s Spec) Resolved() (Spec, error) {
	out := s
	out.applyDefaults()

	if err := out.Validate(); err != nil {
		return Spec{}, err
	}

	palette := PaletteFor(out.Mode)
	out.Palette = &palette

	out.Panels = make([]Panel, len(s.Panels))
	copy(out.Panels, s.Panels)

	slot := 0
	for i := range out.Panels {
		p := &out.Panels[i]
		p.Series = make([]Series, len(s.Panels[i].Series))
		copy(p.Series, s.Panels[i].Series)
		for j := range p.Series {
			ser := &p.Series[j]
			ser.Unit = ser.EffectiveUnit()
			ser.Mark = ser.EffectiveMark()
			if ser.Slot == 0 {
				slot++
				ser.Slot = slot
			}
			ser.Color = palette.SeriesColor(ser.Slot)
		}
		p.resolveBands(&palette)
		p.resolveY()
	}

	if out.Kind == KindWindrose {
		out.resolveRose()
		return out, nil
	}
	out.resolveX()
	return out, nil
}

func (s *Spec) applyDefaults() {
	if s.Mode == "" {
		s.Mode = ModeLight
	}
	if s.Width == 0 {
		s.Width = DefaultWidth
	}
	if s.Height == 0 {
		s.Height = DefaultHeight
	}
}

// resolveBands materialises the registry's threshold bands when the panel asked
// for them, and resolves every band's status to a hex.
func (p *Panel) resolveBands(palette *ResolvedPalette) {
	if p.ShowThresholds && len(p.Bands) == 0 && len(p.Series) > 0 {
		p.Bands = BandsFor(p.Series[0].Parameter)
	} else if len(p.Bands) > 0 {
		bands := make([]Band, len(p.Bands))
		copy(bands, p.Bands)
		p.Bands = bands
	}
	for i := range p.Bands {
		p.Bands[i].Color = palette.StatusColor(p.Bands[i].Status)
	}
}

// resolveY settles the panel's y domain and ticks. The domain widens to clean
// numbers so the ticks read 0 / 5 / 10, and includes zero whenever the
// parameter is an accumulation — a rain total drawn from a floating baseline
// misstates its own magnitude.
func (p *Panel) resolveY() {
	if p.Y.Label == "" && len(p.Series) > 0 {
		p.Y.Label = ParameterLabel(p.Series[0].Parameter)
	}
	if p.Y.Unit == "" && len(p.Series) > 0 {
		p.Y.Unit = p.Series[0].Unit
	}

	zero := p.Y.Zero
	for i := range p.Series {
		if def, ok := Parameter(p.Series[i].Parameter); ok && def.Zero {
			zero = true
		}
	}

	lo, hi, ok := seriesExtent(p)
	if !ok {
		lo, hi = 0, 1
	}
	// The domain is the data's. Threshold bands clip to it rather than
	// stretching it: widening a 0..17 °C trace to 40 °C so an unreached
	// caution band can be shown spends half the panel on empty air and
	// flattens the only thing the reader came for. A band the data does not
	// approach is simply not drawn, which is the honest reading — and a caller
	// who wants the headroom anyway sets Y.Max.
	nlo, nhi, step := niceDomain(lo, hi, zero)
	pinned := p.Y.Min != nil || p.Y.Max != nil
	if p.Y.Min != nil {
		nlo = *p.Y.Min
	}
	if p.Y.Max != nil {
		nhi = *p.Y.Max
	}
	if nhi <= nlo {
		nhi = nlo + 1
	}
	if pinned {
		// The step has to follow the domain that is actually drawn. Derived
		// from the data extent instead, a caller who pins a wide domain — a
		// symmetric ±20 frame around a series that only spans 10 — gets a tick
		// every 2 across the whole axis, which is noise.
		step = niceStep((nhi - nlo) / targetTicks)
	}
	p.Y.Min, p.Y.Max = Float(nlo), Float(nhi)
	p.Y.Zero = zero
	p.Y.Ticks = numericTicks(nlo, nhi, step, p.decimals())
}

// decimals is the precision of the panel's dominant parameter.
func (p *Panel) decimals() int {
	if len(p.Series) == 0 {
		return 1
	}
	if def, ok := Parameter(p.Series[0].Parameter); ok {
		return def.Decimals
	}
	return 1
}

// resolveX settles the shared time axis. Min and Max travel as Unix
// milliseconds so the marshalled spec needs no separate time encoding.
func (s *Spec) resolveX() {
	t0, t1, ok := timeExtent(s)
	if !ok {
		t0, t1 = time.Now().UTC(), time.Now().UTC().Add(time.Hour)
	}
	if s.X.Min != nil {
		t0 = time.UnixMilli(int64(*s.X.Min)).In(t0.Location())
	}
	if s.X.Max != nil {
		t1 = time.UnixMilli(int64(*s.X.Max)).In(t1.Location())
	}
	if !t1.After(t0) {
		t1 = t0.Add(time.Hour)
	}
	s.X.Min = Float(float64(t0.UnixMilli()))
	s.X.Max = Float(float64(t1.UnixMilli()))
	s.X.Ticks = timeTicks(t0, t1)
}

// xTimeDomain reads back the resolved time domain.
func (s *Spec) xTimeDomain() (t0, t1 time.Time) {
	if s.X.Min == nil || s.X.Max == nil {
		now := time.Now().UTC()
		return now, now.Add(time.Hour)
	}
	return time.UnixMilli(int64(*s.X.Min)).UTC(), time.UnixMilli(int64(*s.X.Max)).UTC()
}

// Build lays a resolved spec out into device-space draw operations. Both
// renderers consume the result unchanged.
func Build(s *Spec) *Drawing {
	d := &Drawing{Width: s.Width, Height: s.Height, Palette: *s.Palette}
	l := computeLayout(s)

	d.rect(0, 0, l.width, l.height, d.Palette.Surface, 1)
	d.chrome(s, &l)

	if s.Kind == KindWindrose {
		d.buildRose(s, &l)
		return d
	}

	t0, t1 := s.xTimeDomain()
	for i := range s.Panels {
		p := &s.Panels[i]
		plot := l.plots[i]
		x := timeScale{t0: t0, t1: t1, r0: plot.x, r1: plot.right()}
		y := linearScale{d0: *p.Y.Min, d1: *p.Y.Max, r0: plot.bottom(), r1: plot.y}

		d.panelBackground(p, plot, y)
		d.panelGrid(s, p, plot, x, y)
		if title := panelTitle(p); title != "" {
			d.text(title, l.panelTitleAt[i].X, l.panelTitleAt[i].Y, sizeAxis, true,
				d.Palette.InkSecondary, 0, 1)
		}
		for j := range p.Series {
			d.series(&p.Series[j], plot, x, y, len(p.Series))
		}
	}
	d.xAxis(s, &l)
	return d
}

// chrome draws the title block, the legend and the footer.
func (d *Drawing) chrome(s *Spec, l *layout) {
	if s.Title != "" {
		d.text(truncateToWidth(s.Title, sizeTitle, true, l.width-2*gutter), l.titleAt.X, l.titleAt.Y,
			sizeTitle, true, d.Palette.InkPrimary, 0, 1)
	}
	if s.Subtitle != "" {
		d.text(truncateToWidth(s.Subtitle, sizeSubtitle, false, l.width-2*gutter), l.subtitleAt.X, l.subtitleAt.Y,
			sizeSubtitle, false, d.Palette.InkSecondary, 0, 1)
	}
	switch {
	case s.Kind == KindWindrose:
		d.roseLegend(s, l)
	case legendEntryCount(s) >= 2:
		d.legend(s, l)
	}
	if l.hasFooter {
		d.footer(s, l)
	}
}

// legend is the dependable identity channel: a swatch plus a name, so nothing
// rests on colour-matching alone. Entries run in slot order on one row and stop
// where the row runs out rather than overflowing the canvas.
func (d *Drawing) legend(s *Spec, l *layout) {
	const swatch = 10.0
	x := l.legendAt.X
	limit := l.legendAt.X + l.legendW
	for i := range s.Panels {
		for j := range s.Panels[i].Series {
			ser := &s.Panels[i].Series[j]
			// The unit is on the panel header, not here: repeating it on every
			// entry pads the row out of the canvas for no added meaning.
			name := ser.Name
			w := swatch + 5 + measureText(name, sizeLegend, false)
			if x+w > limit {
				return
			}
			cy := l.legendAt.Y - textHeight(sizeLegend)/2
			d.rect(x, cy-swatch/2, swatch, swatch, ser.Color, 1)
			d.text(name, x+swatch+5, l.legendAt.Y, sizeLegend, false, d.Palette.InkSecondary, 0, 1)
			x += w + 14
		}
	}
}

func (d *Drawing) footer(s *Spec, l *layout) {
	line := s.Source
	if s.Generated != nil {
		stamp := s.Generated.UTC().Format("2006-01-02 15:04 UTC")
		if line != "" {
			line += " · "
		}
		line += stamp
	}
	d.text(truncateToWidth(line, sizeFooter, false, l.width-2*gutter), l.footerAt.X, l.footerAt.Y,
		sizeFooter, false, d.Palette.InkMuted, 0, 1)
}

// panelBackground shades the threshold bands behind the marks, clipped to the
// panel's domain.
func (d *Drawing) panelBackground(p *Panel, plot rect, y linearScale) {
	for i := range p.Bands {
		b := &p.Bands[i]
		from, to := *p.Y.Min, *p.Y.Max
		if b.From != nil {
			from = math.Max(from, *b.From)
		}
		if b.To != nil {
			to = math.Min(to, *b.To)
		}
		if to <= from {
			continue
		}
		top, bottom := y.at(to), y.at(from)
		d.rect(plot.x, top, plot.w, bottom-top, b.Color, thresholdAlpha)
	}
}

// panelGrid draws the recessive grid, the y tick labels and the zero baseline.
func (d *Drawing) panelGrid(s *Spec, p *Panel, plot rect, x timeScale, y linearScale) {
	for _, t := range p.Y.Ticks {
		if t.Value < *p.Y.Min || t.Value > *p.Y.Max {
			continue
		}
		py := math.Round(y.at(t.Value)) + 0.5
		d.polyline([]Pt{{X: plot.x, Y: py}, {X: plot.right(), Y: py}}, d.Palette.Gridline, hairline)
		d.text(t.Label, plot.x-yLabelGap, py, sizeAxis, false, d.Palette.InkMuted, 1, 0.5)
	}
	for _, t := range s.X.Ticks {
		px := math.Round(x.at(time.UnixMilli(int64(t.Value)).UTC())) + 0.5
		if px < plot.x || px > plot.right() {
			continue
		}
		d.polyline([]Pt{{X: px, Y: plot.y}, {X: px, Y: plot.bottom()}}, d.Palette.Gridline, hairline)
	}
	// The baseline is the axis; a domain that straddles zero gets it drawn at
	// zero, where the reader expects the reference.
	base := plot.bottom()
	if *p.Y.Min < 0 && *p.Y.Max > 0 {
		base = y.at(0)
	}
	d.polyline([]Pt{{X: plot.x, Y: base}, {X: plot.right(), Y: base}}, d.Palette.Baseline, hairline)
}

// xAxis labels the shared time axis under the lowest panel.
func (d *Drawing) xAxis(s *Spec, l *layout) {
	if len(l.plots) == 0 {
		return
	}
	last := l.plots[len(l.plots)-1]
	t0, t1 := s.xTimeDomain()
	x := timeScale{t0: t0, t1: t1, r0: last.x, r1: last.right()}
	for _, t := range s.X.Ticks {
		px := x.at(time.UnixMilli(int64(t.Value)).UTC())
		if px < last.x-1 || px > last.right()+1 {
			continue
		}
		d.polyline([]Pt{{X: px, Y: last.bottom()}, {X: px, Y: last.bottom() + tickLen}},
			d.Palette.Baseline, hairline)
		d.text(t.Label, px, l.xAxisLabelY, sizeAxis, false, d.Palette.InkMuted, 0.5, 0.5)
	}
}

// panelTitle names a panel and states its unit, which is why no chart here
// needs a rotated axis label. A panel carrying several series has no title of
// its own — the legend names them — so its header is the unit alone.
func panelTitle(p *Panel) string {
	switch {
	case p.Title != "" && p.Y.Unit != "":
		return p.Title + " (" + p.Y.Unit + ")"
	case p.Title != "":
		return p.Title
	default:
		return p.Y.Unit
	}
}

// series draws one series with its mark.
func (d *Drawing) series(ser *Series, plot rect, x timeScale, y linearScale, seriesInPanel int) {
	switch ser.Mark {
	case MarkBand:
		d.bandMark(ser, x, y)
	case MarkBar:
		d.barMark(ser, plot, x, y)
	case MarkStep:
		d.lineMark(ser, x, y, true)
		d.endMarker(ser, x, y, seriesInPanel, plot)
	case MarkLine:
		d.lineMark(ser, x, y, false)
		d.endMarker(ser, x, y, seriesInPanel, plot)
	default:
		d.lineMark(ser, x, y, false)
		d.endMarker(ser, x, y, seriesInPanel, plot)
	}
}

// runs splits a series into stretches of consecutive reported values. The gaps
// between them are the whole point: a missing reading is never bridged with a
// line and never drawn as a zero.
func runs(pts []Point) [][]int {
	var out [][]int
	var cur []int
	for i := range pts {
		if pts[i].Value == nil {
			if len(cur) > 0 {
				out = append(out, cur)
				cur = nil
			}
			continue
		}
		cur = append(cur, i)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

func (d *Drawing) lineMark(ser *Series, x timeScale, y linearScale, step bool) {
	for _, run := range runs(ser.Points) {
		pts := make([]Pt, 0, len(run)*2)
		for _, i := range run {
			p := Pt{X: x.at(ser.Points[i].Time), Y: y.at(*ser.Points[i].Value)}
			if step && len(pts) > 0 {
				pts = append(pts, Pt{X: p.X, Y: pts[len(pts)-1].Y})
			}
			pts = append(pts, p)
		}
		if len(pts) == 1 {
			// A lone reading between two gaps would vanish as a zero-length
			// line, so it becomes a dot.
			d.circle(pts[0].X, pts[0].Y, markerRadius, ser.Color, markerRing, d.Palette.Surface)
			continue
		}
		d.polyline(pts, ser.Color, lineWidth)
	}
}

// bandMark fills between lo and hi as a wash, with the mid line on top when the
// samples carry one.
func (d *Drawing) bandMark(ser *Series, x timeScale, y linearScale) {
	var lower []Pt
	upper := make([]Pt, 0, len(ser.Points))
	for i := range ser.Points {
		pt := &ser.Points[i]
		if pt.Low == nil || pt.High == nil {
			continue
		}
		px := x.at(pt.Time)
		upper = append(upper, Pt{X: px, Y: y.at(*pt.High)})
		lower = append(lower, Pt{X: px, Y: y.at(*pt.Low)})
	}
	if len(upper) >= 2 {
		poly := make([]Pt, 0, len(upper)+len(lower))
		poly = append(poly, upper...)
		for i := len(lower) - 1; i >= 0; i-- {
			poly = append(poly, lower[i])
		}
		d.polygon(poly, ser.Color, bandAlpha, "", 0)
	}
	d.lineMark(ser, x, y, false)
}

// barMark grows every value from the baseline. Bars keep a 2px surface gap
// between neighbours — white does the separating, never a stroke.
func (d *Drawing) barMark(ser *Series, plot rect, x timeScale, y linearScale) {
	reported := 0
	for i := range ser.Points {
		if ser.Points[i].Value != nil {
			reported++
		}
	}
	if reported == 0 {
		return
	}
	// The slot is the spacing between neighbouring samples — the first and last
	// sample sit on the plot edges, so n samples leave n-1 intervals. Counting
	// the unreported samples too keeps a bar the width of its own interval
	// instead of letting it swallow the gaps.
	intervals := math.Max(float64(len(ser.Points)-1), 1)
	slot := plot.w / intervals
	// 2px of surface between neighbours: white does the separating.
	w := math.Min(math.Max(slot-2, 1), barCap)

	base := y.at(0)
	if *ser.pointsDomainMin() > 0 {
		base = plot.bottom()
	}
	for i := range ser.Points {
		pt := &ser.Points[i]
		if pt.Value == nil {
			continue
		}
		cx := x.at(pt.Time)
		// The end bars are centred on the plot edges, so half of each would
		// hang over the axis labels. Trim them to the plot instead.
		left := math.Max(cx-w/2, plot.x)
		right := math.Min(cx+w/2, plot.right())
		top := y.at(*pt.Value)
		d.roundedTopRect(left, top, right-left, base-top, barRadius, ser.Color)
	}
}

// pointsDomainMin is the lowest reported value, or zero for an empty series.
func (s *Series) pointsDomainMin() *float64 {
	lo := math.Inf(1)
	for i := range s.Points {
		if s.Points[i].Value != nil {
			lo = math.Min(lo, *s.Points[i].Value)
		}
	}
	if math.IsInf(lo, 1) {
		return Float(0)
	}
	return Float(lo)
}

// endMarker dots the last reported value and, where the chart is uncrowded,
// labels it. Labels stay sparing on purpose: a number on every point is chaos
// and goes unread.
func (d *Drawing) endMarker(ser *Series, x timeScale, y linearScale, seriesInPanel int, plot rect) {
	last := -1
	for i := range ser.Points {
		if ser.Points[i].Value != nil {
			last = i
		}
	}
	if last < 0 {
		return
	}
	px, py := x.at(ser.Points[last].Time), y.at(*ser.Points[last].Value)
	d.circle(px, py, markerRadius, ser.Color, markerRing, d.Palette.Surface)

	if seriesInPanel > 2 {
		return
	}
	label := FormatValue(ser.Parameter, *ser.Points[last].Value)
	if ser.Unit != "" {
		label += " " + ser.Unit
	}
	// Set inside the plot's right edge: the margin outside is only wide enough
	// for half a tick label, and a clipped label is worse than none.
	if measureText(label, sizeAxis, true) > plot.w/3 {
		return
	}
	d.text(label, px-markerRadius-4, py-markerRadius-2, sizeAxis, true, d.Palette.InkSecondary, 1, 1)
}
