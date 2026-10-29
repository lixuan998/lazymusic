package tui

import (
	tea "charm.land/bubbletea/v2"
	"lazymusic/internal/catalog"
	"lazymusic/internal/session"
)

type navItem struct{ page, title string }

// Keep an independent queue viewport so focusing the current song can center
// it, and subsequent navigation scrolls only when the cursor leaves the view.
func (m *Model) queueViewport(rows int) (cursor, offset int) {
	rows = max(1, rows)
	cursor = max(0, min(len(m.State.Queue)-1, m.UI.QueueCursor))
	offset = max(0, min(max(0, len(m.State.Queue)-rows), m.UI.QueueOffset))
	if cursor < offset {
		offset = cursor
	} else if cursor >= offset+rows {
		offset = cursor - rows + 1
	}
	return cursor, offset
}

func (m *Model) alignQueue() {
	m.UI.QueueCursor, m.UI.QueueOffset = m.queueViewport(max(1, m.bodyHeight()-4))
}

func (m *Model) focusPlayingQueue() tea.Cmd {
	index := m.State.Index
	if index < 0 || index >= len(m.State.Queue) {
		return nil
	}
	rows := max(1, m.bodyHeight()-4)
	m.UI.QueueCursor = index
	m.UI.QueueOffset = max(0, min(index-rows/2, len(m.State.Queue)-rows))
	return m.dirty()
}

func (m *Model) navItems() []navItem {
	c := m.State.Source.Capabilities
	var out []navItem
	if c.Home {
		out = append(out, navItem{"home", "Home"})
	}
	if c.Library {
		out = append(out, navItem{"library", "Library"})
	}
	if c.Folders {
		out = append(out, navItem{"folders", "Folders"})
	}
	if c.Playlists {
		out = append(out, navItem{"playlists", "Playlists"})
	}
	if c.Library || c.CatalogSearch {
		out = append(out, navItem{"search", "Search"})
	}
	return append(out, navItem{"login", "Login"})
}
func (m *Model) findNav(page string) int {
	for i, n := range m.navItems() {
		if n.page == page {
			return i
		}
	}
	return 0
}
func (m *Model) restoreNavigation() {
	page := m.UI.NavPage
	hadPage := page != ""
	if page == "" {
		page = m.UI.Location.Page
		if len(m.UI.History) > 0 {
			page = m.UI.History[0].Page
		}
	}
	found := false
	for _, n := range m.navItems() {
		if n.page == page {
			found = true
		}
	}
	if !found {
		n := m.navItems()[0]
		m.UI.Location = session.Location{Source: m.State.Source.ID, Page: n.page, Section: "songs", Title: n.title}
		page = n.page
		m.UI.History = nil
	}
	m.UI.NavPage = page
	if !hadPage || !found || m.UI.NavIndex < 0 || m.UI.NavIndex >= len(m.navItems()) {
		m.UI.NavIndex = m.findNav(page)
	}
	if !m.State.Source.Capabilities.CatalogSearch {
		m.UI.SearchScope = "library"
		if m.UI.Location.Page == "search" {
			m.UI.Location.Scope = "library"
		}
	}
}
func (m *Model) cursorFor(q catalog.Query) session.Cursor {
	if c, ok := m.UI.Cursors[q.Key()]; ok {
		return c
	}
	q.Source = ""
	if c, ok := m.UI.Cursors[q.Key()]; ok {
		return c
	}
	// Pre-P4 snapshots used a provider name for the online search scope.
	if q.Scope == "catalog" {
		q.Scope = "apple"
		return m.UI.Cursors[q.Key()]
	}
	return session.Cursor{}
}
func (m *Model) focus(action string) tea.Cmd {
	current := m.UI.Focus
	panels := []int{0, 1, 2}
	if m.UI.Layout == "content" {
		panels = []int{1}
	} else if m.UI.Layout == "queue" {
		panels = []int{2}
	}
	if m.UI.Zen {
		panels = []int{5}
	} else if m.UI.Layout == "full" {
		panels = append(panels, 5)
	}
	if m.UI.ShowLogs {
		panels = append(panels, 4)
	}
	panels = append(panels, 3)
	index := 0
	for i, p := range panels {
		if p == current {
			index = i
		}
	}
	switch action {
	case "focus_next":
		m.UI.Focus = panels[(index+1)%len(panels)]
	case "focus_previous":
		m.UI.Focus = panels[(index+len(panels)-1)%len(panels)]
	case "left":
		if current < 3 && index > 0 {
			m.UI.Focus = panels[index-1]
		}
	case "right":
		if current == 5 && !m.UI.Zen {
			m.UI.Focus = 1
			break
		}
		if current < 3 && index+1 < len(panels) && panels[index+1] < 3 {
			m.UI.Focus = panels[index+1]
		}
	case "panel_down":
		if current == 0 && m.UI.Layout == "full" && !m.UI.Zen {
			m.UI.Focus = 5
		} else if current < 3 || current == 5 {
			if current < 3 || current == 5 {
				m.UI.LastMainFocus = current
			}
			if m.UI.ShowLogs {
				m.UI.Focus = 4
			} else {
				m.UI.Focus = 3
			}
		} else if current == 4 {
			m.UI.Focus = 3
		}
	case "panel_up":
		if current == 3 && m.UI.ShowLogs {
			m.UI.Focus = 4
		} else if current == 5 && !m.UI.Zen {
			m.UI.Focus = 0
		} else if current >= 3 {
			if m.UI.Zen {
				m.UI.Focus = 5
				break
			}
			m.UI.Focus = m.UI.LastMainFocus
			if m.UI.Layout == "content" {
				m.UI.Focus = 1
			} else if m.UI.Layout == "queue" {
				m.UI.Focus = 2
			}
		}
	}
	if m.UI.Focus < 3 || m.UI.Focus == 5 {
		m.UI.LastMainFocus = m.UI.Focus
	}
	return m.dirty()
}
func (m *Model) openChild() tea.Cmd {
	if m.UI.Focus != 1 {
		return nil
	}
	items := m.visible()
	if len(items) == 0 || m.cursor >= len(items) {
		return nil
	}
	item := items[m.cursor]
	if item.Open == nil {
		return nil
	}
	q := *item.Open
	return m.navigate(session.Location{Source: q.Source, Page: q.Page, Section: q.Section, Scope: q.Scope, Kind: q.Kind, ID: q.ID, Query: q.Term, Title: item.Title, Playback: item.Play}, true)
}

