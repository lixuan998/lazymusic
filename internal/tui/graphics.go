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
	"lazymusic/internal/artwork"
	"lazymusic/internal/termimage"
	"lazymusic/internal/turntable"
)

type graphicsState struct {
	id                                                int
	disabled, relay, known, queried, replied, checked bool
	active, pending, preparing, clearing              bool
	generation                                        uint64
	used                                              bool
	queryPending, querySent, queryTimed               bool
	queryGeneration                                   uint64
	queryAttempts                                     int
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
	confirmed := relay && os.Getenv("LAZYMUSIC_IMAGE_SUPPORTED") == "1"
	return graphicsState{replied: confirmed, id: termimage.MinID + int(binary.BigEndian.Uint16(nonce[:]))%65535, relay: relay,
		known:    confirmed || strings.Contains(host, "ghostty") || strings.Contains(host, "kitty"),
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
	g := &m.graphics
	var cleanup string
	if g.used {
		cleanup += termimage.Delete(g.id)
	}
	if g.queried {
		cleanup += termimage.Delete(g.probeID())
	}
	if m.video.used {
		cleanup += termimage.Delete(m.videoImageID())
	}
	return g.wrap(cleanup)
}
func (m *Model) ClearGraphics() tea.Cmd {
	g := &m.graphics
	g.generation++
	g.active, g.pending, g.preparing, g.probing = false, false, false, false
	g.sent = ""
	if g.used {
		cleanup := g.wrap(termimage.Delete(g.id))
		g.clearing = true
		out := graphicsWrite{g.generation, cleanup, graphicsCleared{g.generation}}
		return func() tea.Msg { return out }
	}
	return nil
}

// TerminalOutput lets the containing application preserve both output order
// and its source epoch before handing raw graphics to the terminal renderer.
func (m *Model) TerminalOutput(msg tea.Msg) (string, tea.Msg, bool) {
	if out, ok := msg.(videoWrite); ok {
		if out.generation != m.video.generation {
			return "", nil, false
		}
		return out.data, out.done, true
	}

	if query, ok := msg.(graphicsQueryWrite); ok {
		g := &m.graphics
		if query.attempt != g.queryGeneration || !g.queryPending || g.disabled || g.replied {
			return "", nil, false
		}
		return g.wrap(termimage.Query(g.probeID())), graphicsQuerySent{query.attempt}, true
	}
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
	if probe := m.syncGraphicsProbe(); probe != nil {
		commands = append(commands, probe)
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
	g.pending, g.used = true, true
	generation := g.generation
	out := graphicsWrite{generation, g.wrap(sequence), graphicsUploaded{generation, key, columns, rows, g.preparedAngle}}
	return func() tea.Msg { return out }
}

func (m *Model) graphicsMessage(msg tea.Msg) {
	g := &m.graphics
	switch msg := msg.(type) {
	case uv.KittyGraphicsEvent:
		if msg.Options.ID == g.probeID() && g.queried {
			g.queryPending = false
			g.checked = true
			// Positive evidence is sticky for this terminal. A late error can describe
			// a deleted image or an older probe; it cannot revoke a successful query.
			if string(msg.Payload) == "OK" {
				g.replied, g.known = true, true
			}
			return
		}
		if msg.Options.ID != g.id || !g.used {
			return
		}
		if g.probing {
			g.probing = false
			g.frameSupport = -1
			if string(msg.Payload) == "OK" {
				g.frameSupport = 1
			}
		} else if g.frameSupport == 1 && string(msg.Payload) != "OK" {
			// Frame errors may omit r=1. Retry a complete native image without
			// confusing a drawing failure with the separate capability query.
			g.frameSupport = -1
			g.prepared, g.sent = "", ""
		}
	case graphicsQuerySent:
		if msg.attempt == g.queryGeneration && g.queryPending {
			g.querySent = true
		}
	case graphicsTimeout:
		if msg.attempt != g.queryGeneration || !g.queryPending || !g.queryTimed || g.replied {
			return
		}
		g.queryPending, g.querySent, g.queryTimed = false, false, false
		if g.queryAttempts >= graphicsProbeAttempts {
			g.checked = true
			m.Report("info", "graphics", "Terminal image query timed out; using pixel artwork until support is confirmed")
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
