package tui

import (
	"crypto/rand"
	"encoding/binary"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"lazymusic/internal/termimage"
)

type graphicsState struct {
	id                                                int
	disabled, relay, known, queried, replied, checked bool
	active, pending, preparing, clearing              bool
	generation                                        uint64
	target, prepared, sent, failed                    string
	png                                               []byte
	columns, rows                                     int
}
type graphicsTimeout struct{}
type graphicsCleared struct{ generation uint64 }
type graphicsWrite struct {
	generation uint64
	data       string
	done       tea.Msg
}
type graphicsPrepared struct {
	generation uint64
	key        string
	png        []byte
	err        error
}
type graphicsUploaded struct {
	generation    uint64
	key           string
	columns, rows int
}

func newGraphicsState() graphicsState {
	var nonce [2]byte
	_, _ = rand.Read(nonce[:])
	host := strings.ToLower(os.Getenv("LAZYMUSIC_IMAGE_HOST") + " " + os.Getenv("TERM_PROGRAM") + " " + os.Getenv("TERM"))
	relay := os.Getenv("LAZYMUSIC_IMAGE_BRIDGE") == "osc"
	return graphicsState{id: termimage.MinID + int(binary.BigEndian.Uint16(nonce[:]))%65535, relay: relay,
		known:    strings.Contains(host, "ghostty") || strings.Contains(host, "kitty"),
		disabled: os.Getenv("LAZYMUSIC_IMAGE_DISABLE") == "1" || os.Getenv("NVIM") != "" && !relay || os.Getenv("TMUX") != "" || os.Getenv("ZELLIJ") != ""}
}
func (g *graphicsState) ready() bool { return !g.disabled && g.replied && g.known }
func (g *graphicsState) wrap(s string) string {
	if g.relay {
		return termimage.Relay(s)
	}
	return s
}

// GraphicsCleanup is also emitted after the terminal program has stopped, so
// interrupted shutdown cannot leave this application's image in GPU storage.
func (m *Model) GraphicsCleanup() string {
	if !m.graphics.queried {
		return ""
	}
	return m.graphics.wrap(termimage.Delete(m.graphics.id))
}
func (m *Model) ClearGraphics() tea.Cmd {
	g := &m.graphics
	g.generation++
	g.active, g.pending, g.preparing = false, false, false
	g.sent = ""
	if cleanup := m.GraphicsCleanup(); cleanup != "" {
		g.clearing = true
		out := graphicsWrite{g.generation, cleanup, graphicsCleared{g.generation}}
		return func() tea.Msg { return out }
	}
	return nil
}

// TerminalOutput lets the containing application preserve both output order
// and its source epoch before handing raw graphics to the terminal renderer.
func (m *Model) TerminalOutput(msg tea.Msg) (string, tea.Msg, bool) {
	out, ok := msg.(graphicsWrite)
	if !ok || out.generation != m.graphics.generation {
		return "", nil, false
	}
	return out.data, out.done, true
}

func (m *Model) syncGraphics() tea.Cmd {
	g := &m.graphics
	if g.clearing {
		return nil
	}
	visible := m.turntableVisible() && m.State.Playback.Track != nil && m.artwork.image.Valid()
	if !visible {
		if g.active || g.pending || g.preparing {
			return m.ClearGraphics()
		}
		return nil
	}
	if g.disabled {
		return nil
	}
	if !g.queried {
		g.queried = true
		commands := []tea.Cmd{tea.Raw(g.wrap(termimage.Query(g.id))), tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg { return graphicsTimeout{} })}
		if !g.relay && !g.known {
			commands = append(commands, tea.Raw(ansi.RequestXTVersion))
		}
		return tea.Batch(commands...)
	}
	if !g.ready() {
		return nil
	}
	key := m.artwork.key
	if g.target != key {
		cleanup := m.ClearGraphics()
		g.target = key
		return cleanup
	}
	if g.failed == key || g.pending || g.preparing {
		return nil
	}
	if g.prepared != key {
		g.preparing = true
		generation, im := g.generation, m.artwork.image
		return func() tea.Msg { data, err := im.PNG(); return graphicsPrepared{generation, key, data, err} }
	}
	_, _, columns, rows := recordCoverRect(m.navWidth()-4, m.turntableGraphicHeight(m.bodyHeight()-m.navHeight()-2, m.navWidth()-4))
	if g.active && g.sent == key && g.columns == columns && g.rows == rows {
		return nil
	}
	sequence := termimage.Placement(g.id, columns, rows)
	if g.sent != key {
		sequence = termimage.Transmit(g.png, g.id, columns, rows)
	}
	g.pending = true
	generation := g.generation
	out := graphicsWrite{generation, g.wrap(sequence), graphicsUploaded{generation, key, columns, rows}}
	return func() tea.Msg { return out }
}

func (m *Model) graphicsMessage(msg tea.Msg) {
	g := &m.graphics
	switch msg := msg.(type) {
	case uv.KittyGraphicsEvent:
		if msg.Options.ID == g.id && g.queried {
			g.replied = string(msg.Payload) == "OK"
			g.checked = true
		}
	case tea.TerminalVersionMsg:
		name := strings.ToLower(msg.Name)
		if strings.Contains(name, "ghostty") || strings.Contains(name, "kitty") {
			g.known = true
		}
	case graphicsTimeout:
		g.checked = true
		if !g.ready() {
			m.Report("info", "graphics", "Terminal image protocol unavailable; using pixel artwork")
		}
	case graphicsCleared:
		if msg.generation == g.generation {
			g.clearing = false
		}
	case graphicsPrepared:
		if msg.generation != g.generation || msg.key != g.target || !g.preparing {
			return
		}
		g.preparing = false
		if msg.err != nil {
			g.failed = msg.key
			m.Report("info", "graphics", "Original artwork unavailable; using pixel artwork")
			return
		}
		g.prepared, g.png = msg.key, msg.png
	case graphicsUploaded:
		if msg.generation != g.generation || msg.key != g.target || !g.pending {
			return
		}
		g.pending, g.active = false, true
		g.sent, g.columns, g.rows = msg.key, msg.columns, msg.rows
	}
}
