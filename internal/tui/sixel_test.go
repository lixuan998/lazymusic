package tui

import (
	"image"
	"strings"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"lazymusic/internal/artwork"
	"lazymusic/internal/termimage"
	"lazymusic/internal/turntable"
)

func enableSixel(m *Model) {
	m.graphics.protocol = "sixel"
	m.graphics.requested = "sixel"
	m.graphics.replied, m.graphics.known, m.graphics.disabled = true, true, false
	m.graphics.cellWidth, m.graphics.cellHeight = 8, 16
}
func finishSixel(t *testing.T, m *Model) {
	t.Helper()
	for step := 0; step < 4; step++ {
		command := m.syncGraphics()
		if command == nil {
			return
		}
		msg := command()
		if _, ok := msg.(graphicsPrepared); ok {
			m.graphicsMessage(msg)
			continue
		}
		if _, ok := msg.(graphicsWrite); ok {
			data := finishGraphicsWrite(t, m, msg)
			if data != termimage.SixelRefresh || strings.Contains(data, "_G") {
				t.Fatal("Sixel used Kitty transport")
			}
			continue
		}
		t.Fatalf("unexpected Sixel message %T", msg)
	}
	t.Fatal("Sixel work did not settle")
}
func TestSixelProbeNeedsPositiveAttributesAndMeasuredPixels(t *testing.T) {
	m := nativeArtModel(t)
	g := &m.graphics
	g.replied, g.queried = false, false
	query := m.syncGraphicsProbe()()
	data, done, ok := m.TerminalOutput(query)
	if !ok || !strings.Contains(data, termimage.SixelQuery) || !strings.Contains(data, "a=q") {
		t.Fatal("missing parallel capability probes")
	}
	m.graphicsMessage(done)
	m.graphicsMessage(uv.PrimaryDeviceAttributesEvent{62, 22})
	if g.ready() {
		t.Fatal("DA without Sixel enabled images")
	}
	m.graphicsMessage(uv.PrimaryDeviceAttributesEvent{62, 4, 22})
	if g.ready() || g.protocol != "sixel" {
		t.Fatal("missing pixel metrics were guessed")
	}
	m.graphicsMessage(uv.CellSizeEvent{Width: 9, Height: 21})
	if !g.ready() {
		t.Fatal("verified Sixel did not become ready")
	}
	m.graphicsMessage(graphicsTimeout{g.queryGeneration})
	if !g.ready() {
		t.Fatal("late timeout revoked support")
	}
	// Auto selection prefers a later successful Kitty response and cancels the
	// old transport's queued frames without carrying them to the new backend.
	g.preparing = true
	generation := g.generation
	m.graphicsMessage(uv.KittyGraphicsEvent{Options: kitty.Options{ID: g.probeID()}, Payload: []byte("OK")})
	if g.protocol != "kitty" || !g.ready() || g.generation == generation || g.preparing {
		t.Fatal("late Kitty support mixed image transports")
	}
}
func TestSixelPixelSizeFallbackAndOverrides(t *testing.T) {
	m := nativeArtModel(t)
	m.graphics.replied = false
	m.graphics.queried = true
	m.graphicsMessage(uv.PrimaryDeviceAttributesEvent{62, 4})
	m.graphicsMessage(uv.PixelSizeEvent{Width: m.width * 8, Height: m.height * 18})
	if !m.graphics.ready() || m.graphics.cellHeight != 18 {
		t.Fatal("window pixel report not used")
	}
	m.graphics.requested = "sixel"
	m.graphicsMessage(uv.KittyGraphicsEvent{Options: kitty.Options{ID: m.graphics.probeID()}, Payload: []byte("OK")})
	if m.graphics.protocol != "sixel" {
		t.Fatal("explicit transport ignored")
	}
	for _, env := range []string{"NVIM", "TMUX", "ZELLIJ", "LAZYMUSIC_IMAGE_DISABLE", "LAZYMUSIC_IMAGE_BRIDGE", "LAZYMUSIC_IMAGE_SUPPORTED"} {
		t.Setenv(env, "")
	}
	t.Setenv("LAZYMUSIC_IMAGE_PROTOCOL", "pixel")
	if g := newGraphicsState(); !g.disabled {
		t.Fatal("pixel override ignored")
	}
	t.Setenv("LAZYMUSIC_IMAGE_PROTOCOL", "sixel")
	t.Setenv("LAZYMUSIC_IMAGE_BRIDGE", "osc")
	t.Setenv("NVIM", "fixture")
	if g := newGraphicsState(); !g.disabled {
		t.Fatal("Sixel escaped into Kitty-only Neovim relay")
	}
}
func TestSixelTurntableLifecycleAndLayoutCoordinates(t *testing.T) {
	m := nativeArtModel(t)
	enableSixel(m)
	finishSixel(t, m)
	layers := m.SixelLayers()
	if len(layers) != 1 || !strings.HasPrefix(layers[0].Data, "\x1bP") {
		t.Fatal("Sixel photograph missing")
	}
	if strings.ContainsRune(m.View().Content, kitty.Placeholder) {
		t.Fatal("Sixel emitted Kitty placeholders")
	}
	if m.syncGraphics() != nil {
		t.Fatal("static photograph encoded twice")
	}
	for _, zen := range []bool{false, true} {
		for _, size := range []image.Point{{80, 40}, {120, 50}, {180, 60}} {
			m.UI.Zen = zen
			m.SetSize(size.X, size.Y)
			finishSixel(t, m)
			layers = m.SixelLayers()
			if len(layers) != 1 {
				t.Fatal("missing resized layer", zen, size)
			}
			frame := strings.Split(ansi.Strip(m.View().Content), "\n")
			rect := layers[0].Rect
			if !rect.In(image.Rect(0, 0, size.X, size.Y)) {
				t.Fatal("image outside screen")
			}
			for y := rect.Min.Y; y < rect.Max.Y; y++ {
				line := []rune(frame[y])
				for x := rect.Min.X; x < rect.Max.X; x++ {
					if line[x] != ' ' {
						t.Fatalf("image covers UI text at %d,%d (Zen=%v): %q", x, y, zen, line[x])
					}
				}
			}
		}
	}
	m.help = true
	if len(m.SixelLayers()) != 0 {
		t.Fatal("image covers modal")
	}
	pending := m.syncGraphics()()
	finishGraphicsWrite(t, m, pending)
	if m.graphics.active {
		t.Fatal("hidden image retained")
	}
	m.help = false
	m.UI.Zen = false
	finishSixel(t, m)
	m.record.angle = .3
	late := m.syncGraphics()()
	m.UI.Layout = "queue"
	m.syncGraphics()
	m.graphicsMessage(late)
	if len(m.SixelLayers()) != 0 || m.graphics.preparing {
		t.Fatal("late image reopened hidden panel")
	}
}
func TestSixelVideoHasOneEncodeInFlightAndKeepsPausedFrame(t *testing.T) {
	m, api := videoModel(t)
	enableSixel(m)
	finishSixel(t, m)
	m.videoMessage(m.syncVideo()())
	encode := m.syncVideo()
	if encode == nil || !m.video.encoding {
		t.Fatal("video not encoding")
	}
	if m.syncVideo() != nil || api.calls != 1 {
		t.Fatal("unbounded frame work")
	}
	prepared := encode()
	m.videoMessage(prepared)
	write := m.syncVideo()
	if write == nil {
		t.Fatal("no Sixel video output")
	}
	data, done, ok := m.TerminalOutput(write())
	if !ok || data != termimage.SixelRefresh {
		t.Fatal("wrong Sixel transport")
	}
	m.videoMessage(done)
	if len(m.SixelLayers()) != 2 {
		t.Fatal("video and turntable ownership not independent", len(m.SixelLayers()))
	}
	m.State.Playback.Playing = false
	m.State.Playback.Position = api.frame.Position
	m.video.ticking = false
	if m.syncVideo() != nil || len(m.SixelLayers()) == 0 {
		t.Fatal("pause lost/reloaded video")
	}
	m.UI.Video = false
	m.syncVideo()
	m.videoMessage(prepared)
	if m.video.active || m.video.encoding || m.video.sixelData != "" {
		t.Fatal("late frame survived hiding")
	}
	if m.videoInterval() != 100*time.Millisecond {
		t.Fatal("Sixel full-frame cadence unbounded")
	}
}
func TestSixelSourceChangeInheritsOnlyCapability(t *testing.T) {
	previous := nativeArtModel(t)
	enableSixel(previous)
	finishSixel(t, previous)
	next, _ := testModel()
	next.graphics.disabled = false
	next.InheritGraphicsSupport(previous)
	if !next.graphics.ready() || next.graphics.protocol != "sixel" || next.graphics.cellHeight != 16 || next.graphics.active || next.graphics.sixelData != "" {
		t.Fatal("source change mixed capability and image state")
	}
}

