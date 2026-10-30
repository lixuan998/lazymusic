package tui

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi/kitty"
	"lazymusic/internal/artwork"
	"lazymusic/internal/music"
	"lazymusic/internal/termimage"
)

func nativeArtModel(t *testing.T) *Model {
	t.Helper()
	m, _ := testModel()
	m.UI.Visualizer = turntableView
	m.State.Playback.Track = &music.Track{ID: "song"}
	m.artwork.key = "song"
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 600, 600)))
	im, err := artwork.Decode(&b)
	if err != nil {
		t.Fatal(err)
	}
	m.artwork.image = im
	m.graphics = graphicsState{id: termimage.MinID, known: true, queried: true, replied: true, target: "song"}
	return m
}

func finishGraphicsWrite(t *testing.T, m *Model, msg any) string {
	t.Helper()
	data, done, ok := m.TerminalOutput(msg)
	if !ok {
		t.Fatal("missing guarded terminal output")
	}
	m.graphicsMessage(done)
	return data
}

func TestNativeArtworkUploadsOnceResizesWithoutResendingAndCleansOwnID(t *testing.T) {
	m := nativeArtModel(t)
	prepare := m.syncGraphics()
	m.graphicsMessage(prepare())
	write := m.syncGraphics()
	if write == nil || m.syncGraphics() != nil {
		t.Fatal("duplicate native upload")
	}
	data := finishGraphicsWrite(t, m, write())
	if !strings.Contains(data, "a=T,U=1,f=100") || !m.graphics.active {
		t.Fatal("original image not uploaded")
	}
	if m.syncGraphics() != nil {
		t.Fatal("animation retransmitted unchanged image")
	}
	frame := strings.Join(m.turntableRows(m.navWidth()-4, m.bodyHeight()-m.navHeight()-2), "\n")
	if !strings.ContainsRune(frame, kitty.Placeholder) {
		t.Fatal("original image absent from the record")
	}
	m.SetSize(160, 50)
	m.UI.NavWidth = 36
	resize := m.syncGraphics()
	if resize == nil {
		t.Fatal("larger window did not resize the native image")
	}
	data = finishGraphicsWrite(t, m, resize())
	if !strings.Contains(data, "a=p,U=1") || strings.Contains(data, "f=100") {
		t.Fatal("resize retransmitted PNG instead of replacing its placement")
	}
	m.UI.Layout = "queue"
	cleanup := m.syncGraphics()
	data = finishGraphicsWrite(t, m, cleanup())
	if !strings.Contains(data, "a=d,d=I,i=") || m.graphics.active {
		t.Fatal("hidden image was not cleaned")
	}
}

func TestUnsupportedTerminalFallsBackAndLateUploadCannotRestoreImage(t *testing.T) {
	m := nativeArtModel(t)
	m.graphics.replied = false
	m.graphicsMessage(uv.KittyGraphicsEvent{Options: kitty.Options{ID: m.graphics.id}, Payload: []byte("ENOTSUP")})
	if m.syncGraphics() != nil || m.graphics.ready() {
		t.Fatal("unsupported terminal received artwork")
	}
	frame := strings.Join(m.turntableRows(24, 18), "\n")
	if strings.ContainsRune(frame, kitty.Placeholder) || !strings.Contains(frame, "PIXEL") {
		t.Fatal("text image fallback missing")
	}
	m.graphics.replied = true
	m.graphicsMessage(m.syncGraphics()())
	write := m.syncGraphics()()
	m.ClearGraphics()
	if _, _, ok := m.TerminalOutput(write); ok {
		t.Fatal("stale image command survived cancellation")
	}
}
