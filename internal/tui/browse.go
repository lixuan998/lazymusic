package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"lazymusic/internal/catalog"
	"lazymusic/internal/daemon"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
	"strings"
	"time"
)

func (m *Model) visible() []catalog.Item {
	if m.UI.Filter == "" {
		return m.items
	}
	out := []catalog.Item{}
	term := strings.ToLower(m.UI.Filter)
	for _, i := range m.items {
		if strings.Contains(strings.ToLower(i.Title+" "+i.Subtitle), term) {
			out = append(out, i)
		}
	}
	return out
}
func (m *Model) fetch(reset, refresh bool) tea.Cmd {
	q := m.query()
	if reset {
		if m.loadCancel != nil {
			m.loadCancel()
		}
		m.generation++
		m.items = nil
		m.next = ""
		m.restorePages = 0
		m.nextSeen = map[string]bool{}
	}
	if q.Page == "login" || (q.Page == "search" && strings.TrimSpace(q.Term) == "") {
		m.items = nil
		m.next = ""
		m.loading = false
		m.restoring = false
		return nil
	}
	q.Refresh = refresh
	if !reset {
		q.Next = m.next
	}
	key, generation := q.Key(), m.generation
	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	m.loadCancel = cancel
	m.loading = true
	m.problem = ""
	return func() tea.Msg {
		defer cancel()
		p, err := m.api.Browse(ctx, q)
		return pageMsg{key: key, generation: generation, append: !reset, page: p, err: err}
	}
}
func (m *Model) navigate(l session.Location, push bool) tea.Cmd {
	m.remember()
	if push {
		m.UI.History = append(m.UI.History, m.UI.Location)
	}
	m.UI.Location = l
	if l.Page == "home" && l.ID != "" {
		m.UI.HomeCategory = l.ID
	}
	m.UI.Filter = ""
	m.problem = ""
	m.restore = m.cursorFor(m.query())
	m.cursor = m.restore.Index
	m.offset = m.restore.Offset
	m.restoring = true
	return tea.Batch(m.fetch(true, false), m.dirty())
}
func (m *Model) activateNav() tea.Cmd {
	if m.UI.NavIndex == m.findNav("login") {
		return m.OpenLogin()
	}
	m.UI.History = nil
	nav := m.navItems()
	m.UI.NavIndex = max(0, min(m.UI.NavIndex, len(nav)-1))
	n := nav[m.UI.NavIndex]
	m.UI.NavPage = n.page
	l := session.Location{Source: m.State.Source.ID, Page: n.page, Section: "songs", Title: n.title}
	switch n.page {
	case "home":
		l.ID = m.UI.HomeCategory
	case "library":
		l.Section = m.UI.LibrarySection
	case "playlists":
		l.Section = "playlists"
	case "search":
		l.Section = m.UI.SearchSection
		l.Scope = m.UI.SearchScope
		l.Query = m.UI.SearchQuery
	}
	return m.navigate(l, false)
}
func (m *Model) category(next bool) tea.Cmd {
	l := m.UI.Location
	if l.Page == "home" {
		if len(m.tabs) == 0 {
			return nil
		}
		i := 0
		for n, t := range m.tabs {
			if t.ID == l.ID {
				i = n
				break
			}
		}
		if next {
			i = (i + 1) % len(m.tabs)
		} else {
			i = (i + len(m.tabs) - 1) % len(m.tabs)
		}
		q := m.tabs[i].Query
		l.Source = q.Source
		l.ID = q.ID
		l.Section = q.Section
		return m.navigate(l, false)
	}
	if l.Page != "library" && l.Page != "search" {
		return nil
	}
	sections := []string{"songs", "albums", "artists"}
	if l.Page == "search" && m.State.Source.Capabilities.Playlists {
		sections = append(sections, "playlists")
	}
	i := 0
	for n, s := range sections {
		if s == l.Section {
			i = n
		}
	}
	if next {
		i = (i + 1) % len(sections)
	} else {
		i = (i + len(sections) - 1) % len(sections)
	}
	l.Section = sections[i]
	if l.Page == "library" {
		m.UI.LibrarySection = l.Section
	} else {
		m.UI.SearchSection = l.Section
	}
	return m.navigate(l, false)
}

func (m *Model) selectItem(op string) tea.Cmd {
	if m.UI.Focus == 2 {
		if op == "replace" || op == "play-selection" {
			return m.command(daemon.Command{Op: "select", Index: m.UI.QueueCursor})
		}
		return nil
	}
	items := m.visible()
	if len(items) == 0 || m.cursor >= len(items) {
		return nil
	}
	item := items[m.cursor]
	if item.Track != nil {
		actual := op
		if actual == "play-selection" {
			actual = "replace"
		}
		cmd := daemon.Command{Op: actual, Tracks: []music.Track{*item.Track}}
		if actual == "replace" && m.UI.Location.Playback != nil {
			q := *m.UI.Location.Playback
			cmd.Query = &q
			cmd.StartID = item.ID
		}
		return m.command(cmd)
	}
	if op == "replace" {
		return m.openChild()
	}
	if item.Play == nil {
		m.notice = "Open this item and choose a playable collection"
		return nil
	}
	actual := op
	if actual == "play-selection" {
		actual = "replace"
	}
	q := *item.Play
	return m.command(daemon.Command{Op: actual, Query: &q})
}
