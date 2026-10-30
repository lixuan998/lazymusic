package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func (m *Model) header(title string, w int) string {
	source := m.State.Source.Name
	if source == "" {
		source = "MUSIC"
	}
	source = ansi.Truncate(plain(source), max(8, w/3), "…")
	brand := accent.Bold(true).Render("LazyMusic ☕️") + muted.Render("  /  ") + text.Render(plain(title))
	line := fit(brand, max(0, w-4-ansi.StringWidth(source)-2)) + "  " + muted.Render(source)
	return box("", []string{line}, w, 3, false)
}

func (m *Model) playerBox(w int) string {
	s := m.State.Playback
	if s.Track == nil && !s.Loading && !s.Playing {
		s.Status = "stopped"
	}
	title, artist, album := "Nothing playing", "Choose a song and press Enter", ""
	total := time.Duration(0)
	if s.Track != nil {
		title, artist, album, total = plain(s.Track.Title), plain(s.Track.Artist), plain(s.Track.Album), s.Track.Duration
	}
	status := "PAUSED"
	if s.Status == "stopped" {
		status = "STOPPED"
	}
	if s.Status == "error" {
		status = "ERROR"
	}
	if s.Playing {
		status = "PLAYING"
	}
	if s.Loading {
		status = "BUFFERING"
	}
	status = playbackIcon(s.Status, s.Playing, s.Loading) + " " + status
	quality := "Original"
	for _, q := range m.State.Source.Qualities {
		if q.ID == m.State.PreferredQuality {
			quality = q.Name + " preferred"
		}
	}
	volume := "system vol —"
	if m.State.SystemVolume.Available {
		volume = fmt.Sprintf("system vol %.0f%%", s.Volume*100)
	}
	mode := playbackMode(s.RepeatMode, s.ShuffleMode)
	clock := duration(s.Position) + " / " + duration(total)
	live := s.Track != nil && s.Track.MediaType == "live"
	if live {
		clock = "● LIVE  " + duration(s.Position)
	}
	bar := func(width int) string {
		filled := 0
		if total > 0 && !live {
			filled = int(float64(s.Position) / float64(total) * float64(width))
		}
		filled = min(width, max(0, filled))
		return accent.Render(strings.Repeat("▄", filled)) + muted.Render(strings.Repeat("▄", width-filled))
	}
	h, inner := m.playerHeight(), w-4
	artistAlbum := artist
	if album != "" {
		artistAlbum += "  ·  " + album
	}
	boxTitle := "4 Now playing"
	if s.Track != nil && s.Track.MediaType == "episode" {
		boxTitle += " · Radio episode"
	}
	if s.Continuous != nil {
		boxTitle += " · Station: " + plain(s.Continuous.Term)
	}
	var rows []string
	if h <= 5 {
		boxTitle += " · " + mode + " · " + quality
		rows = append(rows, text.Bold(true).Render(title)+muted.Render("  ·  "+artistAlbum))
		if h == 5 {
			rows = append(rows, "")
		}
		remaining := max(1, inner-ansi.StringWidth(status)-ansi.StringWidth(clock)-ansi.StringWidth(volume)-6)
		rows = append(rows, good.Render(status)+"  "+bar(remaining)+"  "+clock+"  "+muted.Render(volume))
	} else {
		rows = []string{text.Bold(true).Render(title), muted.Render(artistAlbum)}
		if h >= 8 {
			rows = append(rows, "")
		}
		rows = append(rows, bar(inner))
		left := good.Render(status) + "  " + clock
		rows = append(rows, fit(left, max(0, inner-ansi.StringWidth(volume)-2))+"  "+muted.Render(volume))
		if h >= 7 {
			rows = append(rows, muted.Render(mode+"  ·  "+quality))
		} else {
			boxTitle += " · " + mode + " · " + quality
		}
		pad := max(0, (h-2-len(rows))/2)
		rows = append(make([]string, pad), rows...)
	}
	return box(boxTitle, rows, w, h, m.UI.Focus == 3)
}
