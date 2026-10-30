package turntable

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"

	"lazymusic/internal/artwork"
)

func TestCircularCoverIsCenteredInThePhotograph(t *testing.T) {
	for _, size := range []image.Point{{300, 300}, {640, 480}, {480, 640}} {
		original := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
		for y := 0; y < size.Y; y++ {
			for x := 0; x < size.X; x++ {
				original.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
			}
		}
		var encoded bytes.Buffer
		png.Encode(&encoded, original)
		cover, err := artwork.Decode(&encoded)
		if err != nil {
			t.Fatal(err)
		}
		scene, err := Compose(cover)
		if err != nil {
			t.Fatal(err)
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(scene.PNG))
		if err != nil || cfg.Width != Width || cfg.Height != Height {
			t.Fatal("photo lost its original dimensions")
		}
		minX, minY, maxX, maxY := Width, Height, 0, 0
		for y := CenterY - CoverSide; y < CenterY+CoverSide; y++ {
			for x := CenterX - CoverSide; x < CenterX+CoverSide; x++ {
				r, g, b, _ := scene.Pixels.At(x, y).RGBA()
				if r == 65535 && g == 0 && b == 0 {
					minX = min(minX, x)
					maxX = max(maxX, x)
					minY = min(minY, y)
					maxY = max(maxY, y)
				}
			}
		}
		if abs(minX+maxX+1-CenterX*2) > 1 || abs(minY+maxY+1-CenterY*2) > 1 {
			t.Fatalf("cover drifted from platter center: %d,%d-%d,%d", minX, minY, maxX, maxY)
		}
		if abs((maxX-minX)-(maxY-minY)) > 1 {
			t.Fatal("round cover was stretched")
		}
		before, _ := Compose(artwork.Image{})
		if before.Pixels.At(CenterX-CoverSide/2+4, CenterY-CoverSide/2+4) != scene.Pixels.At(CenterX-CoverSide/2+4, CenterY-CoverSide/2+4) {
			t.Fatal("cover was not clipped to a circle")
		}
		if before.Pixels.At(1200, 100) != scene.Pixels.At(1200, 100) {
			t.Fatal("album compositing changed the tonearm or chassis")
		}
	}
}