func TestSixelRecordBackpressurePauseAndStop(t *testing.T) {
	m := nativeArtModel(t)
	enableSixel(m)
	m.State.Playback.Playing = true
	m.State.Playback.Status = "playing"
	m.syncRecord(time.Now())
	finishSixel(t, m)
	m.record.angle = .1
	prepare := m.syncGraphics()
	if prepare == nil || m.syncGraphics() != nil {
		t.Fatal("multiple frame encoders started")
	}
	frame := prepare()
	m.record.angle = .2
	m.graphicsMessage(frame)
	write := m.syncGraphics()
	if write == nil {
		t.Fatal("slow prepared frame was starved")
	}
	finishGraphicsWrite(t, m, write())
	if m.graphics.sentAngle != .1 {
		t.Fatal("encoded frame confused with newer clock angle")
	}
	finishSixel(t, m)
	m.State.Playback.Playing = false
	m.State.Playback.Status = "paused"
	finishSixel(t, m)
	if m.record.angle != .2 || m.syncRecord(time.Now()) != nil || m.syncGraphics() != nil {
		t.Fatal("pause moved or re-encoded the frozen record")
	}
	m.State.Playback.Status = "stopped"
	m.syncRecord(time.Now())
	finishSixel(t, m)
	base, err := turntable.NewRendererState(artwork.Image{}, turntable.Stopped)
	if err != nil {
		t.Fatal(err)
	}
	if m.record.angle != 0 || m.graphics.scene.At(turntable.CenterX, turntable.CenterY) != base.Image(0).At(turntable.CenterX, turntable.CenterY) {
		t.Fatal("stop retained the album or angle")
	}
}
