package tui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"lazymusic/internal/eventlog"
	"math"
	"sort"
	"strings"
	"time"
)

type ContentPanel struct {
	Title string
	Rows  []string
}

func (m *Model) LoginSelected() bool { return m.UI.NavIndex == m.findNav("login") }
func (m *Model) OpenLogin() tea.Cmd {
	m.UI.NavIndex = m.findNav("login")
	m.UI.Focus = 1
	if m.UI.Layout == "queue" {
		m.UI.Layout = "content"
	}
	return m.dirty()
}
func (m *Model) Editing() bool    { return m.mode != "" }
func (m *Model) HasModal() bool   { return m.prompt || m.help || m.qualityDialog || m.work != nil }
func (m *Model) Size() (int, int) { return m.width, m.height }
func (m *Model) SetSize(w, h int) { m.width, m.height = w, h }
func (m *Model) cycleLayout(next bool) tea.Cmd {
	modes := []string{"full", "content", "queue"}
	i := 0
	for n, v := range modes {
		if v == m.UI.Layout {
			i = n
		}
	}
	if next {
		i = (i + 1) % len(modes)
	} else {
		i = (i + len(modes) - 1) % len(modes)
	}
	m.UI.Zen = false
	m.UI.Layout = modes[i]
	if m.UI.Layout == "content" {
		m.UI.Focus = 1
	} else if m.UI.Layout == "queue" {
		m.UI.Focus = 2
	}
	return m.dirty()
}
func (m *Model) bodyHeight() int {
	h := m.height - 3 - m.playerHeight()
	if m.UI.ShowLogs {
		h -= m.logHeight()
	}
	return max(3, h)
}
func (m *Model) playerHeight() int {
	extra := 0
	if m.UI.ShowLogs {
		extra = m.logHeight()
	}
	return min(max(4, m.UI.PlayerHeight), max(4, min(m.height/2, m.height-9-extra)))
}
func (m *Model) logHeight() int {
	h := m.UI.LogHeight
	if h == 0 {
		h = 8
	}
	return min(max(3, h), max(3, min(m.height/3, m.height-13)))
}
func (m *Model) allLogs() []eventlog.Entry {
	out := append([]eventlog.Entry(nil), m.State.Logs...)
	out = append(out, m.localLogs...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out
}
func (m *Model) acceptLogs(entries []eventlog.Entry) {
	for _, e := range entries {
		if e.ID > m.seenLog {
			m.seenLog = e.ID
			if e.Level == "error" || e.Level == "warn" {
				copy := e
				m.toast = &copy
				m.toastVersion++
			}
		}
	}
}
func (m *Model) Report(level, component, message string) {
	message = eventlog.Redact(message)
	if len(m.localLogs) > 0 && m.localLogs[len(m.localLogs)-1].Message == message {
		return
	}
	e := eventlog.Entry{Time: time.Now(), Level: level, Component: component, Message: message}
	m.localLogs = append(m.localLogs, e)
	if len(m.localLogs) > 100 {
		m.localLogs = m.localLogs[len(m.localLogs)-100:]
	}
	if level == "error" || level == "warn" {
		m.toast = &e
		m.toastVersion++
	}
}
func (m *Model) logRows(h int) []string {
	logs := m.allLogs()
	end := max(0, len(logs)-m.logOffset)
	start := max(0, end-h)
	var rows []string
	for _, e := range logs[start:end] {
		label := fmt.Sprintf("%s %-5s %s · %s", e.Time.Local().Format("15:04:05"), strings.ToUpper(e.Level), e.Component, plain(e.Message))
		if e.Level == "error" || e.Level == "warn" {
			label = accent.Render(label)
		} else {
			label = muted.Render(label)
		}
		rows = append(rows, label)
	}
	if len(rows) == 0 {
		rows = []string{muted.Render("No runtime messages")}
	}
	return rows
}
func (m *Model) visualizerRows(width, height int) []string {
	return m.rhythmRows(m.UI.Visualizer, width, height)
}
func (m *Model) rhythmRows(mode, width, height int) []string {
	if height < 4 {
		return nil
	}
	height = max(3, height)
	rows := make([]string, height)
	phase := m.State.Playback.Position.Seconds()
	playing := m.State.Playback.Playing
	if mode == 0 {
		for row := 0; row < height-1; row++ {
			var b strings.Builder
			for col := 0; col < width; col++ {
				level := 0.0
				if playing {
					level = (math.Sin(phase*3+float64(col)*.7) + math.Sin(phase*1.2+float64(col)*.27) + 2) / 4
				}
				if int(level*float64(height-1)) >= height-1-row {
					b.WriteString("▆")
				} else {
					b.WriteString(" ")
				}
			}
			rows[row] = accent.Render(b.String())
		}
		rows[height-1] = muted.Render("Pulse")
	} else {
		for row := 0; row < height-1; row++ {
			var b strings.Builder
			for col := 0; col < width; col++ {
				point := (height - 2) / 2
				if playing {
					point = int((math.Sin(float64(col)*.4-phase*4) + 1) / 2 * float64(height-2))
				}
				if row == point {
					b.WriteString("•")
				} else {
					b.WriteString(" ")
				}
			}
			rows[row] = accent.Render(b.String())
		}
		rows[height-1] = muted.Render("Wave")
	}
	return rows
}
func playbackIcon(status string, playing, loading bool) string {
	if loading {
		return "◌"
	}
	if playing {
		return "▶"
	}
	if status == "stopped" {
		return "■"
	}
	if status == "error" {
		return "⚠"
	}
	return "Ⅱ"
}
func playbackMode(repeat int, shuffle bool) string {
	if shuffle {
		return "⤨ Shuffle"
	}
	switch repeat {
	case 1:
		return "↺1 Repeat one"
	case 2:
		return "↻ Repeat all"
	}
	return "→ In order"
}
func (m *Model) IndexForPage(page string) int { return m.findNav(page) }

func (m *Model) HasToast() bool { return m.toast != nil }

// The application calls this too when it reports a connection failure outside Update.
func (m *Model) NotificationTimer() tea.Cmd {
	version := m.toastVersion
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return toastExpired(version) })
}
