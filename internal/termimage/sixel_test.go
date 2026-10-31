package termimage

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi/sixel"
)

func TestSixelRoundTripFitsAndKeepsAspectAndColors(t *testing.T) {
	src := image.NewRGBA(image.Rect(4, 7, 12, 11))
	for y := 7; y < 11; y++ {
		for x := 4; x < 12; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	data, err := EncodeSixel(src, 32, 24)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(data, "\x1bP0;0;0q") || !strings.HasSuffix(data, "\x1b\\") {
		t.Fatal("missing bounded DCS envelope")
	}
	decoded, err := (&sixel.Decoder{}).Decode(strings.NewReader(strings.TrimSuffix(strings.TrimPrefix(data, "\x1bP0;0;0q"), "\x1b\\")))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds() != image.Rect(0, 0, 32, 24) {
		t.Fatal(decoded.Bounds())
	}
	red, green, blue, _ := decoded.At(16, 12).RGBA()
	if red < 65000 || green != 0 || blue != 0 {
		t.Fatal("color lost", decoded.At(16, 12))
	}
	for _, p := range []image.Point{{16, 0}, {16, 3}, {16, 20}, {16, 23}} {
		r, g, b, _ := decoded.At(p.X, p.Y).RGBA()
		if r+g+b != 0 {
			t.Fatal("aspect ratio/letterbox lost", p)
		}
	}
}
func TestSixelRejectsUnboundedWork(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 1, 1))
	for _, size := range []image.Point{{0, 1}, {1, 0}, {-1, 1}, {MaxSixelWidth + 1, 1}, {1, MaxSixelHeight + 1}} {
		if _, err := EncodeSixel(im, size.X, size.Y); err == nil {
			t.Fatal("invalid viewport accepted", size)
		}
	}
	if _, err := EncodeSixel(nil, 1, 1); err == nil {
		t.Fatal("nil image accepted")
	}
	var b limitedSixel
	if _, err := b.WriteString(strings.Repeat("x", maxSixelBytes+1)); err == nil || !b.overflow || b.Len() != 0 {
		t.Fatal("string writes bypass output cap")
	}
}

// Walk RLE and color commands independently, so a matching encoder/decoder
// bug cannot hide dropped literal pixels after a repeated run.
func decodeSixelPixels(t *testing.T, encoded string) image.Image {
	t.Helper()
	data := []byte(strings.TrimSuffix(strings.TrimPrefix(encoded, "\x1bP0;0;0q"), "\x1b\\"))
	raster, n := sixel.DecodeRaster(data)
	if n == 0 {
		t.Fatal("missing raster attributes")
	}
	im := image.NewRGBA(image.Rect(0, 0, raster.Ph, raster.Pv))
	palette := sixel.DefaultPalette()
	x, y, index := 0, 0, 0
	draw := func(value byte, count int) {
		if x+count > im.Bounds().Dx() {
			t.Fatal("Sixel scanline overflows raster")
		}
		for k := 0; k < count; k++ {
			for bit := 0; bit < 6; bit++ {
				if (value-'?')&(1<<bit) != 0 && y+bit < im.Bounds().Dy() {
					im.Set(x, y+bit, palette[index])
				}
			}
			x++
		}
	}
	for i := n; i < len(data); {
		b := data[i]
		switch {
		case b == '#':
			c, n := sixel.DecodeColor(data[i:])
			if n == 0 || c.Pc < 0 || c.Pc >= 256 {
				t.Fatal("invalid color")
			}
			index = c.Pc
			if c.Pu > 0 {
				palette[index] = c
			}
			i += n
		case b == '!':
			r, n := sixel.DecodeRepeat(data[i:])
			if n == 0 {
				t.Fatal("invalid repeat")
			}
			draw(r.Char, r.Count)
			i += n
		case b == '$':
			x = 0
			i++
		case b == '-':
			x = 0
			y += 6
			i++
		case b >= '?' && b <= '~':
			draw(b, 1)
			i++
		default:
			t.Fatalf("unexpected Sixel byte %x", b)
		}
	}
	return im
}
func TestSixelPhotographicColorsAndMixedRuns(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 128, 96))
	for y := 0; y < 96; y++ {
		for x := 0; x < 128; x++ {
			src.SetRGBA(x, y, color.RGBA{uint8(x * 2), uint8(y * 2), uint8(x + y), 255})
		}
	}
	data, err := EncodeSixel(src, 128, 96)
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeSixelPixels(t, data)
	var errorSum uint64
	for y := 0; y < 96; y++ {
		for x := 0; x < 128; x++ {
			a, b, c, _ := src.At(x, y).RGBA()
			u, v, w, _ := decoded.At(x, y).RGBA()
			for _, p := range [][2]uint32{{a, u}, {b, v}, {c, w}} {
				if p[0] > p[1] {
					errorSum += uint64(p[0] - p[1])
				} else {
					errorSum += uint64(p[1] - p[0])
				}
			}
		}
	}
	if average := float64(errorSum) / (128 * 96 * 3 * 257); average > 8 {
		t.Fatalf("quantization or raster lost pixels: mean channel error %.2f", average)
	}
}
