package tui

import (
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi/kitty"
	"lazymusic/internal/termimage"
)

func TestCachedArtworkCleanupCannotCancelTheCapabilityQuery(t *testing.T) {
	m := nativeArtModel(t)
	g := &m.graphics
	g.known, g.replied, g.queried = false, false, false
	query := m.syncGraphicsProbe()()
	// A cache hit or another song changes the scene before the query is even
	// emitted. The image generation changes, but the query must still be sent.
	m.ClearGraphics()
	data, done, ok := m.TerminalOutput(query)
	if !ok || !strings.Contains(data, "a=q,i=") || !strings.Contains(data, termimage.Query(g.probeID())) || g.probeID() == g.id {
		t.Fatal("artwork refresh canceled or shared the capability query")
	}
	m.graphicsMessage(done)
	g.used = true
	cleanup := m.ClearGraphics()
	cleared := finishGraphicsWrite(t, m, cleanup())
	if strings.Contains(cleared, termimage.Delete(g.probeID())) {
		t.Fatal("image cleanup removed the pending query's response routing")
	}
	m.graphicsMessage(uv.KittyGraphicsEvent{Options: kitty.Options{ID: g.probeID()}, Payload: []byte("OK")})
	if !g.ready() {
		t.Fatal("late terminal reply could not enable native pictures")
	}
	final := m.GraphicsCleanup()
	if !strings.Contains(final, termimage.Delete(g.id)) || !strings.Contains(final, termimage.Delete(g.probeID())) {
		t.Fatal("final cleanup did not release both owned IDs")
	}
}

func TestVerifiedCapabilitySurvivesDrawingErrorsAndLateTimeouts(t *testing.T) {
	m := nativeArtModel(t)
	g := &m.graphics
	g.used, g.frameSupport = true, 1
	// Some terminals omit r=1 in a frame error. It still belongs to drawing,
	// never to the dedicated capability query.
	m.graphicsMessage(uv.KittyGraphicsEvent{Options: kitty.Options{ID: g.id}, Payload: []byte("ENOENT")})
	if !g.ready() || g.frameSupport != -1 {
		t.Fatal("drawing error downgraded confirmed image support")
	}
	g.queryPending, g.queryTimed = true, true
	m.graphicsMessage(graphicsTimeout{g.queryGeneration})
	m.graphicsMessage(uv.KittyGraphicsEvent{Options: kitty.Options{ID: g.probeID()}, Payload: []byte("ENOTSUP")})
	if !g.ready() {
		t.Fatal("late negative response or timeout revoked positive proof")
	}
}

func TestLostQueriesRetryAndLateSuccessRecoversFromFallback(t *testing.T) {
	m := nativeArtModel(t)
	g := &m.graphics
	g.replied, g.queried = false, false
	for attempt := 1; attempt <= graphicsProbeAttempts; attempt++ {
		query := m.syncGraphicsProbe()
		if query == nil {
			t.Fatal("query was not retried", attempt)
		}
		msg := query()
		if m.syncGraphicsProbe() != nil || g.queryTimed {
			t.Fatal("query timeout started before terminal output")
		}
		_, done, ok := m.TerminalOutput(msg)
		if !ok {
			t.Fatal("query output missing")
		}
		m.graphicsMessage(done)
		if m.syncGraphicsProbe() == nil {
			t.Fatal("sent query has no response deadline")
		}
		m.graphicsMessage(graphicsTimeout{g.queryGeneration})
		if attempt < graphicsProbeAttempts && g.checked {
			t.Fatal("one lost reply permanently selected pixel rendering")
		}
	}
	if !g.checked || g.ready() || m.syncGraphicsProbe() != nil {
		t.Fatal("unsupported terminal did not settle after bounded retries")
	}
	m.graphicsMessage(uv.KittyGraphicsEvent{Options: kitty.Options{ID: g.probeID()}, Payload: []byte("OK")})
	if !g.ready() {
		t.Fatal("late success could not recover native rendering")
	}
}

func TestConfirmedNeovimTTYCanBeReusedButExplicitDisableWins(t *testing.T) {
	for _, name := range []string{"TMUX", "ZELLIJ", "TERM_PROGRAM", "LAZYMUSIC_IMAGE_HOST", "LAZYMUSIC_IMAGE_DISABLE"} {
		t.Setenv(name, "")
	}
	t.Setenv("NVIM", "test-nvim")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("LAZYMUSIC_IMAGE_BRIDGE", "osc")
	t.Setenv("LAZYMUSIC_IMAGE_SUPPORTED", "1")
	if g := newGraphicsState(); !g.ready() {
		t.Fatal("verified Neovim host was probed as an unknown terminal again")
	}
	t.Setenv("LAZYMUSIC_IMAGE_DISABLE", "1")
	if g := newGraphicsState(); g.ready() {
		t.Fatal("cached proof bypassed explicit pixel mode")
	}
	t.Setenv("LAZYMUSIC_IMAGE_DISABLE", "")
	t.Setenv("LAZYMUSIC_IMAGE_BRIDGE", "")
	if g := newGraphicsState(); g.ready() {
		t.Fatal("Neovim proof escaped its relay context")
	}
}

func TestSourceChangeInheritsSupportWithoutReusingImages(t *testing.T) {
	previous := nativeArtModel(t)
	next, _ := testModel()
	next.graphics.disabled = false
	next.graphics.id = termimage.MinID + 10
	next.InheritGraphicsSupport(previous)
	if !next.graphics.ready() || next.graphics.used || next.graphics.queried || next.graphics.id == previous.graphics.id {
		t.Fatal("source change lost support or inherited unrelated image storage")
	}
}
