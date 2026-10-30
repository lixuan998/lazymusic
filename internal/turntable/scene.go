// Package turntable composes album art into the embedded photographic player.
package turntable

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"sync"

	"lazymusic/internal/artwork"
)

//go:embed assets/turntable.png
var basePNG []byte

const Width = 1536
const Height = 1024

// These coordinates are measured on the embedded image, not terminal cells.
// Composing the cover and chassis into one raster keeps them aligned at every
// terminal font size, cell aspect ratio and placement scale.
const CenterX = 571
const CenterY = 471
const CoverSide = 320

var baseOnce sync.Once
var baseImage image.Image
var baseError error

type Scene struct {
	PNG    []byte
	Pixels image.Image
}

func Compose(cover artwork.Image) (Scene, error) {
	baseOnce.Do(func() { baseImage, baseError = png.Decode(bytes.NewReader(basePNG)) })
	if baseError != nil {
		return Scene{}, fmt.Errorf("turntable image could not be decoded")
	}
	if !cover.Valid() {
		return Scene{basePNG, baseImage}, nil
	}
	var source image.Image
	var err error
	if len(cover.Original) > 0 {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(cover.Original))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > 16<<20 {
			return Scene{}, fmt.Errorf("invalid album artwork dimensions")
		}
		source, _, err = image.Decode(bytes.NewReader(cover.Original))
		if err != nil {
			return Scene{}, fmt.Errorf("album artwork could not be decoded")
		}
	} else {
		fallback := image.NewRGBA(image.Rect(0, 0, cover.Width, cover.Height))
		for y := 0; y < cover.Height; y++ {
			for x := 0; x < cover.Width; x++ {
				i := (y*cover.Width + x) * 3
				fallback.SetRGBA(x, y, color.RGBA{cover.RGB[i], cover.RGB[i+1], cover.RGB[i+2], 255})
			}
		}
		source = fallback
	}
	out := image.NewRGBA(baseImage.Bounds())
	draw.Draw(out, out.Bounds(), baseImage, baseImage.Bounds().Min, draw.Src)
	box := image.Rect(CenterX-CoverSide/2, CenterY-CoverSide/2, CenterX+CoverSide/2, CenterY+CoverSide/2)
	cropSide := float64(min(source.Bounds().Dx(), source.Bounds().Dy()))
	cropLeft := float64(source.Bounds().Min.X) + (float64(source.Bounds().Dx())-cropSide)/2
	cropTop := float64(source.Bounds().Min.Y) + (float64(source.Bounds().Dy())-cropSide)/2
	radius := float64(CoverSide) / 2
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			coverage := math.Max(0, math.Min(1, radius+.5-math.Hypot(float64(x)+.5-CenterX, float64(y)+.5-CenterY)))
			if coverage == 0 {
				continue
			}
			sx := cropLeft + (float64(x-box.Min.X)+.5)*cropSide/float64(CoverSide) - .5
			sy := cropTop + (float64(y-box.Min.Y)+.5)*cropSide/float64(CoverSide) - .5
			c := sample(source, sx, sy)
			mask := uint32(math.Round(coverage * 255))
			c = color.RGBA{uint8((uint32(c.R)*mask + 127) / 255), uint8((uint32(c.G)*mask + 127) / 255), uint8((uint32(c.B)*mask + 127) / 255), uint8((uint32(c.A)*mask + 127) / 255)}
			old := out.RGBAAt(x, y)
			a := uint32(c.A)
			out.SetRGBA(x, y, color.RGBA{R: uint8(uint32(c.R) + (uint32(old.R)*(255-a)+127)/255), G: uint8(uint32(c.G) + (uint32(old.G)*(255-a)+127)/255), B: uint8(uint32(c.B) + (uint32(old.B)*(255-a)+127)/255), A: 255})
		}
	}
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, out); err != nil {
		return Scene{}, fmt.Errorf("turntable image could not be encoded")
	}
	return Scene{encoded.Bytes(), out}, nil
}

// Bilinear sampling retains smooth image detail while the terminal scales the
// completed scene. Inputs are premultiplied, as required for alpha compositing.
func sample(im image.Image, x, y float64) color.RGBA {
	b := im.Bounds()
	x = math.Max(float64(b.Min.X), math.Min(float64(b.Max.X-1), x))
	y = math.Max(float64(b.Min.Y), math.Min(float64(b.Max.Y-1), y))
	x0, y0 := int(x), int(y)
	x1, y1 := min(x0+1, b.Max.X-1), min(y0+1, b.Max.Y-1)
	fx, fy := x-float64(x0), y-float64(y0)
	var channels [4]float64
	for _, p := range []struct {
		x, y   int
		weight float64
	}{{x0, y0, (1 - fx) * (1 - fy)}, {x1, y0, fx * (1 - fy)}, {x0, y1, (1 - fx) * fy}, {x1, y1, fx * fy}} {
		r, g, b, a := im.At(p.x, p.y).RGBA()
		for i, v := range [4]uint32{r, g, b, a} {
			channels[i] += float64(v) * p.weight / 257
		}
	}
	return color.RGBA{uint8(channels[0] + .5), uint8(channels[1] + .5), uint8(channels[2] + .5), uint8(channels[3] + .5)}
}