func (m *Model) move(action string) tea.Cmd {
	step := 1
	if action == "up" {
		step = -1
	}
	if action == "page_down" {
		step = max(1, m.height-12)
	}
	if action == "page_up" {
		step = -max(1, m.height-12)
	}
	if m.UI.Focus == 0 {
		count := len(m.navItems())
		m.UI.NavIndex = (m.UI.NavIndex + step%count + count) % count
		if action == "top" {
			m.UI.NavIndex = 0
		} else if action == "bottom" {
			m.UI.NavIndex = count - 1
		}
		if m.UI.NavIndex == m.findNav("login") {
			return m.dirty()
		}
		return tea.Batch(m.activateNav(), m.dirty())
	}
	if m.UI.Focus == 4 {
		if action == "top" {
			m.logOffset = max(0, len(m.allLogs())-1)
		} else if action == "bottom" {
			m.logOffset = 0
		} else {
			m.logOffset = max(0, m.logOffset-step)
		}
		return nil
	}
	if m.UI.Focus == 3 || m.UI.Focus == 5 {
		return nil
	}
	if m.UI.Focus == 2 {
		m.UI.QueueCursor = max(0, min(len(m.State.Queue)-1, m.UI.QueueCursor+step))
		if action == "top" {
			m.UI.QueueCursor = 0
		}
		if action == "bottom" {
			m.UI.QueueCursor = max(0, len(m.State.Queue)-1)
		}
		return m.dirty()
	}
	items := m.visible()
	if action == "top" {
		m.cursor = 0
	} else if action == "bottom" {
		m.cursor = max(0, len(items)-1)
	} else {
		m.cursor = max(0, min(len(items)-1, m.cursor+step))
	}
	if m.cursor >= len(items)-2 && m.next != "" && !m.loading {
		return tea.Batch(m.fetch(false, false), m.dirty())
	}
	return m.dirty()
}
