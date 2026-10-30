// Package artwork retains original album art and a terminal text fallback.
package artwork

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
)

const Size = 96
const MaxBytes = 4 << 20

// Image crosses IPC as encoded original bytes plus a small RGB fallback,
// never as a remote URL or an arbitrary file path.
type Image struct {
	Original []byte `json:"original,omitempty"`
	Format   string `json:"format,omitempty"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	RGB      []byte `json:"rgb,omitempty"`
}

func (im Image) Valid() bool {
	return len(im.Original) <= MaxBytes && (len(im.Original) == 0 || im.Format == "png" || im.Format == "jpeg") &&
		im.Width > 0 && im.Height > 0 && im.Width <= Size && im.Height <= Size && len(im.RGB) == im.Width*im.Height*3
}

func Decode(r io.Reader) (Image, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil || len(data) > MaxBytes {
		return Image{}, fmt.Errorf("album artwork exceeds the download limit or could not be read")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > 16<<20 {
		return Image{}, fmt.Errorf("album artwork has an unsupported format or size")
	}
	im, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Image{}, fmt.Errorf("album artwork could not be decoded")
	}
	out := Thumbnail(im)
	out.Original, out.Format = data, format
	return out, nil
}

// PNG preserves the original resolution. JPEG needs a lossless format
// conversion because the terminal graphics protocol accepts PNG, not JPEG.
func (im Image) PNG() ([]byte, error) {
	if len(im.Original) == 0 || len(im.Original) > MaxBytes {
		return nil, fmt.Errorf("original album artwork is unavailable")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(im.Original))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > 16<<20 {
		return nil, fmt.Errorf("invalid original album artwork")
	}
	if format == "png" {
		return im.Original, nil
	}
	if format != "jpeg" {
		return nil, fmt.Errorf("unsupported original album artwork format")
	}
	source, _, err := image.Decode(bytes.NewReader(im.Original))
	if err != nil {
		return nil, fmt.Errorf("original album artwork could not be decoded")
	}
	var out bytes.Buffer
	if err := png.Encode(&out, source); err != nil {
		return nil, fmt.Errorf("original album artwork could not be encoded")
	}
	return out.Bytes(), nil
}

// Thumbnail center-crops and averages the source into a square cover. Alpha is
// composited on the same dark surface as the record, avoiding bright fringes.
func Thumbnail(im image.Image) Image {
	b := im.Bounds()
	side := min(b.Dx(), b.Dy())
	if side <= 0 {
		return Image{}
	}
	left, top := b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2
	out := Image{Width: Size, Height: Size, RGB: make([]byte, Size*Size*3)}
	for y := 0; y < Size; y++ {
		for x := 0; x < Size; x++ {
			x0, y0 := x*side/Size, y*side/Size
			x1, y1 := min(side, max(x0+1, (x+1)*side/Size)), min(side, max(y0+1, (y+1)*side/Size))
			var rr, gg, bb, count uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					r, g, b, a := im.At(left+sx, top+sy).RGBA()
					rr += uint64(r + (0xffff-a)*0x18/255)
					gg += uint64(g + (0xffff-a)*0x1c/255)
					bb += uint64(b + (0xffff-a)*0x26/255)
					count++
				}
			}
			i := (y*Size + x) * 3
			out.RGB[i], out.RGB[i+1], out.RGB[i+2] = byte(rr/count>>8), byte(gg/count>>8), byte(bb/count>>8)
		}
	}
	return out
}
