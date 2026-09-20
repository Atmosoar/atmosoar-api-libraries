package chart

// This file defines the one intermediate form both renderers consume.
//
// Layout and geometry happen once, in build.go, and land here as a flat list of
// device-space operations. svg.go and png.go do nothing but transcribe that
// list. That is what keeps the two outputs from drifting: there is no second
// place where a margin, a tick position or a marker radius is decided, so a
// geometry change lands in both formats or in neither.

// OpKind discriminates a draw operation.
type OpKind string

// The draw operations. Deliberately few: everything a weather chart needs is a
// rectangle, a polyline, a polygon, a circle or a run of text.
const (
	OpRect     OpKind = "rect"
	OpPolyline OpKind = "polyline"
	OpPolygon  OpKind = "polygon"
	OpCircle   OpKind = "circle"
	OpText     OpKind = "text"
)

// Pt is a device-space point, in pixels from the top-left of the canvas.
type Pt struct {
	X, Y float64
}

// Op is one draw operation. Which fields matter depends on Kind; the zero value
// of the rest is ignored.
type Op struct {
	Kind OpKind

	// Rect: top-left plus size. Circle: centre in X, Y. Text: anchor point.
	X, Y, W, H float64
	// Radius is a circle's radius, or a rectangle's corner radius.
	Radius float64
	// Points carries a polyline or polygon.
	Points []Pt

	// Fill is a resolved hex colour, empty for no fill. Alpha scales it; zero
	// means fully opaque (an explicit 0 opacity would draw nothing, so the
	// builders never emit one).
	Fill  string
	Alpha float64
	// Stroke and StrokeWidth outline the shape.
	Stroke      string
	StrokeWidth float64
	// Ring draws a band of the surface colour just outside a circle, so a
	// marker stays legible where it crosses a line or another marker.
	Ring      float64
	RingColor string

	// Text and its typography. AnchorX/AnchorY place the string relative to
	// X, Y: 0 is left/top, 0.5 centre, 1 right/bottom.
	Text             string
	Size             float64
	Bold             bool
	AnchorX, AnchorY float64
}

// Drawing is a fully laid-out chart: a canvas size, the resolved palette and
// the ops that paint it, in back-to-front order.
type Drawing struct {
	Width, Height int
	Palette       ResolvedPalette
	Ops           []Op
}

// alpha returns the op's effective opacity.
func (o *Op) alpha() float64 {
	if o.Alpha <= 0 || o.Alpha > 1 {
		return 1
	}
	return o.Alpha
}

func (d *Drawing) add(op Op) {
	d.Ops = append(d.Ops, op)
}

func (d *Drawing) rect(x, y, w, h float64, fill string, alpha float64) {
	if w <= 0 || h <= 0 {
		return
	}
	d.add(Op{Kind: OpRect, X: x, Y: y, W: w, H: h, Fill: fill, Alpha: alpha})
}

// roundedTopRect is the bar mark: rounded at the data end, square at the
// baseline. Bars growing downward (a negative value) round at the bottom.
func (d *Drawing) roundedTopRect(x, y, w, h, radius float64, fill string) {
	if w <= 0 || h == 0 {
		return
	}
	if h < 0 {
		y, h = y+h, -h
	}
	if radius > w/2 {
		radius = w / 2
	}
	if radius > h {
		radius = h
	}
	d.add(Op{Kind: OpRect, X: x, Y: y, W: w, H: h, Radius: radius, Fill: fill})
}

func (d *Drawing) polyline(pts []Pt, stroke string, width float64) {
	if len(pts) < 2 {
		return
	}
	d.add(Op{Kind: OpPolyline, Points: pts, Stroke: stroke, StrokeWidth: width})
}

func (d *Drawing) polygon(pts []Pt, fill string, alpha float64, stroke string, strokeWidth float64) {
	if len(pts) < 3 {
		return
	}
	d.add(Op{
		Kind: OpPolygon, Points: pts,
		Fill: fill, Alpha: alpha, Stroke: stroke, StrokeWidth: strokeWidth,
	})
}

func (d *Drawing) circle(cx, cy, r float64, fill string, ring float64, ringColor string) {
	if r <= 0 {
		return
	}
	d.add(Op{Kind: OpCircle, X: cx, Y: cy, Radius: r, Fill: fill, Ring: ring, RingColor: ringColor})
}

func (d *Drawing) strokedCircle(cx, cy, r float64, stroke string, width float64) {
	if r <= 0 {
		return
	}
	d.add(Op{Kind: OpCircle, X: cx, Y: cy, Radius: r, Stroke: stroke, StrokeWidth: width})
}

// text places a string. Colour is always an ink token: a label never wears its
// series' hue, because a light categorical step is illegible as text.
func (d *Drawing) text(s string, x, y, size float64, bold bool, ink string, ax, ay float64) {
	if s == "" {
		return
	}
	d.add(Op{
		Kind: OpText, Text: s, X: x, Y: y, Size: size, Bold: bold,
		Fill: ink, AnchorX: ax, AnchorY: ay,
	})
}

// textOrigin resolves a text op's anchors into the left edge and the baseline.
// Both renderers call it, so a label sits in exactly the same place in the PNG
// and the SVG.
func (o *Op) textOrigin() (x, y float64) {
	x = o.X
	if o.AnchorX != 0 {
		x -= o.AnchorX * measureText(o.Text, o.Size, o.Bold)
	}
	// AnchorY 1 puts the baseline on Y, 0.5 centres the cap height on it, 0
	// hangs the text below it.
	return x, o.Y + (1-o.AnchorY)*textHeight(o.Size)
}
