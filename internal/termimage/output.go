package termimage

import (
	"fmt"
	"image"
	"io"
	"strings"
	"sync"
)

// Layer belongs to one reserved text rectangle in this application's alternate
// screen. Data is an already encoded, immutable Sixel frame.
type Layer struct {
	Rect image.Rectangle
	Data string
}

// Output composites Sixel after Bubble Tea's text writes. Sixel has no image ID
// or Unicode placement: ordinary text redraws can erase its pixels. Replaying
// cached frames here keeps overlays intact without re-encoding or racing stdout.
// The terminal file descriptor remains available to Bubble Tea's TTY detection.
type Output struct {
	io.Writer
	mu            sync.Mutex
	layers, shown []Layer
	alt, closed   bool
}

func NewOutput(w io.Writer) *Output { return &Output{Writer: w} }
func (o *Output) Read(p []byte) (int, error) {
	if reader, ok := o.Writer.(io.Reader); ok {
		return reader.Read(p)
	}
	return 0, fmt.Errorf("terminal output is not readable")
}
func (o *Output) Fd() uintptr {
	if f, ok := o.Writer.(interface{ Fd() uintptr }); ok {
		return f.Fd()
	}
	return ^uintptr(0)
}
func (o *Output) SetLayers(layers []Layer) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return
	}
	o.layers = append(o.layers[:0], layers...)
}
func writeOutput(w io.Writer, s string) error {
	n, err := io.WriteString(w, s)
	if err == nil && n != len(s) {
		err = io.ErrShortWrite
	}
	return err
}
func clearLayer(r image.Rectangle) string {
	var b strings.Builder
	b.WriteString("\x1b7\x1b[0m")
	for y := r.Min.Y; y < r.Max.Y; y++ {
		fmt.Fprintf(&b, "\x1b[%d;%dH\x1b[%dX", y+1, r.Min.X+1, r.Dx())
	}
	b.WriteString("\x1b8")
	return b.String()
}
func (o *Output) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	s := string(p)
	exiting := strings.Contains(s, "\x1b[?1049l") || strings.Contains(s, "\x1b[?1047l")
	if o.alt && !o.closed && !exiting {
		for _, old := range o.shown {
			keep := false
			for _, next := range o.layers {
				if old.Rect == next.Rect {
					keep = true
					break
				}
			}
			if !keep {
				if err := writeOutput(o.Writer, clearLayer(old.Rect)); err != nil {
					return 0, err
				}
			}
		}
	}
	wire := p
	const endSync = "[?2026l"
	synchronized := !exiting && !o.closed && len(o.layers) > 0 && strings.Contains(s, endSync)
	if synchronized {
		wire = []byte(strings.ReplaceAll(s, endSync, ""))
	}
	n, err := o.Writer.Write(wire)
	if err == nil && n != len(wire) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return 0, err
	}
	n = len(p)
	if exiting {
		o.alt = false
		o.shown = nil
		return n, nil
	}
	if strings.Contains(s, "\x1b[?1049h") || strings.Contains(s, "\x1b[?1047h") {
		o.alt = true
	}
	if !o.alt || o.closed {
		return n, nil
	}
	// Disable absolute screen-origin Sixel mode while drawing, and use private
	// color registers. Save/restore these modes as well as cursor and attributes.
	for _, layer := range o.layers {
		sequence := fmt.Sprintf("\x1b7\x1b[?80;1070s\x1b[?80l\x1b[?1070h\x1b[%d;%dH%s\x1b[?80;1070r\x1b8", layer.Rect.Min.Y+1, layer.Rect.Min.X+1, layer.Data)
		if err := writeOutput(o.Writer, sequence); err != nil {
			return n, err
		}
	}
	if synchronized {
		if err := writeOutput(o.Writer, endSync); err != nil {
			return n, err
		}
	}
	o.shown = append(o.shown[:0], o.layers...)
	return n, nil
}

// Close releases only owned rectangles, and never paints after leaving alt screen.
// It deliberately does not close stdout.
func (o *Output) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil
	}
	o.closed = true
	o.layers = nil
	if o.alt {
		for _, l := range o.shown {
			if err := writeOutput(o.Writer, clearLayer(l.Rect)); err != nil {
				return err
			}
		}
	}
	o.shown = nil
	return nil
}
