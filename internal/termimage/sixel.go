package termimage

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi/sixel"
)

const MaxSixelWidth = 1280
const MaxSixelHeight = 720
const maxSixelBytes = 8 << 20

// SixelRefresh wakes the output compositor even when the text view is unchanged.
// Saving/restoring the cursor leaves the text renderer's position and style intact.
const SixelRefresh = "\x1b7\x1b8"
const SixelQuery = "\x1b[c\x1b[16t\x1b[14t"
const SixelSizeQuery = "\x1b[16t\x1b[14t"

type limitedSixel struct {
	strings.Builder
	overflow bool
}

func (b *limitedSixel) Write(p []byte) (int, error) {
	if len(p) > maxSixelBytes-b.Len() {
		b.overflow = true
		return 0, fmt.Errorf("sixel frame exceeds output limit")
	}
	return b.Builder.Write(p)
}

func (b *limitedSixel) WriteString(s string) (int, error) { return b.Write([]byte(s)) }

// EncodeSixel returns one opaque, self-contained DCS image. A bounded viewport
// and private palette keep frames independent; no files or subprocesses are used.
func EncodeSixel(src image.Image, width, height int) (string, error) {
	if src == nil || src.Bounds().Empty() || width < 1 || height < 1 || width > MaxSixelWidth || height > MaxSixelHeight {
		return "", fmt.Errorf("invalid sixel image dimensions")
	}
	bounds := src.Bounds()
	if bounds.Dx() > 4096 || bounds.Dy() > 4096 || bounds.Dx()*bounds.Dy() > 16<<20 {
		return "", fmt.Errorf("sixel source too large")
	}
	// Bilinear resampling preserves the source aspect ratio, with opaque black
	// letterboxing where rounding of terminal cells leaves a different ratio.
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	scale := math.Min(float64(width)/float64(bounds.Dx()), float64(height)/float64(bounds.Dy()))
	w, h := max(1, int(float64(bounds.Dx())*scale+.5)), max(1, int(float64(bounds.Dy())*scale+.5))
	left, top := (width-w)/2, (height-h)/2
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			dst.SetRGBA(x, y, color.RGBA{A: 255})
		}
	}
	for y := 0; y < h; y++ {
		sy := max(0, min(float64(bounds.Dy()-1), (float64(y)+.5)/scale-.5))
		y0 := int(sy)
		y1 := min(y0+1, bounds.Dy()-1)
		fy := sy - float64(y0)
		for x := 0; x < w; x++ {
			sx := max(0, min(float64(bounds.Dx()-1), (float64(x)+.5)/scale-.5))
			x0 := int(sx)
			x1 := min(x0+1, bounds.Dx()-1)
			fx := sx - float64(x0)
			var rgb [3]float64
			for _, p := range [...]struct {
				x, y   int
				weight float64
			}{{x0, y0, (1 - fx) * (1 - fy)}, {x1, y0, fx * (1 - fy)}, {x0, y1, (1 - fx) * fy}, {x1, y1, fx * fy}} {
				a, b, c, _ := src.At(bounds.Min.X+p.x, bounds.Min.Y+p.y).RGBA()
				for i, v := range [3]uint32{a, b, c} {
					rgb[i] += float64(v) * p.weight / 257
				}
			}
			dst.SetRGBA(left+x, top+y, color.RGBA{uint8(rgb[0] + .5), uint8(rgb[1] + .5), uint8(rgb[2] + .5), 255})
		}
	}
	var payload limitedSixel
	if err := (&sixel.Encoder{}).Encode(&payload, dst); err != nil {
		return "", err
	}
	// The upstream encoder ignores some writer errors. Keep our own sticky limit.
	if payload.overflow {
		return "", fmt.Errorf("sixel frame exceeds output limit")
	}
	return "\x1bP0;0;0q" + payload.String() + "\x1b\\", nil
}
