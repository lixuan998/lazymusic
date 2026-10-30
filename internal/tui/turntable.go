package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"lazymusic/internal/artwork"
)

func centered(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	return fit(strings.Repeat(" ", max(0, (w-ansi.StringWidth(s))/2))+s, w)
}

func (m *Model) turntableRows(w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	album, artist, title := "No album", "No artist", "Choose a song"
	im := m.artwork.image
	if t := m.State.Playback.Track; t != nil {
		album, artist, title = plain(t.Album), plain(t.Artist), plain(t.Title)
		if album == "" {
			album = "Unknown album"
		}
		if artist == "" {
			artist = "Unknown artist"
		}
	} else {
		im = artwork.Image{}
	}
	var metadata []string
	if h >= 14 {
		metadata = []string{muted.Render(centered("ALBUM", w)), text.Render(centered(album, w)),
			muted.Render(centered("ARTIST", w)), text.Render(centered(artist, w)),
			muted.Render(centered("TRACK", w)), accent.Bold(true).Render(centered(title, w))}
	} else {
		metadata = []string{muted.Render("▤ ") + text.Render(album), muted.Render("♬ ") + text.Render(artist), accent.Render("♪ " + title)}
	}
	var rows []string
	if h >= 7 {
		graphicH := min(max(3, (min(w, 44)*3+3)/8+1), h-len(metadata)-1)
		rows = recordPlayer(w, graphicH, im, m.rhythm.phase)
		label := "VINYL  ·  " + playbackIcon(m.State.Playback.Status, m.State.Playback.Playing, m.State.Playback.Loading)
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

// Two RGB pixels per cell make covers work in plain terminals and nested
// Neovim terminals, without external image programs or graphics protocols.
func recordPlayer(width, height int, cover artwork.Image, phase float64) []string {
	w, ph := min(width, 44), max(2, height*2)
	const background = 0x15191f
	pixels := make([]uint32, w*ph)
	for i := range pixels {
		pixels[i] = background
	}
	put := func(x, y int, color uint32) {
		if x >= 0 && x < w && y >= 0 && y < ph {
			pixels[y*w+x] = color
		}
	}
	for y := 1; y < ph-2; y++ {
		for x := 0; x < w; x++ {
			c := uint32(0x292c36)
			if y == 1 || x == 0 {
				c = 0x5a5c6b
			}
			if y >= ph-4 {
				c = 0x473740
			}
			if x == w-1 {
				c = 0x23242c
			}
			put(x, y, c)
		}
	}
	cx, cy := float64(w)*.37, float64(ph-1)*.46
	radius := math.Min(float64(w)*.33, float64(ph-3)*.47)
	for y := 1; y < ph-3; y++ {
		for x := 1; x < w-1; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			d := math.Hypot(dx, dy)
			if d > radius {
				continue
			}
			c := uint32(0x11131b)
			if d > radius-.7 {
				c = 0x777c8e
			} else if d > radius*.69 {
				shade := uint32(30 + 12*(1+math.Sin(d*5))/2)
				c = shade<<16 | (shade+2)<<8 | (shade + 9)
				glint := math.Cos(math.Atan2(dy, dx) - phase*1.8)
				if glint > .94 || glint < -.96 {
					c += 0x15131c
				}
			} else if cover.Valid() {
				u := (dx/(radius*1.38) + .5) * float64(cover.Width-1)
				v := (dy/(radius*1.38) + .5) * float64(cover.Height-1)
				i := (min(cover.Height-1, max(0, int(v)))*cover.Width + min(cover.Width-1, max(0, int(u)))) * 3
				c = uint32(cover.RGB[i])<<16 | uint32(cover.RGB[i+1])<<8 | uint32(cover.RGB[i+2])
			} else {
				c = 0x9d425a
				if d < radius*.38 {
					c = 0xd48b8d
				}
			}
			if d < .65 {
				c = 0xe8d8b6
			}
			put(x, y, c)
		}
	}
	line := func(x0, y0, x1, y1 float64, color uint32) {
		n := max(1, int(math.Ceil(math.Hypot(x1-x0, y1-y0)*2)))
		for i := 0; i <= n; i++ {
			t := float64(i) / float64(n)
			put(int(math.Round(x0+(x1-x0)*t)), int(math.Round(y0+(y1-y0)*t)), color)
		}
	}
	ax, ay := float64(w-3), 3.
	kx, ky := float64(w-3), float64(ph)*.55
	ex, ey := cx+radius*.73, cy+radius*.55
	line(ax, ay, kx, ky, 0xd2bd9a)
	line(kx, ky, ex, ey, 0xd2bd9a)
	put(int(ax), int(ay), 0xffd8ac)
	put(int(ex), int(ey), 0xf47b96)
	put(w-3, ph-5, 0x94d8ba)
	for _, x := range []int{2, w - 4} {
		put(x, ph-2, 0x827780)
		put(x+1, ph-2, 0x827780)
	}
	rows := make([]string, height)
	for y := range rows {
		var row strings.Builder
		row.WriteString(strings.Repeat(" ", max(0, (width-w)/2)))
		var lastTop, lastBottom uint32 = 0xffffffff, 0xffffffff
		for x := 0; x < w; x++ {
			top, bottom := pixels[(y*2)*w+x], pixels[(y*2+1)*w+x]
			if top != lastTop || bottom != lastBottom {
				fmt.Fprintf(&row, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm", top>>16&255, top>>8&255, top&255, bottom>>16&255, bottom>>8&255, bottom&255)
				lastTop, lastBottom = top, bottom
			}
			row.WriteRune('▀')
		}
		row.WriteString("\x1b[0m")
		rows[y] = fit(row.String(), width)
	}
	return rows
}
