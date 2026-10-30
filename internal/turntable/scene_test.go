package turntable

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
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
