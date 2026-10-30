package tui

import (
	"fmt"
	"image"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"lazymusic/internal/termimage"
	"lazymusic/internal/turntable"
)

func centered(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	return fit(strings.Repeat(" ", max(0, (w-ansi.StringWidth(s))/2))+s, w)
}

func (m *Model) turntableAppearance() turntable.Appearance {
	if !m.recordCoverVisible() {
		return turntable.Stopped
	}
	if m.rhythmPlaying() {
		return turntable.Playing
	}
	return turntable.Paused
}

func (m *Model) turntableCoverKey() string {
	key := m.State.Source.ID + "|empty"
	if t := m.State.Playback.Track; m.recordCoverVisible() {
		key = m.State.Source.ID + "|" + t.ID
		if m.artwork.image.Valid() {
			key += "|" + m.artwork.key
		}
	}
	return key
}

func (m *Model) turntableSceneKey() string {
	return fmt.Sprintf("%s|state:%d", m.turntableCoverKey(), m.turntableAppearance())
}

func (m *Model) turntableRows(w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	lyricRows := m.lyricRows(w, m.lyricsHeight(h))
	var rows []string
	if h >= 7 {
		graphicH := m.turntableGraphicHeight(h, w)
		rows = make([]string, graphicH)
		for i := range rows {
			rows[i] = fit("", w)
		}
		left, top, columns, height := turntableImageRect(w, graphicH)
		g, key := m.graphics, m.turntableSceneKey()

		var picture []string
		if g.active && (g.sent == key || g.coverKey == m.turntableCoverKey()) && g.columns == columns && g.rows == height {
			picture = termimage.PlaceholderRows(g.id, columns, height)
		} else if g.scene != nil && g.prepared == key && (g.disabled || g.checked && !g.ready() || g.failed == key) {
			picture = bitmapRows(g.scene, columns, height)
		}
		for y, line := range picture {
			rows[top+y] = fit(strings.Repeat(" ", left)+line, w)
		}
		rows = append(rows, "")
	}
	rows = append(rows, lyricRows...)
	if len(rows) > h {
		rows = rows[len(rows)-h:]
	}
	pad := max(0, (h-len(rows))/2)
	rows = append(make([]string, pad), rows...)
	for len(rows) < h {
		rows = append(rows, "")
	}
	for i := range rows {
		rows[i] = fit(rows[i], w)
	}
	return rows
}

func (m *Model) turntableGraphicHeight(h, w int) int {
	return min(max(3, (w+2)/3), h-m.lyricsHeight(h)-1)
}

// The entire 3:2 photograph shares one placement. The cover's center is fixed
// inside that image, so terminal cell rounding can never shift it on the disc.
func turntableImageRect(w, h int) (left, top, columns, rows int) {
	if w <= 0 || h <= 0 {
		return
	}
	columns = min(w, h*3)
	rows = (columns + 2) / 3
	left, top = (w-columns)/2, (h-rows)/2
	return
}

// Unsupported terminals use a character approximation of the same composed
// photograph. This fallback never mixes an original cover with a pixel chassis.
func bitmapRows(im image.Image, w, h int) []string {
	if im == nil || w <= 0 || h <= 0 {
		return nil
	}
	b := im.Bounds()
	rows := make([]string, h)
	for y := range rows {
		var line strings.Builder
		var lastTop, lastBottom uint32 = 0xffffffff, 0xffffffff
		for x := 0; x < w; x++ {
			colors := [2]uint32{}
			for half := range colors {
				sx := b.Min.X + min(b.Dx()-1, (x*2+1)*b.Dx()/(2*w))
				sy := b.Min.Y + min(b.Dy()-1, ((y*2+half)*2+1)*b.Dy()/(4*h))
				r, g, b, _ := im.At(sx, sy).RGBA()
				colors[half] = r>>8<<16 | g>>8<<8 | b>>8
			}
			a, b := colors[0], colors[1]
			if a != lastTop || b != lastBottom {
				fmt.Fprintf(&line, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm", a>>16&255, a>>8&255, a&255, b>>16&255, b>>8&255, b&255)
				lastTop, lastBottom = a, b
			}
			line.WriteRune('▀')
		}
		line.WriteString("\x1b[0m")
		rows[y] = line.String()
	}
	return rows
}
