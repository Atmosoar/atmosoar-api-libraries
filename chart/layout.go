package chart

import "math"

// Fixed geometry. These are the only place any of these numbers is decided, so
// SVG and PNG cannot disagree about them.
const (
	// gutter is the side margin. 16px matches the platform's phone-width rule,
	// so a chart embedded in a narrow column never touches the edge.
	gutter = 16.0

	titleGap    = 6.0
	legendGap   = 10.0
	panelGap    = 18.0
	panelTitleH = 16.0
	// xAxisH is the strip under the lowest plot that carries the time labels.
	xAxisH = 18.0
	// footerH is the attribution strip; zero when there is nothing to attribute.
	footerH = 14.0

	// tickLen is how far a tick mark pokes out of the plot.
	tickLen = 4.0
	// yLabelGap separates a y tick label from the plot edge.
	yLabelGap = 6.0

	// lineWidth is every data line. Grid and axis are hairlines.
	lineWidth = 2.0
	hairline  = 1.0
	// markerRadius is the end-dot; ≥ 4 keeps the 8px minimum mark size.
	markerRadius = 4.0
	// markerRing is the surface ring that keeps a dot legible over a line.
	markerRing = 2.0
	// barCap caps a bar's thickness so a sparse series does not draw slabs.
	barCap = 24.0
	// barRadius rounds the data end of a bar; the baseline end stays square.
	barRadius = 4.0
	// bandAlpha is the wash for an area or spread fill: a wash, never a block.
	bandAlpha = 0.10
	// thresholdAlpha is the shading behind the marks for a status band. Lower
	// than a series wash — it is background, and the data has to win.
	thresholdAlpha = 0.08
)

type rect struct {
	x, y, w, h float64
}

func (r rect) right() float64  { return r.x + r.w }
func (r rect) bottom() float64 { return r.y + r.h }

// layout holds every rectangle a chart is assembled from.
type layout struct {
	width, height float64

	titleAt    Pt
	subtitleAt Pt
	legendAt   Pt
	legendW    float64

	// plots is one data area per panel, top to bottom.
	plots []rect
	// panelTitleAt is where each panel's title sits, empty for untitled panels.
	panelTitleAt []Pt

	xAxisLabelY float64
	footerAt    Pt
	hasFooter   bool

	// roseCentre and roseRadius are only set for a windrose.
	roseCentre Pt
	roseRadius float64
}

// computeLayout divides the canvas. It needs the resolved spec because the
// y-axis width depends on the widest tick label, and the right margin on half
// the last time label — measuring after the fact is how labels get clipped.
func computeLayout(s *Spec) layout {
	l := layout{width: float64(s.Width), height: float64(s.Height)}

	y := gutter
	if s.Title != "" {
		l.titleAt = Pt{X: gutter, Y: y + textHeight(sizeTitle)}
		y += textHeight(sizeTitle) + titleGap
	}
	if s.Subtitle != "" {
		l.subtitleAt = Pt{X: gutter, Y: y + textHeight(sizeSubtitle)}
		y += textHeight(sizeSubtitle) + titleGap
	}
	if legendEntryCount(s) >= 2 {
		l.legendAt = Pt{X: gutter, Y: y + textHeight(sizeLegend)}
		l.legendW = l.width - 2*gutter
		y += textHeight(sizeLegend) + legendGap
	}

	bottom := l.height - gutter
	if s.Source != "" || s.Generated != nil {
		l.hasFooter = true
		l.footerAt = Pt{X: gutter, Y: bottom}
		bottom -= footerH
	}

	if s.Kind == KindWindrose {
		l.layoutRose(y, bottom)
		return l
	}

	l.xAxisLabelY = bottom - textHeight(sizeAxis)/2
	plotBottom := bottom - xAxisH

	yAxisW := 0.0
	for i := range s.Panels {
		for _, t := range s.Panels[i].Y.Ticks {
			yAxisW = math.Max(yAxisW, measureText(t.Label, sizeAxis, false))
		}
	}
	left := gutter + yAxisW + yLabelGap + tickLen

	// The last time label is centred on the plot's right edge, so half of it
	// has to fit outside.
	rightPad := gutter
	if n := len(s.X.Ticks); n > 0 {
		rightPad = math.Max(gutter, measureText(s.X.Ticks[n-1].Label, sizeAxis, false)/2+4)
	}
	plotW := l.width - left - rightPad

	totalWeight, titled := 0.0, 0
	for i := range s.Panels {
		totalWeight += panelWeight(&s.Panels[i])
		if panelTitle(&s.Panels[i]) != "" {
			titled++
		}
	}
	gaps := panelGap * float64(len(s.Panels)-1)
	avail := plotBottom - y - gaps - panelTitleH*float64(titled)
	avail = math.Max(avail, float64(len(s.Panels))*minPanelHeight)

	l.plots = make([]rect, len(s.Panels))
	l.panelTitleAt = make([]Pt, len(s.Panels))
	top := y
	for i := range s.Panels {
		if panelTitle(&s.Panels[i]) != "" {
			l.panelTitleAt[i] = Pt{X: left, Y: top + textHeight(sizeAxis)}
			top += panelTitleH
		}
		h := avail * panelWeight(&s.Panels[i]) / totalWeight
		l.plots[i] = rect{x: left, y: top, w: plotW, h: h}
		top += h + panelGap
	}
	return l
}

// minPanelHeight keeps a panel tall enough to read even when a caller asks for
// more panels than the canvas comfortably holds.
const minPanelHeight = 48.0

// layoutRose centres a square polar plot in the space left over, leaving room
// for the compass labels outside the outermost ring.
func (l *layout) layoutRose(top, bottom float64) {
	area := rect{x: gutter, y: top, w: l.width - 2*gutter, h: bottom - top}
	l.roseCentre = Pt{X: area.x + area.w/2, Y: area.y + area.h/2}
	// The compass labels ride outside the rings; reserve an em and a half.
	l.roseRadius = math.Max(20, math.Min(area.w, area.h)/2-sizeAxis*1.5)
}

func panelWeight(p *Panel) float64 {
	if p.Weight <= 0 {
		return 1
	}
	return p.Weight
}

// legendEntryCount is how many entries a legend would carry. One series needs
// no legend box: the title already names what is plotted. A windrose is the
// exception — its legend carries the speed classes, not the series, so it is
// there even though the rose plots one series.
func legendEntryCount(s *Spec) int {
	if s.Kind == KindWindrose {
		if s.Windrose == nil {
			return len(defaultSpeedBands) + 1
		}
		return len(s.Windrose.SpeedBands) + 1
	}
	n := 0
	single := true
	for i := range s.Panels {
		n += len(s.Panels[i].Series)
		if len(s.Panels[i].Series) != 1 || s.Panels[i].Title == "" {
			single = false
		}
	}
	// Stacked panels that each carry one titled series name themselves; a
	// legend would only restate the titles.
	if s.Kind == KindMeteogram && single {
		return 0
	}
	return n
}
