package artwork

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
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

func TestDecodedArtworkRetainsOriginalResolutionAndBytes(t *testing.T) {
	for _, format := range []string{"png", "jpeg"} {
		var data bytes.Buffer
		im := image.NewRGBA(image.Rect(0, 0, 640, 480))
		if format == "png" {
			png.Encode(&data, im)
		} else {
			jpeg.Encode(&data, im, nil)
		}
		cover, err := Decode(bytes.NewReader(data.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(cover.Original))
		if err != nil || cfg.Width != 640 || cfg.Height != 480 {
			t.Fatal("native art was reduced to thumbnail resolution")
		}
		if !bytes.Equal(cover.Original, data.Bytes()) {
			t.Fatal("original encoded image was modified")
		}
		if cover.Width != Size || cover.Height != Size {
			t.Fatal("text fallback missing")
		}
	}
}
