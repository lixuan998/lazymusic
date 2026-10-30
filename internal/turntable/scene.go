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

//go:embed assets/turntable-stopped.png
var stoppedPNG []byte

const Width = 1536
const Height = 1024

// Coordinates belong to the photograph, never terminal cells. The cover fills
// the platter except for a narrow black rim, beneath the stationary tonearm.
const CenterX = 571
const CenterY = 471
const CoverSide = 816

var baseOnce sync.Once
var baseImage, pausedImage, stoppedImage *image.RGBA
var pausedPNG []byte
var baseError error

type Scene struct {
	PNG    []byte
	Pixels image.Image
}

// Renderer is immutable and retains only the decoded cover and its mask.
// Frames do not decode artwork again or retain an unbounded animation cache.
type Appearance uint8

const (
	Playing Appearance = iota
	Paused
	Stopped
)

type Renderer struct {
	base    *image.RGBA
	basePNG []byte
	cover   *image.RGBA
	mask    []uint8
}

func CoverBounds() image.Rectangle {
	return image.Rect(CenterX-CoverSide/2, CenterY-CoverSide/2, CenterX+CoverSide/2, CenterY+CoverSide/2)
}

func NewRenderer(cover artwork.Image) (*Renderer, error) {
	return NewRendererState(cover, Playing)
}

func NewRendererState(cover artwork.Image, appearance Appearance) (*Renderer, error) {
	baseOnce.Do(func() {
		decode := func(data []byte) *image.RGBA {
			decoded, err := png.Decode(bytes.NewReader(data))
			if err != nil || decoded.Bounds() != image.Rect(0, 0, Width, Height) {
				baseError = fmt.Errorf("invalid player photograph")
				return nil
			}
			im := image.NewRGBA(decoded.Bounds())
			draw.Draw(im, im.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
			return im
		}
		baseImage, stoppedImage = decode(basePNG), decode(stoppedPNG)
		if baseError != nil {
			return
		}
		pausedImage = image.NewRGBA(baseImage.Bounds())
		copy(pausedImage.Pix, baseImage.Pix)
		// Reuse the generated unlit lens with a feathered wood surround. The rest of
		// the paused photograph is pixel-identical to the playing chassis and arm.
		box := image.Rect(1197, 702, 1254, 760)
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := box.Min.X; x < box.Max.X; x++ {
				weight := math.Min(1, float64(min(x-box.Min.X, box.Max.X-1-x, y-box.Min.Y, box.Max.Y-1-y))/8)
				a, b := baseImage.RGBAAt(x, y), stoppedImage.RGBAAt(x, y)
				blend := func(p, q uint8) uint8 { return uint8(float64(p)*(1-weight) + float64(q)*weight + .5) }
				pausedImage.SetRGBA(x, y, color.RGBA{blend(a.R, b.R), blend(a.G, b.G), blend(a.B, b.B), 255})
			}
		}
		var b bytes.Buffer
		baseError = png.Encode(&b, pausedImage)
		pausedPNG = b.Bytes()
	})
	if baseError != nil {
		return nil, fmt.Errorf("turntable image could not be decoded")
	}
	r := &Renderer{base: baseImage, basePNG: basePNG}
	switch appearance {
	case Playing:
	case Paused:
		r.base, r.basePNG = pausedImage, pausedPNG
	case Stopped:
		r.base, r.basePNG = stoppedImage, stoppedPNG
	default:
		return nil, fmt.Errorf("invalid turntable state")
	}
	if appearance == Stopped || !cover.Valid() {
		return r, nil
	}
	var source image.Image
	if len(cover.Original) > 0 {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(cover.Original))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > 16<<20 {
			return nil, fmt.Errorf("invalid album artwork dimensions")
		}
		source, _, err = image.Decode(bytes.NewReader(cover.Original))
		if err != nil {
			return nil, fmt.Errorf("album artwork could not be decoded")
		}
	} else {
		im := image.NewRGBA(image.Rect(0, 0, cover.Width, cover.Height))
		for y := 0; y < cover.Height; y++ {
			for x := 0; x < cover.Width; x++ {
				i := (y*cover.Width + x) * 3
				im.SetRGBA(x, y, color.RGBA{cover.RGB[i], cover.RGB[i+1], cover.RGB[i+2], 255})
			}
		}
		source = im
	}
	r.cover = image.NewRGBA(image.Rect(0, 0, CoverSide, CoverSide))
	r.mask = make([]uint8, CoverSide*CoverSide)
	side := float64(min(source.Bounds().Dx(), source.Bounds().Dy()))
	left := float64(source.Bounds().Min.X) + (float64(source.Bounds().Dx())-side)/2
	top := float64(source.Bounds().Min.Y) + (float64(source.Bounds().Dy())-side)/2
	box := CoverBounds()
	for y := 0; y < CoverSide; y++ {
		for x := 0; x < CoverSide; x++ {
			r.cover.SetRGBA(x, y, sample(source, left+(float64(x)+.5)*side/CoverSide-.5, top+(float64(y)+.5)*side/CoverSide-.5))
			coverage := math.Max(0, math.Min(1, float64(CoverSide)/2+.5-math.Hypot(float64(x)+.5-CoverSide/2, float64(y)+.5-CoverSide/2)))
			coverage *= 1 - tonearmCoverage(float64(box.Min.X+x)+.5, float64(box.Min.Y+y)+.5)
			r.mask[y*CoverSide+x] = uint8(math.Round(coverage * 255))
		}
	}
	return r, nil
}

