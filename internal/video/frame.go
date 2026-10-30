// Package video defines bounded playback pictures independently of sources and terminals.
package video

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"sync"
	"time"
)

const FPS = 30
const MaxWidth = 1280
const MaxHeight = 720
const Interval = time.Second / FPS
const MaxBytes = 6 << 20

type Frame struct {
	Source        string        `json:"source"`
	ID            string        `json:"id"`
	Position      time.Duration `json:"position_ns"`
	Width, Height int
	PNG           []byte `json:"png,omitempty"`
}

func (f Frame) Valid() bool {
	if len(f.PNG) == 0 || len(f.PNG) > MaxBytes {
		return false
	}
	c, format, err := image.DecodeConfig(bytes.NewReader(f.PNG))
	return err == nil && format == "png" && c.Width == f.Width && c.Height == f.Height && f.Width > 0 && f.Height > 0 && f.Width <= MaxWidth && f.Height <= MaxHeight
}

type Provider interface {
	VideoFrame(context.Context, string) (Frame, error)
}

func Encode(pixels []byte, width, height int, position time.Duration) (Frame, error) {
	if width < 1 || height < 1 || width > MaxWidth || height > MaxHeight || len(pixels) != width*height*4 {
		return Frame{}, fmt.Errorf("invalid video output dimensions")
	}
	picture := &image.RGBA{Pix: pixels, Stride: width * 4, Rect: image.Rect(0, 0, width, height)}
	var output bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed, BufferPool: &pngBuffers}
	if err := encoder.Encode(&output, picture); err != nil {
		return Frame{}, fmt.Errorf("video picture encoding failed")
	}
	if output.Len() > MaxBytes {
		return Frame{}, fmt.Errorf("video picture exceeds size limit")
	}
	return Frame{PNG: output.Bytes(), Width: width, Height: height, Position: position}, nil
}

// Reuse PNG filter/deflate workspaces rather than allocating them for every
// picture. Returned image bytes remain independently owned by the caller.
type pngBufferPool struct{ pool sync.Pool }

func (p *pngBufferPool) Get() *png.EncoderBuffer {
	if b := p.pool.Get(); b != nil {
		return b.(*png.EncoderBuffer)
	}
	return new(png.EncoderBuffer)
}
func (p *pngBufferPool) Put(b *png.EncoderBuffer) { p.pool.Put(b) }

var pngBuffers pngBufferPool
