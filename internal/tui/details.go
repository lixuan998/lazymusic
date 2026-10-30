package tui

import (
	"context"
	"errors"
	"lazymusic/internal/catalog"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type workDialog struct {
	subject catalog.Subject
	mode    string
	title   string
	loading bool
	cursor  int
	scroll  int
	details catalog.Details
}

type detailsMsg struct {
	generation uint64
	details    catalog.Details
	err        error
}

func (m *Model) workSubject() (catalog.Subject, string, bool) {
	var track *music.Track
	if m.UI.Focus == 1 {
		items := m.visible()
		if m.LoginSelected() || m.cursor < 0 || m.cursor >= len(items) {
			return catalog.Subject{}, "", false
		}
		item := items[m.cursor]
		subject := catalog.Subject{Source: item.Source, ID: item.ID, Kind: item.Kind}
		if item.Track != nil {
			subject.PlaybackID = item.Track.PlaybackID
		}
		if subject.Source == "" {
			subject.Source = m.State.Source.ID
		}
		return subject, item.Title, item.ID != ""
	} else if m.UI.Focus == 2 && m.UI.QueueCursor >= 0 && m.UI.QueueCursor < len(m.State.Queue) {
		track = &m.State.Queue[m.UI.QueueCursor]
	} else if m.UI.Focus == 3 {
		track = m.State.Playback.Track
		if track == nil && m.State.Index >= 0 && m.State.Index < len(m.State.Queue) {
			track = &m.State.Queue[m.State.Index]
		}
	}
	if track == nil || track.ID == "" {
		return catalog.Subject{}, "", false
	}
	source := track.Source
	if source == "" {
		source = m.State.Source.ID
	}
	return catalog.Subject{Kind: track.Kind, Source: source, ID: track.ID, PlaybackID: track.PlaybackID}, track.Title, true
}

func (m *Model) inspectWork(mode string) tea.Cmd {
	subject, title, ok := m.workSubject()
	if !ok {
		return nil
	}
	return m.inspectSubject(subject, title, mode)
}

func (m *Model) inspectSubject(subject catalog.Subject, title, mode string) tea.Cmd {
	m.closeWork()
	subject.Credits = mode == "credits"
	m.work = &workDialog{subject: subject, mode: mode, title: title, loading: true}
	generation := m.inspectGeneration
	inspector, ok := m.api.(catalog.Inspector)
	if !ok {
		m.work.loading = false
		m.work.details.CreditsNote = "This source does not provide contributor credits."
		return nil
	}
	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	m.inspectCancel = cancel
	return func() tea.Msg {
		defer cancel()
		details, err := inspector.Inspect(ctx, subject)
		return detailsMsg{generation: generation, details: details, err: err}
	}
}

func (m *Model) closeWork() {
	if m.inspectCancel != nil {
		m.inspectCancel()
		m.inspectCancel = nil
	}
	m.inspectGeneration++
	m.work = nil
}

func (m *Model) acceptDetails(msg detailsMsg) {
	if m.work == nil || msg.generation != m.inspectGeneration {
		return
	}
	if msg.err != nil {
		m.closeWork()
		if m.ctx.Err() == nil && !errors.Is(msg.err, context.Canceled) {
			m.Report("error", "details", msg.err.Error())
		}
		return
	}
	m.work.loading = false
	m.work.details = msg.details
	if msg.details.Title != "" {
		m.work.title = msg.details.Title
	}
}

func (m *Model) workKey(key string) tea.Cmd {
	d := m.work
	action := m.keys.ActionFor("Dialog", key)
	if key == "esc" || key == "q" || key == "ctrl+c" || action == "back" {
		m.closeWork()
		return nil
	}
	if action == "credits" || action == "related" {
		if action == "credits" && !d.subject.Credits {
			return m.inspectSubject(d.subject, d.title, "credits")
		}
		d.mode = action
		d.scroll = 0
		return nil
	}
	if d.loading {
		return nil
	}
	if d.mode == "related" {
		n := len(d.details.Links)
		if n == 0 {
			return nil
		}
		switch action {
		case "up":
			d.cursor = (d.cursor + n - 1) % n
		case "down":
			d.cursor = (d.cursor + 1) % n
		case "open", "activate":
			item := d.details.Links[d.cursor]
			if item.Open == nil || (item.Open.Source != "" && item.Open.Source != m.State.Source.ID) {
				return nil
			}
			q := *item.Open
			m.closeWork()
			m.UI.Focus, m.UI.LastMainFocus = 1, 1
			m.UI.Zen = false
			if m.UI.Layout == "queue" {
				m.UI.Layout = "content"
			}
			m.UI.NavIndex = m.findNav(m.UI.NavPage)
			return m.navigate(session.Location{Source: q.Source, Page: q.Page, Section: q.Section, Scope: q.Scope, Kind: q.Kind, ID: q.ID, Query: q.Term, Title: item.Title, Playback: item.Play}, true)
		}
	} else {
		step := 1
		if action == "page_down" || action == "page_up" {
			step = max(1, m.height-10)
		}
		switch action {
		case "up", "page_up":
			d.scroll = max(0, d.scroll-step)
		case "down", "page_down":
			d.scroll += step
		case "top":
			d.scroll = 0
		case "bottom":
			d.scroll = 1 << 20
		}
	}
	return nil
}

func (m *Model) workView() tea.View {
	d := m.work
	w := min(78, m.width-4)
	var content []string
	title := "Go to"
	if d.mode == "credits" {
		title = "Credits"
	}
	if d.loading {
		content = []string{muted.Render("Loading…")}
	} else if d.mode == "related" {
		for i, item := range d.details.Links {
			kind := "Artist"
			if strings.HasSuffix(item.Kind, "albums") {
				kind = "Album"
			}
			row := "  " + kind + " · " + plain(item.Title)
			if i == d.cursor {
				row = selected.Render(fit("› "+kind+" · "+plain(item.Title), w-4))
			}
			content = append(content, row)
		}
		if len(content) == 0 {
			content = []string{muted.Render("No album or artist links available for this work.")}
		}
	} else {
		group := ""
		for _, credit := range d.details.Credits {
			if credit.Group != group {
				if len(content) > 0 {
					content = append(content, "")
				}
				content = append(content, accent.Bold(true).Render(plain(credit.Group)))
				group = credit.Group
			}
			content = append(content, strings.Split(ansi.Wrap(plain(credit.Name), w-6, ""), "\n")...)
			if len(credit.Roles) > 0 {
				for _, row := range strings.Split(ansi.Wrap(plain(strings.Join(credit.Roles, " · ")), w-6, ""), "\n") {
					content = append(content, muted.Render("  "+row))
				}
			}
		}
		if len(content) == 0 {
			content = append(content, muted.Render("No contributor credits provided."))
		}
		if d.details.CreditsNote != "" {
			content = append(content, "")
			for _, row := range strings.Split(ansi.Wrap(plain(d.details.CreditsNote), w-4, ""), "\n") {
				content = append(content, muted.Render(row))
			}
		}
	}
	rows := []string{text.Bold(true).Render(fit(plain(d.title), w-4)), ""}
	visible := max(1, m.height-10)
	if d.mode == "related" {
		d.scroll = min(max(0, d.cursor-visible+1), max(0, len(content)-visible))
	} else {
		d.scroll = min(d.scroll, max(0, len(content)-visible))
	}
	rows = append(rows, content[d.scroll:min(len(content), d.scroll+visible)]...)
	hint := "j/k select · Enter/l open · c credits · Esc close"
	if d.mode == "credits" {
		hint = "j/k scroll · Ctrl-u/d page · o go to · Esc close"
	}
	rows = append(rows, "", muted.Render(hint))
	return terminalView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box(title, rows, w, len(rows)+2, true)))
}
