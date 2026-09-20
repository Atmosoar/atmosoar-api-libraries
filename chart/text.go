package chart

import (
	"fmt"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// Type sizes, in pixels. One sans family at a few fixed sizes: the data is the
// only thing allowed to be loud.
const (
	sizeTitle    = 15
	sizeSubtitle = 12
	sizeAxis     = 11
	sizeLegend   = 11
	sizeFooter   = 10
)

// svgFontStack is what the SVG output asks the viewer for. Layout is measured
// with Go Sans, whose metrics are close enough to a UI sans that reserved space
// still fits; the PNG output rasterises Go Sans itself, so it is exact there.
const svgFontStack = `system-ui, -apple-system, "Segoe UI", "Go", sans-serif`

type faceKey struct {
	size float64
	bold bool
}

// Faces are not safe for concurrent use, and neither is a gg context holding
// one. Parsing happens once; faces are then lent out one owner at a time, so
// two concurrent renders never share one.
var (
	fontOnce    sync.Once
	fontRegular *sfnt.Font
	fontBold    *sfnt.Font
	errFont     error

	faceMu   sync.Mutex
	facePool = map[faceKey][]font.Face{}
)

func loadFonts() (*sfnt.Font, *sfnt.Font, error) {
	fontOnce.Do(func() {
		fontRegular, errFont = sfnt.Parse(goregular.TTF)
		if errFont != nil {
			errFont = fmt.Errorf("chart: parse regular font: %w", errFont)
			return
		}
		fontBold, errFont = sfnt.Parse(gobold.TTF)
		if errFont != nil {
			errFont = fmt.Errorf("chart: parse bold font: %w", errFont)
		}
	})
	return fontRegular, fontBold, errFont
}

// acquireFace lends out a face for one size/weight. The caller must release it.
func acquireFace(size float64, bold bool) (font.Face, error) {
	key := faceKey{size: size, bold: bold}

	faceMu.Lock()
	if pooled := facePool[key]; len(pooled) > 0 {
		f := pooled[len(pooled)-1]
		facePool[key] = pooled[:len(pooled)-1]
		faceMu.Unlock()
		return f, nil
	}
	faceMu.Unlock()

	regular, boldFont, err := loadFonts()
	if err != nil {
		return nil, err
	}
	src := regular
	if bold {
		src = boldFont
	}
	f, err := opentype.NewFace(src, &opentype.FaceOptions{
		Size:    size,
		DPI:     72, // 72 DPI makes one point one pixel, so Size is the pixel size.
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("chart: build %vpx face: %w", size, err)
	}
	return f, nil
}

// releaseFace returns a face to the pool. The pool is bounded because the set
// of sizes is fixed and small.
func releaseFace(size float64, bold bool, f font.Face) {
	if f == nil {
		return
	}
	key := faceKey{size: size, bold: bold}
	faceMu.Lock()
	if len(facePool[key]) < 8 {
		facePool[key] = append(facePool[key], f)
	}
	faceMu.Unlock()
}

// measureText is the width a string will occupy. Layout calls it to reserve
// space, so a label is never clipped by its own mark: where the text does not
// fit, the builders drop it rather than crop it.
func measureText(s string, size float64, bold bool) float64 {
	if s == "" {
		return 0
	}
	f, err := acquireFace(size, bold)
	if err != nil {
		// Without metrics, assume a generous half-em per rune: over-reserving
		// space costs a little air, under-reserving costs a clipped label.
		return float64(len([]rune(s))) * size * 0.6
	}
	w := font.MeasureString(f, s)
	releaseFace(size, bold, f)
	return float64(w) / 64
}

// textHeight is the cap-to-baseline allowance for one line at a size. Used for
// vertical anchoring, which both renderers must agree on.
func textHeight(size float64) float64 {
	return size * 0.72
}

// truncateToWidth shortens a string with an ellipsis until it fits, and returns
// the empty string when even the ellipsis will not fit.
func truncateToWidth(s string, size float64, bold bool, maxW float64) string {
	if measureText(s, size, bold) <= maxW {
		return s
	}
	runes := []rune(s)
	for n := len(runes) - 1; n > 0; n-- {
		cand := string(runes[:n]) + "…"
		if measureText(cand, size, bold) <= maxW {
			return cand
		}
	}
	return ""
}
