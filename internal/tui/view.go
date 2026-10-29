package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	text     = lipgloss.NewStyle().Foreground(lipgloss.Color("#dedee6"))
	muted    = lipgloss.NewStyle().Foreground(lipgloss.Color("#77798d"))
	accent   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5c78"))
	good     = lipgloss.NewStyle().Foreground(lipgloss.Color("#94d8ba"))
	selected = lipgloss.NewStyle().Foreground(lipgloss.Color("#fff1f5")).Background(lipgloss.Color("#442939"))
)

func plain(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(0, w-ansi.StringWidth(s)))
}
func duration(d time.Duration) string {
	seconds := max(0, int(d.Seconds()))
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func box(title string, rows []string, w, h int, active bool) string {
	w = max(4, w)
	h = max(3, h)
	style := muted
	if active {
		style = accent
	}
	top := "─ " + plain(title) + " "
	top = ansi.Truncate(top, w-2, "")
	lines := []string{style.Render("┌" + top + strings.Repeat("─", max(0, w-2-ansi.StringWidth(top))) + "┐")}
	for n := 0; n < h-2; n++ {
		s := ""
		if n < len(rows) {
			s = rows[n]
		}
		lines = append(lines, style.Render("│")+" "+fit(s, w-4)+" "+style.Render("│"))
	}
	lines = append(lines, style.Render("└"+strings.Repeat("─", w-2)+"┘"))
	return strings.Join(lines, "\n")
}

func (m *Model) align() {
	rows := max(1, m.bodyHeight()-5)
	m.cursor = max(0, min(m.cursor, len(m.visible())-1))
	m.offset = max(0, m.offset)
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	m.offset = min(m.offset, max(0, len(m.visible())-rows))
}

func (m *Model) mainRows(h int) []string {
	l := m.UI.Location
	var rows []string
	if l.Page == "home" {
		rows = append(rows, m.homeTabBar())
	} else if l.Page == "library" || l.Page == "search" {
		sections := []string{"songs", "albums", "artists"}
		if l.Page == "search" && m.State.Source.Capabilities.Playlists {
			sections = append(sections, "playlists")
		}
		tabs := []string{}
		for _, s := range sections {
			label := titleCase(s)
			if s == l.Section {
				tabs = append(tabs, accent.Bold(true).Render(label))
			} else {
				tabs = append(tabs, muted.Render(label))
			}
		}
		rows = append(rows, strings.Join(tabs, "  "))
	} else {
		rows = append(rows, muted.Render("Enter open/play   p play collection   a enqueue"))
	}
	if l.Page == "search" {
		scope := m.State.Source.Name + " catalog"
		if l.Scope == "library" {
			scope = "Library"
		}
		rows = append(rows, muted.Render(scope+"  ·  ")+plain(l.Query)+muted.Render(m.scopeHint()))
	} else if l.Page == "home" {
		label := "Personal recommendations"
		items := m.visible()
		if len(items) > 0 && m.cursor < len(items) && items[m.cursor].Group != "" {
			label = items[m.cursor].Group
		}
		rows = append(rows, muted.Render(plain(label)))
	} else if m.UI.Filter != "" {
		rows = append(rows, muted.Render("Filter loaded: ")+plain(m.UI.Filter))
	} else {
		rows = append(rows, muted.Render("TITLE                              ARTIST / ALBUM"))
	}
	items := m.visible()
	available := max(1, h-3)
	offset := max(0, min(m.offset, max(0, len(items)-available)))
	if m.loading && len(items) == 0 {
		rows = append(rows, muted.Render("Loading…"))
	} else if len(items) == 0 {
		label := "No items"
		if l.Page == "home" || l.Page == "home-section" {
			label = "No recommendations available; press R to refresh"
		}
		if l.Page == "search" && l.Query == "" {
			label = "Press i or Ctrl-f to search"
		}
		if m.UI.Filter != "" {
			label = "No matches in the loaded items"
		}
		rows = append(rows, muted.Render(label))
	} else {
		for i := offset; i < min(len(items), offset+available); i++ {
			item := items[i]
			prefix := "  "
			if i == m.cursor {
				prefix = "› "
			}
			label := plain(item.Title)
			if l.Page == "home" && item.Track == nil {
				label += "  " + muted.Render("["+plain(item.Kind)+"]")
			}
			if item.Track != nil {
				label += "  " + muted.Render("· "+plain(item.Subtitle)+"  "+duration(item.Track.Duration))
			} else if item.Subtitle != "" {
				label += "  " + muted.Render("· "+plain(item.Subtitle))
			}
			if i == m.cursor {
				label = selected.Render(prefix + label)
			} else {
				label = text.Render(prefix) + label
			}
			rows = append(rows, label)
		}
	}
	for len(rows) < h-1 {
		rows = append(rows, "")
	}
	more := ""
	if m.next != "" {
		more = "  ↓ more"
	}
	if m.loading {
		more = "  loading…"
	}
	rows = append(rows, muted.Render(fmt.Sprintf("%d loaded%s", len(m.items), more)))
	return rows
}

func (m *Model) queueRows(h int) []string {
	rows := []string{}
	q := m.State.Queue
	cursor := max(0, min(len(q)-1, m.UI.QueueCursor))
	available := max(1, h-2)
	offset := max(0, cursor-available+1)
	if len(q) == 0 {
		rows = append(rows, muted.Render("Your queue is empty"), muted.Render("a adds a selected song"))
	}
	for i := offset; i < min(len(q), offset+available); i++ {
		prefix := "  "
		if i == m.State.Index {
			if m.State.Playback.Playing {
				prefix = "▶ "
			} else {
				prefix = "• "
			}
		}
		label := prefix + plain(q[i].Title)
		if i == cursor && m.UI.Focus == 2 {
			label = selected.Render(label)
		} else if i == m.State.Index {
			label = accent.Render(label)
		}
		rows = append(rows, label)
	}
	for len(rows) < h-1 {
		rows = append(rows, "")
	}
	rows = append(rows, muted.Render(fmt.Sprintf("%d tracks  ·  J/K reorder", len(q))))
	return rows
}

func (m *Model) View() tea.View { return m.ViewWithContent(nil) }
func (m *Model) ViewWithContent(override *ContentPanel) tea.View {
	w, h := m.width, m.height
	if w < 60 || h < 16 {
		label := "q opens the exit prompt."
		if m.prompt {
			label = "Leave lazymusic?\nq Background · Q Quit · Esc Cancel"
		}
		v := tea.NewView(fmt.Sprintf("lazymusic\nResize terminal to at least 60×16 (currently %d×%d).\n%s", w, h, label))
		v.AltScreen = true
		v.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
		return v
	}
	if m.qualityDialog {
		rows := []string{}
		for i, q := range m.State.Source.Qualities {
			label := "  " + q.Name
			if i == m.qualityCursor {
				label = selected.Render("› " + q.Name)
			}
			rows = append(rows, label)
		}
		rows = append(rows, "", "Enter select · Esc cancel")
		v := tea.NewView(lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box("Preferred quality", rows, min(60, w-4), len(rows)+2, true)))
		v.AltScreen = true
		v.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
		return v
	}
	if m.prompt || m.help {
		rows := []string{}
		title := "Leave lazymusic?"
		if m.help {
			title = "Keyboard · Vim mode"
			rows = []string{"j/k / ↑↓    Move through a list", "Ctrl-h/l    Left/right panel · Ctrl-j/k Down/up", "gg / G      Top / bottom    Ctrl-u/d Half page", "h/l         Parent/child   Enter Open/play", "Ctrl-f / i  Search query    S Search scope", "L           Login / switch music source", ", / .       Change category / search type", "/           Filter loaded items", "Space       Play/pause      x Stop", "[ / ]       Previous/next   < / > Seek ±10s", "= / -       System volume   b Quality", "+ / _       Cycle layouts   @ Runtime logs", "Ctrl +/-    Zen · Ctrl-arrows resize · Ctrl-= reset", "6 Sound     ,/. Pulse/Wave/EQ · h/l band j/k gain", "Player ,/.  Playback mode   r Repeat  z Shuffle", "a / A       Add to queue / play next", "p           Play album/playlist", "Queue: d remove · c clear · J/K reorder", ":seek 30    :vol 50  :pause  :play", "", "q / Esc closes help · keys.json customizes keys"}
		} else {
			rows = []string{"Your queue, position and view are saved.", "", accent.Bold(true).Render("q   Keep running in background"), "    Close the TUI; music keeps playing.", "", accent.Bold(true).Render("Q   Quit completely"), "    Stop playback and exit the background service.", "", muted.Render("Esc Return to music")}
			if m.exiting {
				rows = append(rows, "", good.Render("Saving session…"))
			}
		}
		if m.problem != "" {
			rows = append(rows, "", accent.Render(plain(m.problem)))
		}
		modal := box(title, rows, min(w-4, 70), min(h-2, len(rows)+2), true)
		v := tea.NewView(lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal))
		v.AltScreen = true
		v.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
		return v
	}
	backend := m.State.Source.Name
	if backend == "" {
		backend = "MUSIC"
	}
	title := m.UI.Location.Title
	if override != nil {
		title = override.Title
	}
	header := fit(accent.Bold(true).Render(" lazymusic ")+muted.Render(" / ")+text.Render(plain(title)), max(0, w-ansi.StringWidth(backend)-2)) + muted.Render(backend) + "  "
	bodyH := m.bodyHeight()
	navW := m.navWidth()
	queueW := m.queueWidth()
	showQueue := w >= 100
	mainW := w - navW
	if showQueue {
		mainW -= queueW
	}
	navRows := []string{}
	for i, entry := range m.navItems() {
		line := "  " + entry.title
		if i == m.UI.NavIndex {
			line = selected.Render("› " + entry.title)
		}
		navRows = append(navRows, line)
	}
	navH := m.navHeight()
	navStart := max(0, m.UI.NavIndex-(navH-3))
	navRows = navRows[navStart:min(len(navRows), navStart+navH-2)]
	sidebar := lipgloss.JoinVertical(lipgloss.Left,
		box("1 Navigation", navRows, navW, navH, m.UI.Focus == 0),
		box(m.soundTitle(), m.soundRows(navW-4, bodyH-navH-2), navW, bodyH-navH, m.UI.Focus == 5))
	mainTitle := "2 " + plain(title)
	mainRows := m.mainRows(bodyH - 2)
	if override != nil {
		mainRows = override.Rows
	}
	var body string
	if m.UI.Zen {
		body = m.zenBody(w, bodyH)
	} else if m.UI.Layout == "content" {
		body = box(mainTitle, mainRows, w, bodyH, m.UI.Focus == 1)
	} else if m.UI.Layout == "queue" {
		body = box("3 Queue", m.queueRows(bodyH-2), w, bodyH, m.UI.Focus == 2)
	} else {
		if !showQueue && m.UI.Focus == 2 {
			mainTitle = "3 Queue"
			mainRows = m.queueRows(bodyH - 2)
		}
		columns := []string{sidebar, box(mainTitle, mainRows, mainW, bodyH, m.UI.Focus == 1 || !showQueue && m.UI.Focus == 2)}
		if showQueue {
			columns = append(columns, box("3 Queue", m.queueRows(bodyH-2), queueW, bodyH, m.UI.Focus == 2))
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, columns...)
	}
	if m.UI.ShowLogs {
		body += "\n" + box("5 Log", m.logRows(m.logHeight()-2), w, m.logHeight(), m.UI.Focus == 4)
	}
	s := m.State.Playback
	if s.Track == nil && !s.Loading && !s.Playing {
		s.Status = "stopped"
	}
	track := "Nothing playing — choose a song and press Enter"
	if s.Track != nil {
		track = plain(s.Track.Title) + muted.Render("  ·  "+plain(s.Track.Artist)+"  ·  "+plain(s.Track.Album))
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
	barW := max(8, w-48)
	filled := 0
	total := time.Duration(0)
	if s.Track != nil {
		total = s.Track.Duration
		if total > 0 {
			filled = int(float64(s.Position) / float64(total) * float64(barW))
		}
	}
	filled = max(0, min(barW, filled))
	bar := accent.Render(strings.Repeat("━", filled)) + muted.Render(strings.Repeat("─", barW-filled))
	progress := good.Render(playbackIcon(s.Status, s.Playing, s.Loading)+" "+status) + "  " + bar + "  " + duration(s.Position) + " / " + duration(total) + muted.Render(fmt.Sprintf("  vol %.0f%%", s.Volume*100))
	if !m.State.SystemVolume.Available {
		progress = good.Render(playbackIcon(s.Status, s.Playing, s.Loading)+" "+status) + "  " + bar + "  " + duration(s.Position) + " / " + duration(total) + muted.Render("  system vol —")
	}
	quality := "Original"
	if m.State.PreferredQuality != "" {
		for _, q := range m.State.Source.Qualities {
			if q.ID == m.State.PreferredQuality {
				quality = q.Name + " preferred"
			}
		}
	}
	playerBox := box("4 Now playing · "+playbackMode(s.RepeatMode, s.ShuffleMode)+" · "+quality, []string{track, progress}, w, m.playerHeight(), m.UI.Focus == 3)
	label := strings.ToUpper(m.UI.NavPage)
	if label == "" {
		label = strings.ToUpper(m.UI.Location.Page)
	}
	if override != nil {
		label = strings.ToUpper(override.Title)
	}
	footer := muted.Render(" " + label + "  h/l navigate · +/_ layout · @ logs · ? help · q leave")
	if m.UI.Focus == 2 {
		footer = muted.Render(" QUEUE  Enter play · d remove · J/K move · c clear · @ logs")
	}
	if m.UI.Focus == 5 {
		footer = muted.Render(" SOUND  ,/. view · Ctrl-arrows resize · Ctrl-= reset")
		if m.UI.Visualizer == 2 || m.UI.Zen {
			footer = muted.Render(" EQUALIZER  h/l band · j/k ±1dB · Space bypass · r flat · p preset")
		}
	}
	statusLine := m.notice
	if m.UI.Focus == 3 {
		footer = muted.Render(" PLAYER  Space play/pause · ,/. mode · b quality · =/- system volume · @ logs")
	}
	if m.UI.Focus == 4 {
		footer = muted.Render(" LOG  j/k scroll · gg/G first/latest · @ close")
	}
	if m.loading {
		statusLine = "Loading…"
	}
	if m.problem != "" {
		statusLine = m.problem
	}
	if m.mode != "" {
		label := map[string]string{"filter": "FILTER / ", "query": "SEARCH > ", "command": ":"}[m.mode]
		statusLine = accent.Bold(true).Render(label) + plain(m.draft) + "▏"
		footer = muted.Render(" INPUT  Enter confirm · Esc cancel · Ctrl-u clear")
	}
	if m.toast != nil {
		statusLine = accent.Bold(true).Render(" " + strings.ToUpper(m.toast.Level) + " · " + plain(m.toast.Message))
	}
	v := tea.NewView(strings.Join([]string{fit(header, w), body, playerBox, fit(footer, w), fit(statusLine, w)}, "\n"))
	v.AltScreen = true
	v.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
	return v
}

