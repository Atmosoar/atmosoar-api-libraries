package chart

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// renderSVG transcribes a draw list into SVG.
//
// The output is single-mode: the palette baked in is the one the spec asked
// for. A page that wants both themes asks for both and swaps them with
// prefers-color-scheme — a half-themed SVG, where the surface follows the
// viewer but the marks do not, is worse than an honest one.
func renderSVG(d *Drawing) []byte {
	var b strings.Builder
	b.Grow(len(d.Ops)*96 + 512)

	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" `+
		`viewBox="0 0 %d %d" role="img" data-mode="%s">`,
		d.Width, d.Height, d.Width, d.Height, d.Palette.Mode)
	// One family for everything, including the numbers: no display face.
	fmt.Fprintf(&b, `<style>text{font-family:%s;fill-opacity:1}</style>`, svgFontStack)

	for i := range d.Ops {
		writeSVGOp(&b, &d.Ops[i])
	}
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

func writeSVGOp(b *strings.Builder, op *Op) {
	switch op.Kind {
	case OpRect:
		writeSVGRect(b, op)
	case OpPolyline:
		fmt.Fprintf(b, `<polyline points="%s" fill="none" stroke="%s" stroke-width="%s"`+
			` stroke-linejoin="round" stroke-linecap="round"/>`,
			svgPoints(op.Points), op.Stroke, num(op.StrokeWidth))
	case OpPolygon:
		b.WriteString(`<polygon points="` + svgPoints(op.Points) + `"`)
		writeSVGPaint(b, op)
		b.WriteString(`/>`)
	case OpCircle:
		fmt.Fprintf(b, `<circle cx="%s" cy="%s" r="%s"`, num(op.X), num(op.Y), num(op.Radius))
		writeSVGCirclePaint(b, op)
		b.WriteString(`/>`)
	case OpText:
		writeSVGText(b, op)
	default:
	}
}

func writeSVGRect(b *strings.Builder, op *Op) {
	if op.Radius > 0 {
		// Rounded at the data end, square at the baseline: a bar that rounds
		// both ends floats off its own zero.
		fmt.Fprintf(b, `<path d="%s"`, roundedTopPath(op))
		writeSVGPaint(b, op)
		b.WriteString(`/>`)
		return
	}
	fmt.Fprintf(b, `<rect x="%s" y="%s" width="%s" height="%s"`,
		num(op.X), num(op.Y), num(op.W), num(op.H))
	writeSVGPaint(b, op)
	b.WriteString(`/>`)
}

// roundedTopPath is the bar outline: two rounded top corners, square feet.
func roundedTopPath(op *Op) string {
	x, y, w, h, r := op.X, op.Y, op.W, op.H, op.Radius
	return fmt.Sprintf("M%s %sA%s %s 0 0 1 %s %sH%sA%s %s 0 0 1 %s %sV%sH%sZ",
		num(x), num(y+r),
		num(r), num(r), num(x+r), num(y),
		num(x+w-r),
		num(r), num(r), num(x+w), num(y+r),
		num(y+h), num(x))
}

func writeSVGPaint(b *strings.Builder, op *Op) {
	if op.Fill != "" {
		b.WriteString(` fill="` + op.Fill + `"`)
		if a := op.alpha(); a < 1 {
			b.WriteString(` fill-opacity="` + num(a) + `"`)
		}
	} else {
		b.WriteString(` fill="none"`)
	}
	if op.Stroke != "" && op.StrokeWidth > 0 {
		fmt.Fprintf(b, ` stroke="%s" stroke-width="%s"`, op.Stroke, num(op.StrokeWidth))
	}
}

// writeSVGCirclePaint paints a marker. The ring is a stroke in the surface
// colour centred on the circle's edge, which is what keeps a dot readable where
// it crosses its own line.
func writeSVGCirclePaint(b *strings.Builder, op *Op) {
	if op.Fill != "" {
		b.WriteString(` fill="` + op.Fill + `"`)
		if a := op.alpha(); a < 1 {
			b.WriteString(` fill-opacity="` + num(a) + `"`)
		}
	} else {
		b.WriteString(` fill="none"`)
	}
	switch {
	case op.Ring > 0 && op.RingColor != "":
		fmt.Fprintf(b, ` stroke="%s" stroke-width="%s"`, op.RingColor, num(op.Ring))
	case op.Stroke != "" && op.StrokeWidth > 0:
		fmt.Fprintf(b, ` stroke="%s" stroke-width="%s"`, op.Stroke, num(op.StrokeWidth))
	default:
	}
}

func writeSVGText(b *strings.Builder, op *Op) {
	x, y := op.textOrigin()
	weight := "400"
	if op.Bold {
		weight = "600"
	}
	fmt.Fprintf(b, `<text x="%s" y="%s" font-size="%s" font-weight="%s" fill="%s">%s</text>`,
		num(x), num(y), num(op.Size), weight, op.Fill, escapeXML(op.Text))
}

func svgPoints(pts []Pt) string {
	var b strings.Builder
	b.Grow(len(pts) * 14)
	for i, p := range pts {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(num(p.X))
		b.WriteByte(',')
		b.WriteString(num(p.Y))
	}
	return b.String()
}

// num prints a coordinate at two decimals without trailing zeros, which keeps
// the markup a third smaller than %g on a dense chart.
func num(v float64) string {
	r := math.Round(v*100) / 100
	if r == 0 {
		return "0"
	}
	return strconv.FormatFloat(r, 'f', -1, 64)
}

var xmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#39;",
)

func escapeXML(s string) string { return xmlEscaper.Replace(s) }
