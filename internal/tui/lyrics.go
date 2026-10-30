package tui

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"lazymusic/internal/catalog"
	"lazymusic/internal/lyrics"
)

type lyricRow struct {
	text string
	line int
}
type lyricsState struct {
	key                      string
	generation               uint64
	cancel                   context.CancelFunc
	loading, loaded, ticking bool
	document                 lyrics.Document
	width                    int
	retryAt                  time.Time
	rows                     []lyricRow
	anchors                  []int
}
type lyricsMsg struct {
	key        string
	generation uint64
	document   lyrics.Document
	err        error
}
type lyricsTick struct{ generation uint64 }

func (m *Model) CancelLyrics() {
	a := &m.lyrics
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.generation++
	a.loading = false
	a.ticking = false
}

func (m *Model) syncLyrics() tea.Cmd {
	a := &m.lyrics
	if !m.turntableVisible() || m.State.Playback.Track == nil {
		if a.loading || a.ticking {
			m.CancelLyrics()
		}
		return nil
	}
	t := m.State.Playback.Track
	source := t.Source
	if source == "" {
		source = m.State.Source.ID
	}
	if source != m.State.Source.ID || t.ID == "" {
		return nil
	}
	subject := catalog.Subject{Source: source, ID: t.ID, Kind: t.Kind, PlaybackID: t.PlaybackID}
	b, _ := json.Marshal(subject)
	key := string(b)
	if a.key != key {
		m.CancelLyrics()
		a.key, a.document, a.loaded = key, lyrics.Document{}, false
		a.rows, a.anchors = nil, nil
		a.retryAt = time.Time{}
	}
	if a.loaded || a.loading || time.Now().Before(a.retryAt) {
		return nil
	}
	provider, ok := m.api.(catalog.LyricsProvider)
	if !ok {
		a.loaded = true
		return nil
	}
	ctx, cancel := context.WithTimeout(m.ctx, 22*time.Second)
	a.cancel, a.loading = cancel, true
	generation := a.generation
	return func() tea.Msg {
		defer cancel()
		document, err := provider.Lyrics(ctx, subject)
		return lyricsMsg{key, generation, document, err}
	}
}

func (m *Model) acceptLyrics(msg lyricsMsg) {
	a := &m.lyrics
	if msg.generation != a.generation || msg.key != a.key || !a.loading {
		return
	}
	a.cancel, a.loading, a.loaded = nil, false, true
	if msg.err == nil && msg.document.Valid() {
		a.document = msg.document
		a.rows, a.anchors = nil, nil
	} else if msg.err != nil && m.ctx.Err() == nil {
		a.loaded = false
		a.retryAt = time.Now().Add(30 * time.Second)
		m.Report("info", "lyrics", "Synchronized lyrics unavailable: "+msg.err.Error())
	}
}

func (m *Model) syncLyricsClock() tea.Cmd {
	a := &m.lyrics
	if !m.turntableVisible() || !m.recordCoverVisible() || !m.rhythmPlaying() || len(a.document.Lines) == 0 {
		if a.ticking {
			a.generation++
			a.ticking = false
		}
		return nil
	}
	if a.ticking {
		return nil
	}
	a.ticking = true
	generation := a.generation
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return lyricsTick{generation} })
}

func (m *Model) lyricPosition() time.Duration {
	position := m.State.Playback.Position
	if m.rhythmPlaying() && !m.State.ObservedAt.IsZero() {
		elapsed := time.Since(m.State.ObservedAt)
		// Do not run ahead indefinitely if the daemon becomes unavailable.
		position += max(0, min(1500*time.Millisecond, elapsed))
	}
	return position
}

func (m *Model) lyricsHeight(h int) int {
	if h >= 24 {
		return 7
	}
	if h >= 14 {
		return 5
	}
	if h >= 9 {
		return 3
	}
	return 1
}

func (m *Model) lyricRows(w, h int) []string {
	rows := make([]string, h)
	for i := range rows {
		rows[i] = fit("", w)
	}
	if h <= 0 || w <= 0 {
		return rows
	}
	a := &m.lyrics
	if m.State.Playback.Track == nil || len(a.document.Lines) == 0 {
		label := "No Lyrics Available"
		if a.loading && m.State.Playback.Track != nil {
			label = "Loading lyrics…"
		}
		rows[h/2] = muted.Render(centered(label, w))
		return rows
	}
	if a.rows == nil || a.width != w {
		a.width = w
		a.rows = nil
		a.anchors = make([]int, len(a.document.Lines))
		for index, line := range a.document.Lines {
			label := plain(line.Text)
			if label == "" {
				label = "♪"
			}
			lines := strings.Split(ansi.Wrap(label, max(1, w-4), ""), "\n")
			a.anchors[index] = len(a.rows) + (len(lines)-1)/2
			for _, part := range lines {
				a.rows = append(a.rows, lyricRow{part, index})
			}
		}
	}
	active := a.document.Active(m.lyricPosition())
	anchor := -1
	if active >= 0 {
		anchor = a.anchors[active]
	}
	for y := range rows {
		index := anchor + y - h/2
		if index < 0 || index >= len(a.rows) {
			continue
		}
		row := a.rows[index]
		style := muted
		if row.line == active {
			style = accent.Bold(true)
		}
		rows[y] = style.Render(centered(row.text, w))
	}
	return rows
}
