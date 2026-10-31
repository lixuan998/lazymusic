package termimage

import (
	"bytes"
	"errors"
	"image"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/term"
)

var _ term.File = (*Output)(nil)

func TestSixelOutputRepaintsAfterTextAndClearsOnlyOwnedRectangles(t *testing.T) {
	var b bytes.Buffer
	o := NewOutput(&b)
	a := Layer{image.Rect(2, 4, 8, 7), "\x1bPqIMAGE_A\x1b\\"}
	c := Layer{image.Rect(12, 4, 20, 8), "\x1bPqIMAGE_B\x1b\\"}
	o.SetLayers([]Layer{a, c})
	o.Write([]byte("before TUI"))
	if strings.Contains(b.String(), "IMAGE_") {
		t.Fatal("painted outside alt screen")
	}
	b.Reset()
	o.Write([]byte("\x1b[?1049hTEXT"))
	got := b.String()
	if strings.Index(got, "TEXT") > strings.Index(got, "IMAGE_A") || !strings.Contains(got, "\x1b[5;3H") || !strings.Contains(got, "IMAGE_B") {
		t.Fatal("incorrect overlay order or origin")
	}
	b.Reset()
	o.Write([]byte("REDRAW"))
	if !strings.Contains(b.String(), "IMAGE_A") || !strings.Contains(b.String(), "IMAGE_B") {
		t.Fatal("text redraw erased images")
	}
	b.Reset()
	o.SetLayers([]Layer{c})
	o.Write([]byte("MODAL"))
	got = b.String()
	if !strings.Contains(got, "\x1b[5;3H\x1b[6X") || strings.Contains(got, "\x1b[5;13H\x1b[8X") || strings.Contains(got, "IMAGE_A") || !strings.Contains(got, "IMAGE_B") || strings.Contains(got, "\x1b[2J") {
		t.Fatal("clear escaped the removed image's rectangle")
	}
	if strings.Index(got, "\x1b[6X") > strings.Index(got, "MODAL") {
		t.Fatal("clear erased newly rendered text")
	}
	b.Reset()
	o.Write([]byte("\x1b[?1049l"))
	o.Close()
	o.SetLayers([]Layer{a})
	o.Write([]byte("shell"))
	if strings.Contains(b.String(), "IMAGE_") || strings.Contains(b.String(), "X") {
		t.Fatal("exit restored stale overlays")
	}
}
func TestSixelOutputSnapshotsLayersAndPreservesTerminalHandle(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	o := NewOutput(f)
	if o.Fd() != f.Fd() {
		t.Fatal("TTY handle lost")
	}
	layers := []Layer{{image.Rect(0, 0, 2, 2), "FRAME"}}
	o.SetLayers(layers)
	layers[0].Data = "STALE"
	if _, err := o.Write([]byte("\x1b[?1049h")); err != nil {
		t.Fatal(err)
	}
	o.Close()
	if _, err := f.WriteString("stdout still open"); err != nil {
		t.Fatal("output closed stdout")
	}
	data, err := os.ReadFile(f.Name())
	if err != nil || !bytes.Contains(data, []byte("FRAME")) || bytes.Contains(data, []byte("STALE")) {
		t.Fatalf("snapshot=%q %v", data, err)
	}
}

type failedOutput struct{ short bool }

func (w failedOutput) Write(p []byte) (int, error) {
	if w.short {
		return len(p) / 2, nil
	}
	return 0, errors.New("closed terminal")
}
func TestSixelOutputPropagatesWriteFailures(t *testing.T) {
	for _, short := range []bool{false, true} {
		o := NewOutput(failedOutput{short})
		if _, err := o.Write([]byte("text")); err == nil || short && !errors.Is(err, io.ErrShortWrite) {
			t.Fatal("output failure hidden", err)
		}
	}
}

func TestSixelRemainsInsideSynchronizedTextUpdate(t *testing.T) {
	var b bytes.Buffer
	o := NewOutput(&b)
	o.SetLayers([]Layer{{image.Rect(1, 1, 3, 3), "\x1bPqFRAME\x1b\\"}})
	input := "\x1b[?1049h\x1b[?2026hTEXT\x1b[?2026l"
	n, err := o.Write([]byte(input))
	if err != nil || n != len(input) {
		t.Fatal(n, err)
	}
	out := b.String()
	if strings.Count(out, "\x1b[?2026l") != 1 || !strings.HasSuffix(out, "\x1b[?2026l") || strings.Index(out, "FRAME") < strings.Index(out, "TEXT") {
		t.Fatal("Sixel escaped synchronized frame")
	}
}

func TestSixelOutputSerializesLayerChangesAndWrites(t *testing.T) {
	var b bytes.Buffer
	o := NewOutput(&b)
	o.Write([]byte("\x1b[?1049h"))
	var wg sync.WaitGroup
	for n := 0; n < 4; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 32; i++ {
				o.SetLayers([]Layer{{image.Rect(2, 2, 4, 4), "\x1bPqFRAME\x1b\\"}})
				o.Write([]byte("TEXT"))
				o.SetLayers(nil)
			}
		}()
	}
	wg.Wait()
	o.Close()
	if !strings.Contains(b.String(), "TEXT") {
		t.Fatal("concurrent output lost")
	}
}
