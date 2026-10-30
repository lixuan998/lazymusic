package tui

import (
	"fmt"
	"image"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"lazymusic/internal/termimage"
)

func centered(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	return fit(strings.Repeat(" ", max(0, (w-ansi.StringWidth(s))/2))+s, w)
}

func (m *Model) turntableSceneKey() string {
	key := m.State.Source.ID + "|empty"
	if t := m.State.Playback.Track; t != nil {
		key = m.State.Source.ID + "|" + t.ID
		if m.artwork.image.Valid() {
			key += "|" + m.artwork.key
		}
	}
	return key
}

func (m *Model) turntableRows(w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	album, artist, title := "No album", "No artist", "Choose a song"
	if t := m.State.Playback.Track; t != nil {
		album, artist, title = plain(t.Album), plain(t.Artist), plain(t.Title)
		if album == "" {
			album = "Unknown album"
		}
		if artist == "" {
			artist = "Unknown artist"
		}
	}
	var metadata []string
	if h >= 14 {
		metadata = []string{muted.Render(centered("ALBUM", w)), text.Render(centered(album, w)), muted.Render(centered("ARTIST", w)), text.Render(centered(artist, w)), muted.Render(centered("TRACK", w)), accent.Bold(true).Render(centered(title, w))}
	} else {
		metadata = []string{muted.Render("▤ ") + text.Render(album), muted.Render("♬ ") + text.Render(artist), accent.Render("♪ " + title)}
	}
	var rows []string
	if h >= 7 {
		graphicH := m.turntableGraphicHeight(h, w)
		rows = make([]string, graphicH)
		for i := range rows {
			rows[i] = fit("", w)
		}
		left, top, columns, height := turntableImageRect(w, graphicH)
		g, key := m.graphics, m.turntableSceneKey()
		label := "Preparing turntable…"
		var picture []string
		if g.active && g.sent == key && g.columns == columns && g.rows == height {
			picture = termimage.PlaceholderRows(g.id, columns, height)
			label = "VINYL · " + playbackIcon(m.State.Playback.Status, m.State.Playback.Playing, m.State.Playback.Loading)
		} else if g.scene != nil && g.prepared == key && (g.disabled || g.checked && !g.ready() || g.failed == key) {
			picture = bitmapRows(g.scene, columns, height)
			label = "PIXEL · " + playbackIcon(m.State.Playback.Status, m.State.Playback.Playing, m.State.Playback.Loading)
		}
		for y, line := range picture {
			rows[top+y] = fit(strings.Repeat(" ", left)+line, w)
		}
		if m.artwork.loading {
			label = "Loading artwork…"
		}
		rows = append(rows, muted.Render(centered(label, w)))
	}
	rows = append(rows, metadata...)
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
	metadata := 3
	if h >= 14 {
		metadata = 6
	}
	return min(max(3, (w+2)/3), h-metadata-1)
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
