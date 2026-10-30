package tui

import (
	"crypto/rand"
	"encoding/binary"
	"image"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"lazymusic/internal/artwork"
	"lazymusic/internal/termimage"
	"lazymusic/internal/turntable"
)

type graphicsState struct {
	id                                                int
	disabled, relay, known, queried, replied, checked bool
	active, pending, preparing, clearing              bool
	generation                                        uint64
	target, prepared, sent, failed                    string
	png                                               []byte
	scene                                             image.Image
	columns, rows                                     int
	coverKey                                          string
	renderer                                          *turntable.Renderer
	preparedAngle, sentAngle                          float64
	preparedPatch                                     bool
	frameSupport                                      int
	probing                                           bool
}
type graphicsTimeout struct{}
type graphicsFrameTimeout struct{ generation uint64 }
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
	scene      image.Image
	err        error
	renderer   *turntable.Renderer
	angle      float64
	patch      bool
	coverKey   string
}
type graphicsUploaded struct {
	generation    uint64
	key           string
	columns, rows int
	angle         float64
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
	g.active, g.pending, g.preparing, g.probing = false, false, false, false
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
	visible := m.turntableVisible()
	if !visible {
		if g.active || g.pending || g.preparing {
			return m.ClearGraphics()
		}
		return nil
	}
	key, coverKey := m.turntableSceneKey(), m.turntableCoverKey()
	if g.target != key {
		if g.active && g.coverKey == coverKey {
			// Keep the current image on screen while changing only its LED state.
			// Song/source changes still clear immediately to avoid stale album art.
			g.generation++
			g.pending, g.preparing, g.probing = false, false, false
			g.prepared = ""
		} else {
			cleanup := m.ClearGraphics()
			g.target = key
			if cleanup != nil {
				return cleanup
			}
		}
		g.target = key
	}
	var commands []tea.Cmd
	if !g.disabled && !g.queried {
		g.queried = true
		commands = append(commands, tea.Raw(g.wrap(termimage.Query(g.id))), tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg { return graphicsTimeout{} }))
		if !g.relay && !g.known {
			commands = append(commands, tea.Raw(ansi.RequestXTVersion))
		}
	}
	if g.failed == key || g.pending || g.preparing {
		return tea.Batch(commands...)
	}
	if g.active && g.sent == key && g.frameSupport == 0 && !g.probing && m.recordCoverVisible() && m.artwork.image.Valid() && m.rhythmPlaying() {
		g.probing = true
		generation := g.generation
		out := graphicsWrite{generation: generation, data: g.wrap(termimage.ProbeFrames(g.id))}
		return tea.Batch(func() tea.Msg { return out }, tea.Tick(time.Second, func(time.Time) tea.Msg { return graphicsFrameTimeout{generation} }))
	}
	if g.probing {
		return nil
	}
	angle := m.record.angle
	newScene := g.prepared != key
	// Upload a completed frame before accepting a newer clock angle. Otherwise
	// rendering that takes longer than one tick can starve output indefinitely.
	newFrame := g.renderer != nil && g.preparedAngle != angle && (!g.ready() || g.sent == key && g.sentAngle == g.preparedAngle)
	fullImageNeeded := g.preparedPatch && (!g.active || g.frameSupport != 1)
	pngNeeded := g.ready() && len(g.png) == 0
	if newScene || newFrame || fullImageNeeded || pngNeeded {
		g.preparing = true
		generation, im, renderer := g.generation, m.artwork.image, g.renderer
		if !m.recordCoverVisible() {
			im = artwork.Image{}
		}
		if g.coverKey != coverKey {
			renderer = nil
		}
		patch := g.active && g.sent == key && g.frameSupport == 1
		encode := g.ready()
		appearance := m.turntableAppearance()
		commands = append(commands, func() tea.Msg {
			var err error
			if renderer == nil {
				renderer, err = turntable.NewRendererState(im, appearance)
			} else {
				renderer = renderer.WithState(appearance)
			}
			var scene turntable.Scene
			if err == nil {
				if encode {
					scene, err = renderer.Render(angle, patch)
				} else {
					scene.Pixels = renderer.Image(angle)
				}
			}
			return graphicsPrepared{generation: generation, key: key, png: scene.PNG, scene: scene.Pixels, err: err, renderer: renderer, angle: angle, patch: patch, coverKey: coverKey}
		})
	}
	if len(commands) > 0 {
		return tea.Batch(commands...)
	}
	if !g.ready() || g.prepared != key {
		return nil
	}
	w, h := m.turntableSize()
	_, _, columns, rows := turntableImageRect(w, m.turntableGraphicHeight(h, w))
	if g.active && g.sent == key && g.columns == columns && g.rows == rows && g.sentAngle == g.preparedAngle {
		return nil
	}
	sequence := termimage.Placement(g.id, columns, rows)
	if g.sent != key || g.sentAngle != g.preparedAngle {
		if g.preparedPatch && g.sent == key {
			box := turntable.CoverBounds()
			sequence = termimage.UpdateFrame(g.png, g.id, box.Min.X, box.Min.Y) + sequence
		} else {
			sequence = termimage.Transmit(g.png, g.id, columns, rows)
		}
	}
	g.pending = true
	generation := g.generation
	out := graphicsWrite{generation, g.wrap(sequence), graphicsUploaded{generation, key, columns, rows, g.preparedAngle}}
	return func() tea.Msg { return out }
}

func (m *Model) graphicsMessage(msg tea.Msg) {
	g := &m.graphics
	switch msg := msg.(type) {
	case uv.KittyGraphicsEvent:
		if msg.Options.ID != g.id || !g.queried {
			return
		}
		if g.probing || msg.Options.Rows == 1 {
			if g.probing {
				g.probing = false
				g.frameSupport = -1
				if string(msg.Payload) == "OK" {
					g.frameSupport = 1
				}
			} else if g.frameSupport == 1 && string(msg.Payload) != "OK" {
				// A terminal can evict or reject a frame. Fall back to complete images.
				g.frameSupport = -1
				g.prepared, g.sent = "", ""
			}
		} else {
			g.replied = string(msg.Payload) == "OK"
			g.checked = true
		}
	case graphicsFrameTimeout:
		if msg.generation == g.generation && g.probing {
			g.probing = false
			g.frameSupport = -1
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
		g.scene = msg.scene
		g.renderer, g.preparedAngle, g.preparedPatch, g.coverKey = msg.renderer, msg.angle, msg.patch, msg.coverKey
	case graphicsUploaded:
		if msg.generation != g.generation || msg.key != g.target || !g.pending {
			return
		}
		g.pending, g.active = false, true
		g.sent, g.columns, g.rows, g.sentAngle = msg.key, msg.columns, msg.rows, msg.angle
	}
}
