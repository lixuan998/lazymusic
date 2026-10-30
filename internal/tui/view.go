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

// Keep printable input as terminal-generated text. Forcing every character to
// CSI-u without associated text makes Shift+/ indistinguishable from '/'.
// Basic disambiguation and alternate codes still support modified shortcuts.
func terminalView(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	v.KeyboardEnhancements.ReportAlternateKeys = true
	return v
}

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
	top := ""
	if title != "" {
		top = "─ " + plain(title) + " "
	}
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
	if len(m.tabs) > 0 || l.Page == "home" {
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
		rows = append(rows, muted.Render("Enter open/play   p/P pick   Ctrl+Enter play   y queue"))
	}
	if l.Page == "search" {
		scope := m.State.Source.Name + " catalog"
		if l.Scope == "library" {
			scope = "Library"
		}
		rows = append(rows, muted.Render(scope+"  ·  ")+plain(l.Query)+muted.Render(m.scopeHint()))
	} else if len(m.tabs) > 0 || l.Page == "home" {
		label := l.Title
		if l.Page == "home" {
			label = "Personal recommendations"
		}
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
			label = "No recommendations available; refresh to try again"
		}
		if l.Page == "search" && l.Query == "" {
			label = "Press i to search"
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
			if p := m.picked; p != nil && !p.fromQueue && p.origin == m.query().Key() && p.itemID == item.CursorID() {
				label = "◆ " + label
			}
			if (len(m.tabs) > 0 || l.Page == "home") && item.Track == nil {
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
	available := max(1, h-2)
	cursor, offset := m.queueViewport(available)
	if len(q) == 0 {
		rows = append(rows, muted.Render("Your queue is empty"), muted.Render("Content: y copies a song here"))
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
		if p := m.picked; p != nil && p.fromQueue && !p.placing && p.index == i {
			label = "◆ " + label
		}
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
	rows = append(rows, muted.Render(fmt.Sprintf("%d tracks  ·  [/] reorder", len(q))))
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
		v := terminalView(fmt.Sprintf("lazymusic\nResize terminal to at least 60×16 (currently %d×%d).\n%s", w, h, label))
		return v
	}
	if m.work != nil {
		return m.workView()
	}
	if m.edit != nil {
		return m.editView()
	}
	if m.help {
		return terminalView(m.helpView())
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
		if note := m.State.Source.QualityNote; note != "" {
			rows = append(rows, "")
			for _, line := range strings.Split(ansi.Wrap(plain(note), min(60, w-4)-4, ""), "\n") {
				rows = append(rows, muted.Render(line))
			}
		}
		rows = append(rows, "", "Enter select · Esc cancel")
		v := terminalView(lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box("Preferred quality", rows, min(60, w-4), len(rows)+2, true)))
		return v
	}
	if m.prompt {
		rows := []string{}
		title := "Leave lazymusic?"
		rows = []string{"Your queue, position and view are saved.", "", accent.Bold(true).Render("q   Keep running in background"), "    Close the TUI; music keeps playing.", "", accent.Bold(true).Render("Q   Quit completely"), "    Stop playback and exit the background service.", "", muted.Render("Esc Return to music")}
		if m.exiting {
			rows = append(rows, "", good.Render("Saving session…"))
		}
		if m.problem != "" {
			rows = append(rows, "", accent.Render(plain(m.problem)))
		}
		modal := box(title, rows, min(w-4, 70), min(h-2, len(rows)+2), true)
		v := terminalView(lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal))
		return v
	}
	title := m.UI.Location.Title
	if override != nil {
		title = override.Title
	}
	header := m.header(title, w)
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
	if bodyH < 6 {
		sidebar = box("1 Navigation", navRows, navW, bodyH, m.UI.Focus == 0)
		if m.UI.Focus == 5 {
			sidebar = box(m.soundTitle(), m.soundRows(navW-4, bodyH-2), navW, bodyH, true)
		}
	}
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
	playerBox := m.playerBox(w)
	label := strings.ToUpper(m.UI.NavPage)
	if label == "" {
		label = strings.ToUpper(m.UI.Location.Page)
	}
	if override != nil {
		label = strings.ToUpper(override.Title)
	}
	footer := muted.Render(" " + label + "  h/l navigate · +/_ layout · @ logs · ? help · q leave")
	if m.UI.Focus == 1 {
		footer = muted.Render(" " + label + "  h/l open/back · p/P pick · Ctrl+Enter play · y queue · a add · ? help")
	}
	if m.UI.Focus == 2 {
		footer = muted.Render(" QUEUE  p/P pick · f current · Enter play · [/] move · d remove · ? help")
		if m.picked != nil {
			footer = muted.Render(" QUEUE  p after · P before · Esc cancel · f current · Enter play · ? help")
			if m.picked.placing {
				footer = muted.Render(" QUEUE  Placing selection… · f current · ? help")
			}
		}
	}
	if m.UI.Focus == 5 {
		footer = muted.Render(" SOUND  ,/. view · HJKL resize · Ctrl-= reset")
		if m.UI.Visualizer == turntableView {
			footer = muted.Render(" TURNTABLE  ,/. view · Space play/pause · HJKL resize · ? help")
		}
		if m.UI.Visualizer == 2 || m.UI.Zen {
			footer = muted.Render(" EQUALIZER  h/l band · j/k ±1dB · e bypass · r flat · p preset · ? help")
		}
		if m.UI.Zen {
			footer = muted.Render(" ZEN  ,/. Pulse/Wave/Turntable · h/l band · j/k gain · e bypass · ? help")
		}
	}
	statusLine := m.notice
	if m.picked != nil {
		statusLine = m.pickedStatus()
	}
	if m.UI.Focus == 3 {
		footer = muted.Render(" PLAYER  Space play/pause · ,/. mode · b quality · o links · c credits · @ logs")
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
	v := terminalView(strings.Join([]string{header, body, playerBox, fit(footer, w), fit(statusLine, w)}, "\n"))
	return v
}

func (m *Model) scopeHint() string {
	if m.State.Source.Capabilities.CatalogSearch {
		return "   s scope · i edit"
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
