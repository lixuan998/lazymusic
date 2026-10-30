package artwork

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestArtworkIsBoundedAndCenterCropped(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 300, 100))
	for y := 0; y < 100; y++ {
		for x := 100; x < 200; x++ {
			im.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var b bytes.Buffer
	png.Encode(&b, im)
	out, err := Decode(&b)
	if err != nil || !out.Valid() || out.RGB[0] != 255 || out.RGB[len(out.RGB)-3] != 255 {
		t.Fatal("cover was not cropped and bounded correctly")
	}
	if _, err = Decode(bytes.NewReader(make([]byte, MaxBytes+1))); err == nil {
		t.Fatal("accepted excessive input")
	}
	if _, err = Decode(bytes.NewBufferString("not an image")); err == nil {
		t.Fatal("accepted invalid input")
	}
	var large bytes.Buffer
	png.Encode(&large, image.NewGray(image.Rect(0, 0, 5000, 1)))
	if _, err = Decode(&large); err == nil {
		t.Fatal("accepted excessive dimensions")
	}
}