// WithState reuses the decoded cover across play/pause changes. Its returned
// renderer is immutable, so an already running frame can safely finish.
func (r *Renderer) WithState(appearance Appearance) *Renderer {
	next := *r
	switch appearance {
	case Playing:
		next.base, next.basePNG = baseImage, basePNG
	case Paused:
		next.base, next.basePNG = pausedImage, pausedPNG
	case Stopped:
		next.base, next.basePNG = stoppedImage, stoppedPNG
		next.cover, next.mask = nil, nil
	}
	return &next
}

func Compose(cover artwork.Image) (Scene, error) {
	r, err := NewRenderer(cover)
	if err != nil {
		return Scene{}, err
	}
	return r.Render(0, false)
}

// Image samples a rotated scene lazily for text terminals. Their few hundred
// half-block pixels do not need a full-resolution PNG encoded at every tick.
func (r *Renderer) Image(angle float64) image.Image {
	s, c := math.Sincos(angle)
	return rotatedImage{renderer: r, sin: s, cos: c}
}

type rotatedImage struct {
	renderer *Renderer
	sin, cos float64
}

func (im rotatedImage) ColorModel() color.Model { return color.RGBAModel }
func (im rotatedImage) Bounds() image.Rectangle { return im.renderer.base.Bounds() }
func (im rotatedImage) At(x, y int) color.Color {
	old := im.renderer.base.RGBAAt(x, y)
	box := CoverBounds()
	if im.renderer.cover == nil || !image.Pt(x, y).In(box) {
		return old
	}
	ix, iy := x-box.Min.X, y-box.Min.Y
	mask := uint32(im.renderer.mask[iy*CoverSide+ix])
	if mask == 0 {
		return old
	}
	dx, dy := float64(ix)+.5-CoverSide/2, float64(iy)+.5-CoverSide/2
	source := sampleRGBA(im.renderer.cover, im.cos*dx+im.sin*dy+CoverSide/2-.5, -im.sin*dx+im.cos*dy+CoverSide/2-.5)
	a := (uint32(source.A)*mask + 127) / 255
	return color.RGBA{uint8((uint32(source.R)*mask + uint32(old.R)*(255-a) + 127) / 255), uint8((uint32(source.G)*mask + uint32(old.G)*(255-a) + 127) / 255), uint8((uint32(source.B)*mask + uint32(old.B)*(255-a) + 127) / 255), 255}
}

