package chart

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"strconv"
	"strings"
	"sync"

	"github.com/fogleman/gg"
)

// pngBufferPool reuses the buffer png.Encode writes into. A chart endpoint
// encodes one image per request; without the pool every request grows and then
// discards a buffer the size of the finished PNG.
var pngBufferPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// encoderBufferPool recycles the encoder's own scratch space — the largest
// single allocation in a render — across requests in the same process.
type encoderBufferPool struct{ pool sync.Pool }

func (p *encoderBufferPool) Get() *png.EncoderBuffer {
	buf, _ := p.pool.Get().(*png.EncoderBuffer)
	return buf
}

func (p *encoderBufferPool) Put(buf *png.EncoderBuffer) {
	p.pool.Put(buf)
}

// One process-wide encoder: it holds nothing but the pool.
var pngEncoder = png.Encoder{BufferPool: &encoderBufferPool{}}

// renderPNG transcribes a draw list into a PNG, op for op, with no geometry of
// its own — every coordinate was decided in build.go, so the raster and the
// vector output are the same picture.
func renderPNG(d *Drawing) ([]byte, error) {
	dc := gg.NewContext(d.Width, d.Height)

	for i := range d.Ops {
		if err := drawPNGOp(dc, &d.Ops[i]); err != nil {
			return nil, err
		}
	}

	buf, _ := pngBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer pngBufferPool.Put(buf)

	if err := pngEncoder.Encode(buf, dc.Image()); err != nil {
		return nil, fmt.Errorf("chart: encode png: %w", err)
	}
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out, nil
}

func drawPNGOp(dc *gg.Context, op *Op) error {
	switch op.Kind {
	case OpRect:
		drawPNGRect(dc, op)
	case OpPolyline:
		dc.NewSubPath()
		for i, p := range op.Points {
			if i == 0 {
				dc.MoveTo(p.X, p.Y)
				continue
			}
			dc.LineTo(p.X, p.Y)
		}
		dc.SetLineWidth(op.StrokeWidth)
		dc.SetLineCapRound()
		dc.SetLineJoinRound()
		dc.SetColor(parseHex(op.Stroke, 1))
		dc.Stroke()
	case OpPolygon:
		dc.NewSubPath()
		for i, p := range op.Points {
			if i == 0 {
				dc.MoveTo(p.X, p.Y)
				continue
			}
			dc.LineTo(p.X, p.Y)
		}
		dc.ClosePath()
		fillThenStroke(dc, op)
	case OpCircle:
		dc.DrawCircle(op.X, op.Y, op.Radius)
		fillThenStroke(dc, op)
	case OpText:
		return drawPNGText(dc, op)
	default:
	}
	return nil
}

func drawPNGRect(dc *gg.Context, op *Op) {
	if op.Radius <= 0 {
		dc.DrawRectangle(op.X, op.Y, op.W, op.H)
		fillThenStroke(dc, op)
		return
	}
	// Two rounded top corners, square feet — the same outline the SVG path
	// draws.
	x, y, w, h, r := op.X, op.Y, op.W, op.H, op.Radius
	dc.NewSubPath()
	dc.MoveTo(x, y+r)
	dc.DrawArc(x+r, y+r, r, gg.Radians(180), gg.Radians(270))
	dc.LineTo(x+w-r, y)
	dc.DrawArc(x+w-r, y+r, r, gg.Radians(270), gg.Radians(360))
	dc.LineTo(x+w, y+h)
	dc.LineTo(x, y+h)
	dc.ClosePath()
	fillThenStroke(dc, op)
}

// fillThenStroke paints in SVG order: fill first, then any stroke on top, so a
// marker's surface ring covers the outer half of its own fill in both formats.
func fillThenStroke(dc *gg.Context, op *Op) {
	stroke, strokeWidth := op.Stroke, op.StrokeWidth
	if op.Ring > 0 && op.RingColor != "" {
		stroke, strokeWidth = op.RingColor, op.Ring
	}

	if op.Fill != "" {
		dc.SetColor(parseHex(op.Fill, op.alpha()))
		if stroke == "" || strokeWidth <= 0 {
			dc.Fill()
			return
		}
		dc.FillPreserve()
	}
	if stroke == "" || strokeWidth <= 0 {
		dc.ClearPath()
		return
	}
	dc.SetLineWidth(strokeWidth)
	dc.SetColor(parseHex(stroke, 1))
	dc.Stroke()
}

func drawPNGText(dc *gg.Context, op *Op) error {
	face, err := acquireFace(op.Size, op.Bold)
	if err != nil {
		return err
	}
	defer releaseFace(op.Size, op.Bold, face)

	dc.SetFontFace(face)
	dc.SetColor(parseHex(op.Fill, op.alpha()))
	x, y := op.textOrigin()
	dc.DrawString(op.Text, x, y)
	return nil
}

// parseHex turns "#rrggbb" (or "#rgb") into a colour with alpha premultiplied
// the way gg expects. An unparseable colour falls back to mid grey rather than
// panicking a render: a wrong-coloured chart is recoverable, a 500 is not.
func parseHex(hex string, alpha float64) color.Color {
	fallback := color.NRGBA{R: 0x89, G: 0x87, B: 0x81, A: 0xFF}
	s := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return fallback
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return fallback
	}
	a := 255.0
	if alpha > 0 && alpha < 1 {
		a *= alpha
	}
	return color.NRGBA{
		R: uint8(v >> 16 & 0xFF),
		G: uint8(v >> 8 & 0xFF),
		B: uint8(v & 0xFF),
		A: uint8(a + 0.5),
	}
}
