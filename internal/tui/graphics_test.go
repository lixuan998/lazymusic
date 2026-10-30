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
	"lazymusic/internal/turntable"
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
	m.graphics = graphicsState{id: termimage.MinID, known: true, queried: true, replied: true, target: m.turntableSceneKey()}
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
	m.graphicsMessage(uv.KittyGraphicsEvent{Options: kitty.Options{ID: m.graphics.probeID()}, Payload: []byte("ENOTSUP")})
	prepare := m.syncGraphics()
	if prepare == nil || m.graphics.ready() {
		t.Fatal("unsupported terminal did not prepare its photographic fallback")
	}
	m.graphicsMessage(prepare())
	frame := strings.Join(m.turntableRows(24, 18), "\n")
	if strings.ContainsRune(frame, kitty.Placeholder) || !strings.ContainsRune(frame, '▀') {
		t.Fatal("text image fallback missing")
	}
	m.graphics.replied = true
	write := m.syncGraphics()()
	m.ClearGraphics()
	if _, _, ok := m.TerminalOutput(write); ok {
		t.Fatal("stale image command survived cancellation")
	}
}

func TestZenTurntableUsesItsOwnSizeAndReusesThePhotograph(t *testing.T) {
	m := nativeArtModel(t)
	m.graphicsMessage(m.syncGraphics()())
	finishGraphicsWrite(t, m, m.syncGraphics()())
	m.normal("ctrl+-")
	if !m.turntableVisible() {
		t.Fatal("native player is hidden in Zen")
	}
	write := m.syncGraphics()
	if write == nil {
		t.Fatal("Zen did not update the image dimensions")
	}
	data := finishGraphicsWrite(t, m, write())
	if !strings.Contains(data, "a=p,U=1") || strings.Contains(data, "f=100") {
		t.Fatal("Zen re-uploaded the same photograph")
	}
	view := m.View().Content
	if !strings.ContainsRune(view, kitty.Placeholder) || !strings.Contains(view, "Equalizer") {
		t.Fatal("Zen lost its image or EQ")
	}
	if m.rhythmVisible() {
		t.Fatal("static photograph started the waveform animation clock")
	}
}

func TestRecordFramesEditOneImageAndNeverAccumulatePendingWork(t *testing.T) {
	m := nativeArtModel(t)
	m.graphics.frameSupport = 1
	m.graphicsMessage(m.syncGraphics()())
	finishGraphicsWrite(t, m, m.syncGraphics()())
	m.record.angle = .1
	prepare := m.syncGraphics()
	if prepare == nil || m.syncGraphics() != nil {
		t.Fatal("duplicate frame preparation")
	}
	frame := prepare().(graphicsPrepared)
	if !frame.patch {
		t.Fatal("animation did not isolate the rotating disc")
	}
	// A slow renderer must upload the completed frame rather than endlessly
	// throwing it away when the animation clock has already reached a new angle.
	m.record.angle = .2
	m.graphicsMessage(frame)
	write := m.syncGraphics()
	if write == nil || m.syncGraphics() != nil {
		t.Fatal("frame upload missing or unbounded")
	}
	data := finishGraphicsWrite(t, m, write())
	if !strings.Contains(data, "a=f,i=") || strings.Contains(data, "a=T") || strings.Contains(data, "a=d") || m.graphics.sentAngle != .1 {
		t.Fatal("record frame replaced the image or was never displayed")
	}
	if m.syncGraphics() == nil {
		t.Fatal("latest angle was lost after a slow frame")
	}
}

func TestFrameProbeFailureKeepsNativeImagesAndStopRemovesCover(t *testing.T) {
	m := nativeArtModel(t)
	m.graphicsMessage(m.syncGraphics()())
	finishGraphicsWrite(t, m, m.syncGraphics()())
	m.graphics.probing = true
	m.graphicsMessage(uv.KittyGraphicsEvent{Options: kitty.Options{ID: m.graphics.id, Rows: 1}, Payload: []byte("ENOTSUP")})
	if !m.graphics.ready() || m.graphics.frameSupport != -1 {
		t.Fatal("missing animation support disabled native pictures")
	}
	m.record.angle = .1
	m.graphicsMessage(m.syncGraphics()())
	data := finishGraphicsWrite(t, m, m.syncGraphics()())
	if !strings.Contains(data, "a=T") {
		t.Fatal("old terminal did not receive the complete rotated image")
	}
	m.State.Playback.Status = "stopped"
	m.record.angle = 0
	cleanup := m.syncGraphics()
	finishGraphicsWrite(t, m, cleanup())
	m.graphicsMessage(m.syncGraphics()())
	renderer, _ := turntable.NewRendererState(artwork.Image{}, turntable.Stopped)
	base, _ := renderer.Render(0, false)
	got := m.graphics.scene.At(turntable.CenterX, turntable.CenterY)
	if got != base.Pixels.At(turntable.CenterX, turntable.CenterY) {
		t.Fatal("stop retained the cover")
	}
	stale := m.syncGraphics()()
	m.UI.Layout = "queue"
	m.syncGraphics()
	if _, _, ok := m.TerminalOutput(stale); ok {
		t.Fatal("hidden frame survived cancellation")
	}
}

func TestPauseChangesLampWithoutBlankingOrResettingTheRecord(t *testing.T) {
	m := nativeArtModel(t)
	m.graphics.frameSupport = 1
	m.State.Playback.Playing = true
	m.graphics.target = m.turntableSceneKey()
	m.graphicsMessage(m.syncGraphics()())
	finishGraphicsWrite(t, m, m.syncGraphics()())
	m.record.angle = .4
	m.graphicsMessage(m.syncGraphics()())
	finishGraphicsWrite(t, m, m.syncGraphics()())
	m.State.Playback.Playing = false
	m.State.Playback.Status = "paused"
	prepare := m.syncGraphics()
	if prepare == nil || !m.graphics.active || m.record.angle != .4 {
		t.Fatal("pause blanked or reset the record")
	}
	m.graphicsMessage(prepare())
	data := finishGraphicsWrite(t, m, m.syncGraphics()())
	if strings.Contains(data, "a=d") || !strings.Contains(data, "a=T") || m.graphics.sentAngle != .4 {
		t.Fatal("pause did not replace the lamp state at the frozen angle")
	}
}