// Render rotates clockwise in image space. A patch encodes just the changed
// rectangle; Pixels always contains the complete, consistently aligned scene.
func (r *Renderer) Render(angle float64, patch bool) (Scene, error) {
	if r.cover == nil {
		return Scene{r.basePNG, r.base}, nil
	}
	out := image.NewRGBA(r.base.Bounds())
	copy(out.Pix, r.base.Pix)
	box := CoverBounds()
	s, c := math.Sincos(angle)
	radius := float64(CoverSide) / 2
	for y := 0; y < CoverSide; y++ {
		dx, dy := .5-radius, float64(y)+.5-radius
		sx, sy := c*dx+s*dy+radius-.5, -s*dx+c*dy+radius-.5
		for x := 0; x < CoverSide; x++ {
			mask := uint32(r.mask[y*CoverSide+x])
			if mask != 0 {
				v := sampleRGBA(r.cover, sx, sy)
				a := (uint32(v.A)*mask + 127) / 255
				i := out.PixOffset(box.Min.X+x, box.Min.Y+y)
				for channel, value := range [3]uint8{v.R, v.G, v.B} {
					out.Pix[i+channel] = uint8((uint32(value)*mask + uint32(out.Pix[i+channel])*(255-a) + 127) / 255)
				}
			}
			sx += c
			sy -= s
		}
	}
	var im image.Image = out
	if patch {
		im = out.SubImage(box)
	}
	var encoded bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&encoded, im); err != nil {
		return Scene{}, fmt.Errorf("turntable image could not be encoded")
	}
	return Scene{encoded.Bytes(), out}, nil
}

// Preserve the photographed headshell and its shadow above the enlarged cover.
// The polygon follows the fixed foreground, rather than a rotating bitmap.
func tonearmCoverage(x, y float64) float64 {
	polygon := [...]image.Point{{941, 634}, {978, 674}, {957, 704}, {996, 757}, {987, 768}, {944, 721}, {906, 772}, {884, 766}, {839, 736}, {831, 715}}
	inside := false
	distance := math.Inf(1)
	for i, a := range polygon {
		b := polygon[(i+1)%len(polygon)]
		ax, ay, bx, by := float64(a.X), float64(a.Y), float64(b.X), float64(b.Y)
		if (ay > y) != (by > y) && x < (bx-ax)*(y-ay)/(by-ay)+ax {
			inside = !inside
		}
		vx, vy := bx-ax, by-ay
		t := math.Max(0, math.Min(1, ((x-ax)*vx+(y-ay)*vy)/(vx*vx+vy*vy)))
		distance = math.Min(distance, math.Hypot(x-ax-t*vx, y-ay-t*vy))
	}
	if inside {
		return math.Min(1, .5+distance)
	}
	return math.Max(0, .5-distance)
}

func sampleRGBA(im *image.RGBA, x, y float64) color.RGBA {
	x = math.Max(0, math.Min(float64(im.Rect.Dx()-1), x))
	y = math.Max(0, math.Min(float64(im.Rect.Dy()-1), y))
	x0, y0 := int(x), int(y)
	x1, y1 := min(x0+1, im.Rect.Dx()-1), min(y0+1, im.Rect.Dy()-1)
	fx, fy := x-float64(x0), y-float64(y0)
	a, b, c, d := im.PixOffset(x0, y0), im.PixOffset(x1, y0), im.PixOffset(x0, y1), im.PixOffset(x1, y1)
	var result [4]uint8
	for i := range result {
		top := float64(im.Pix[a+i])*(1-fx) + float64(im.Pix[b+i])*fx
		bottom := float64(im.Pix[c+i])*(1-fx) + float64(im.Pix[d+i])*fx
		result[i] = uint8(top*(1-fy) + bottom*fy + .5)
	}
	return color.RGBA{result[0], result[1], result[2], result[3]}
}

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