func (m *Model) scopeHint() string {
	if m.State.Source.Capabilities.CatalogSearch {
		return "   S scope · i edit"
	}
	return "   i edit"
}

func (m *Model) homeTabBar() string {
	if len(m.tabs) == 0 {
		return muted.Render("Home · loading categories…")
	}
	width := m.width - m.navWidth() - 4
	if m.UI.Layout == "content" {
		width = m.width - 4
	}
	if m.width >= 100 && m.UI.Layout != "content" {
		width -= m.queueWidth()
	}
	width = max(8, width)
	active := 0
	for i, t := range m.tabs {
		if t.ID == m.UI.Location.ID {
			active = i
			break
		}
	}
	labels := make([]string, len(m.tabs))
	for i, t := range m.tabs {
		labels[i] = ansi.Truncate(plain(t.Title), max(8, width-8), "…")
	}
	start := 0
	used := 0
	for i := 0; i <= active; i++ {
		used += ansi.StringWidth(labels[i]) + 3
	}
	for used > width-2 && start < active {
		used -= ansi.StringWidth(labels[start]) + 3
		start++
	}
	var parts []string
	used = 0
	if start > 0 {
		parts = append(parts, muted.Render("‹"))
		used = 2
	}
	for i := start; i < len(labels); i++ {
		label := labels[i]
		needed := ansi.StringWidth(label) + 3
		if used+needed > width && i > active {
			parts = append(parts, muted.Render("›"))
			break
		}
		if i == active {
			label = accent.Bold(true).Underline(true).Render(label)
		} else {
			label = muted.Render(label)
		}
		parts = append(parts, label)
		used += needed
	}
	return strings.Join(parts, " · ")
}