func TestCircularCoverUsesTheCenterCropWithoutStretching(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 600, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 600; x++ {
			c := color.RGBA{G: 255, A: 255}
			if x >= 150 && x < 450 {
				c = color.RGBA{R: 255, A: 255}
			}
			source.SetRGBA(x, y, c)
		}
	}
	var data bytes.Buffer
	png.Encode(&data, source)
	cover, _ := artwork.Decode(&data)
	scene, err := Compose(cover)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []image.Point{{CenterX - 120, CenterY}, {CenterX + 120, CenterY}, {CenterX, CenterY - 120}, {CenterX, CenterY + 120}} {
		r, g, b, _ := scene.Pixels.At(p.X, p.Y).RGBA()
		if r != 65535 || g != 0 || b != 0 {
			t.Fatal("side borders entered the round crop")
		}
	}
}
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func quadrantCover(t testing.TB) artwork.Image {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 400, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 400; x++ {
			c := color.RGBA{R: 220, G: 30, B: 30, A: 255}
			if x >= 200 {
				c = color.RGBA{R: 30, G: 200, B: 40, A: 255}
			}
			im.SetRGBA(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	result, err := artwork.Decode(&b)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestRotationMovesOnlyTheLargeCoverAndPreservesTonearm(t *testing.T) {
	renderer, err := NewRenderer(quadrantCover(t))
	if err != nil {
		t.Fatal(err)
	}
	first, err := renderer.Render(0, false)
	if err != nil {
		t.Fatal(err)
	}
	quarter, err := renderer.Render(math.Pi/2, true)
	if err != nil {
		t.Fatal(err)
	}
	// A green point on the right moves to the bottom in a clockwise quarter turn.
	r, g, _, _ := quarter.Pixels.At(CenterX, CenterY+200).RGBA()
	if g <= r {
		t.Fatal("cover did not rotate clockwise")
	}
	r, g, _, _ = quarter.Pixels.At(CenterX, CenterY-200).RGBA()
	if r <= g {
		t.Fatal("rotation did not preserve the crop")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(quarter.PNG))
	if err != nil || cfg.Width != CoverSide || cfg.Height != CoverSide {
		t.Fatal("animation resent the whole chassis")
	}
	empty, _ := Compose(artwork.Image{})
	for _, p := range []image.Point{{100, 100}, {CenterX - 425, CenterY}, {900, 700}} {
		if first.Pixels.At(p.X, p.Y) != empty.Pixels.At(p.X, p.Y) || quarter.Pixels.At(p.X, p.Y) != empty.Pixels.At(p.X, p.Y) {
			t.Fatal("rotation changed the rim, tonearm or chassis", p)
		}
	}
	// The label fills over 90% of the black disc diameter.
	r, g, _, _ = first.Pixels.At(CenterX-395, CenterY).RGBA()
	if r <= g {
		t.Fatal("album cover is still a small central label")
	}
}

func BenchmarkTurntableRotation(b *testing.B) {
	renderer, err := NewRenderer(quadrantCover(b))
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := renderer.Render(.35, true); err != nil {
			b.Fatal(err)
		}
	}
}

func TestPixelFallbackSamplesTheSameRotatedSceneWithoutEncoding(t *testing.T) {
	r, err := NewRenderer(quadrantCover(t))
	if err != nil {
		t.Fatal(err)
	}
	scene, err := r.Render(.45, false)
	if err != nil {
		t.Fatal(err)
	}
	fallback := r.Image(.45)
	for _, p := range []image.Point{{100, 100}, {CenterX, CenterY}, {CenterX - 200, CenterY + 100}, {900, 700}} {
		a, b := color.RGBAModel.Convert(scene.Pixels.At(p.X, p.Y)).(color.RGBA), color.RGBAModel.Convert(fallback.At(p.X, p.Y)).(color.RGBA)
		if abs(int(a.R)-int(b.R)) > 1 || abs(int(a.G)-int(b.G)) > 1 || abs(int(a.B)-int(b.B)) > 1 {
			t.Fatal("fallback changed the scene", p)
		}
	}
}

func BenchmarkTurntablePhotographicRotation(b *testing.B) {
	cover, err := artwork.Decode(bytes.NewReader(basePNG))
	if err != nil {
		b.Fatal(err)
	}
	renderer, err := NewRenderer(cover)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		frame, err := renderer.Render(.35, true)
		if err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(float64(len(frame.PNG)), "bytes/frame")
	}
}

func TestTurntableNeedleAndLampFollowPlaybackState(t *testing.T) {
	renderer, err := NewRenderer(quadrantCover(t))
	if err != nil {
		t.Fatal(err)
	}
	playing := renderer.Image(.3)
	paused := renderer.WithState(Paused).Image(.3)
	stopped := renderer.WithState(Stopped).Image(0)
	// Playing/paused cover and the lowered cartridge stay aligned and identical.
	for _, p := range []image.Point{{CenterX - 100, CenterY}, {900, 700}} {
		if playing.At(p.X, p.Y) != paused.At(p.X, p.Y) {
			t.Fatal("pause moved the needle or cover")
		}
	}
	brightness := func(im image.Image) uint32 { r, g, b, _ := im.At(1225, 731).RGBA(); return r + g + b }
	if brightness(playing) <= brightness(paused)*2 || brightness(playing) <= brightness(stopped)*2 {
		t.Fatal("indicator stays lit while paused/stopped")
	}
	r, g, b, _ := stopped.At(CenterX-100, CenterY).RGBA()
	if r > g*2 || g > r*2 || b > r*2 {
		t.Fatal("stop retained the colored cover")
	}
	changes := 0
	for y := 650; y < 750; y++ {
		for x := 850; x < 950; x++ {
			if playing.At(x, y) != stopped.At(x, y) {
				changes++
			}
		}
	}
	if changes < 5000 {
		t.Fatal("stopped needle was not lifted away from the disc")
	}
}
