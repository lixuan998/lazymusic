package tui

import (
	"fmt"
	"image"

	tea "charm.land/bubbletea/v2"
	"lazymusic/internal/artwork"
	"lazymusic/internal/termimage"
	"lazymusic/internal/turntable"
	"lazymusic/internal/video"
)

func (m *Model) selectGraphicsProtocol(protocol string) {
	g := &m.graphics
	if g.protocol == protocol {
		return
	}
	// Changing transport invalidates only image work, never verified capability.
	g.generation++
	g.active, g.pending, g.preparing, g.clearing, g.probing = false, false, false, false, false
	g.prepared, g.sent, g.sixelData, g.sixelSize = "", "", "", ""
	g.failed = ""
	g.frameSupport = 0
	g.protocol = protocol
	m.CancelVideo()
	m.video.active = false
	m.video.frame = video.Frame{}
	m.video.sixelData, m.video.sixelSize, m.video.failed = "", "", ""
	m.video.clearing, m.video.waiting = false, false
	m.video.frameSupport = 0
}

func (m *Model) turntableImageRect(w, h int) (left, top, columns, rows int) {
	if m.graphics.protocol != "sixel" || m.graphics.cellWidth <= 0 || m.graphics.cellHeight <= 0 {
		return turntableImageRect(w, h)
	}
	g := &m.graphics
	columns = min(w, termimage.MaxSixelWidth/g.cellWidth)
	rows = max(1, (columns*g.cellWidth*turntable.Height+g.cellHeight*turntable.Width-1)/(g.cellHeight*turntable.Width))
	limit := min(h, termimage.MaxSixelHeight/g.cellHeight)
	if rows > limit {
		rows = limit
		columns = min(columns, rows*g.cellHeight*turntable.Width/(g.cellWidth*turntable.Height))
	}
	left, top = max(0, (w-columns)/2), max(0, (h-rows)/2)
	return
}
func (m *Model) sixelSize(columns, rows int) string {
	return fmt.Sprintf("%dx%d/%dx%d", columns, rows, m.graphics.cellWidth, m.graphics.cellHeight)
}
func (m *Model) syncSixelGraphics() tea.Cmd {
	g := &m.graphics
	if g.clearing {
		return nil
	}
	if !m.turntableVisible() {
		if g.active || g.pending || g.preparing {
			return m.ClearGraphics()
		}
		return nil
	}
	key, coverKey := m.turntableSceneKey(), m.turntableCoverKey()
	if g.target != key {
		if g.active && g.coverKey == coverKey {
			g.generation++
			g.pending, g.preparing = false, false
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
	probe := m.syncGraphicsProbe()
	if !g.ready() {
		return probe
	}
	if g.failed == key || g.preparing || g.pending {
		return probe
	}
	w, h := m.turntableSize()
	_, _, columns, rows := m.turntableImageRect(w, m.turntableGraphicHeight(h, w))
	if columns < 1 || rows < 1 {
		return probe
	}
	size := m.sixelSize(columns, rows)
	angle := m.record.angle
	if g.prepared != key || g.sixelSize != size || g.preparedAngle != angle && (!g.active || g.sentAngle == g.preparedAngle) {
		g.preparing = true
		generation, im, renderer, appearance := g.generation, m.artwork.image, g.renderer, m.turntableAppearance()
		if !m.recordCoverVisible() {
			im = artwork.Image{}
		}
		if g.coverKey != coverKey {
			renderer = nil
		}
		width, height := columns*g.cellWidth, rows*g.cellHeight
		return func() tea.Msg {
			var err error
			if renderer == nil {
				renderer, err = turntable.NewRendererState(im, appearance)
			} else {
				renderer = renderer.WithState(appearance)
			}
			var scene image.Image
			var data string
			if err == nil {
				scene = renderer.Image(angle)
				data, err = termimage.EncodeSixel(scene, width, height)
			}
			return graphicsPrepared{generation: generation, key: key, scene: scene, err: err, renderer: renderer, angle: angle, coverKey: coverKey, sixelData: data, sixelSize: size}
		}
	}
	if g.active && g.sent == key && g.columns == columns && g.rows == rows && g.sentAngle == g.preparedAngle {
		return probe
	}
	// Publish the immutable layer before waking output: a text-identical view
	// otherwise gives Bubble Tea no reason to flush after frame preparation.
	g.active, g.pending, g.used = true, true, true
	g.sent, g.columns, g.rows, g.sentAngle = key, columns, rows, g.preparedAngle
	out := graphicsWrite{g.generation, termimage.SixelRefresh, graphicsUploaded{g.generation, key, columns, rows, g.preparedAngle}}
	return func() tea.Msg { return out }
}

// SixelLayers returns immutable, visible overlays in absolute terminal cells.
// Neovim's Kitty relay stays separate; Sixel is supported in a direct TTY only.
func (m *Model) SixelLayers() []termimage.Layer {
	g := &m.graphics
	if g.protocol != "sixel" || !g.ready() || m.exiting || m.ctx.Err() != nil {
		return nil
	}
	var layers []termimage.Layer
	if m.turntableVisible() && g.active && g.sixelData != "" && (g.sent == m.turntableSceneKey() || g.coverKey == m.turntableCoverKey()) {
		w, h := m.turntableSize()
		x, y, cols, rows := m.turntableImageRect(w, m.turntableGraphicHeight(h, w))
		if cols == g.columns && rows == g.rows && g.sixelSize == m.sixelSize(cols, rows) {
			y += max(0, (h-m.turntableGraphicHeight(h, w)-1-m.lyricsHeight(h))/2)
			x += 2
			y += 4 // 3 header rows, one box border, and two columns of box padding.
			if !m.UI.Zen && m.bodyHeight() >= 6 {
				y += m.navHeight()
			}
			layers = append(layers, termimage.Layer{Rect: image.Rect(x, y, x+cols, y+rows), Data: g.sixelData})
		}
	}
	v := &m.video
	if m.videoVisible() && v.active && v.sixelData != "" && v.key == m.videoKey() {
		x, y, cols, rows := m.videoRect(v.frame.Width, v.frame.Height)
		if cols == v.columns && rows == v.rows && v.sixelSize == m.sixelSize(cols, rows) {
			x += 2
			y += 4
			if m.UI.Layout != "content" {
				x += m.navWidth()
			}
			layers = append(layers, termimage.Layer{Rect: image.Rect(x, y, x+cols, y+rows), Data: v.sixelData})
		}
	}
	// No overlay may escape the visible alternate screen even during a resize.
	screen := image.Rect(0, 0, m.width, m.height)
	out := layers[:0]
	for _, l := range layers {
		if !l.Rect.Empty() && l.Rect.In(screen) {
			out = append(out, l)
		}
	}
	return out
}
