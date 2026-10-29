package tui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"lazymusic/internal/catalog"
	"lazymusic/internal/daemon"
	"lazymusic/internal/eventlog"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
)

type API interface {
	State(context.Context) (daemon.State, error)
	Browse(context.Context, catalog.Query) (catalog.Page, error)
	Command(context.Context, daemon.Command) (daemon.State, error)
	SaveUI(context.Context, session.UI, uint64) error
	Detach(context.Context, session.UI, uint64) error
	Quit(context.Context) error
}

type stateMsg struct {
	s   daemon.State
	err error
}
type commandMsg struct {
	s   daemon.State
	err error
}
type pageMsg struct {
	key        string
	generation uint64
	append     bool
	page       catalog.Page
	err        error
}
type toastExpired uint64
type saveTick uint64
type savedMsg struct{ err error }
type exitMsg struct {
	action string
	err    error
}

type Model struct {
	work              *workDialog
	inspectGeneration uint64
	inspectCancel     context.CancelFunc
	toastVersion      uint64
	logOffset         int
	toast             *eventlog.Entry
	seenLog           uint64
	localLogs         []eventlog.Entry
	qualityDialog     bool
	qualityCursor     int
	api               API
	keys              Keys
	ctx               context.Context
	State             daemon.State
	UI                session.UI
	width, height     int
	items             []catalog.Item
	tabs              []catalog.Tab
	next              string
	nextSeen          map[string]bool
	loading           bool
	loadCancel        context.CancelFunc
	generation        uint64
	revision          uint64
	cursor, offset    int
	restore           session.Cursor
	restorePages      int
	restoring         bool
	mode              string
	draft             string
	prompt            bool
	help              bool
	gPrefix           bool
	exiting           bool
	ExitAction        string
	notice            string
	problem           string
}

func (m *Model) RevisionForExit() uint64 { m.remember(); m.revision++; return m.revision }

func New(ctx context.Context, api API, state daemon.State, keys Keys) *Model {
	if state.Source.ID == "" && state.Backend != "" {
		state.Source = music.Source{ID: state.Backend, Name: strings.ToUpper(state.Backend), Capabilities: music.Capabilities{Home: true, Library: true, Playlists: true, CatalogSearch: true}}
	}
	u := state.UI.Clone()
	if u.Location.Page == "" {
		u = session.DefaultUI()
	}
	if u.Cursors == nil {
		u.Cursors = map[string]session.Cursor{}
	}
	if u.LibrarySection == "" {
		u.LibrarySection = "songs"
	}
	if u.SearchSection == "" {
		u.SearchSection = "songs"
	}
	if u.SearchScope == "" {
		u.SearchScope = "catalog"
	}
	if keys == nil {
		keys = DefaultKeys()
	}
	m := &Model{api: api, keys: keys, ctx: ctx, State: state, UI: u, width: 100, height: 30}
	if u.Layout == "" {
		m.UI.Layout = "full"
	}
	for _, e := range state.Logs {
		if e.ID > m.seenLog {
			m.seenLog = e.ID
		}
	}
	m.restoreNavigation()
	m.restore = m.cursorFor(m.query())
	m.cursor = m.restore.Index
	m.offset = m.restore.Offset
	m.restoring = true
	return m
}

func (m *Model) Init() tea.Cmd { return tea.Batch(m.poll(), m.fetch(true, false)) }
func (m *Model) query() catalog.Query {
	l := m.UI.Location
	return catalog.Query{Source: m.State.Source.ID, Page: l.Page, Section: l.Section, Scope: l.Scope, Term: l.Query, Kind: l.Kind, ID: l.ID}
}

func (m *Model) remember() {
	m.align()
	m.UI.QueueCursor = max(0, min(m.UI.QueueCursor, len(m.State.Queue)-1))
	items := m.visible()
	m.cursor = max(0, min(m.cursor, len(items)-1))
	id := ""
	if len(items) > 0 {
		id = items[m.cursor].CursorID()
	}
	m.UI.Cursors[m.query().Key()] = session.Cursor{Index: m.cursor, Offset: m.offset, ID: id}
}
func (m *Model) dirty() tea.Cmd {
	if !m.restoring {
		m.remember()
	}
	m.revision++
	rev := m.revision
	return tea.Tick(180*time.Millisecond, func(time.Time) tea.Msg { return saveTick(rev) })
}
func (m *Model) poll() tea.Cmd {
	return func() tea.Msg {
		select {
		case <-time.After(400 * time.Millisecond):
		case <-m.ctx.Done():
			return stateMsg{err: m.ctx.Err()}
		}
		ctx, cancel := context.WithTimeout(m.ctx, 3*time.Second)
		defer cancel()
		s, err := m.api.State(ctx)
		return stateMsg{s, err}
	}
}
func (m *Model) persist() tea.Cmd {
	u, rev := m.UI.Clone(), m.revision
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		return savedMsg{m.api.SaveUI(ctx, u, rev)}
	}
}
func (m *Model) command(c daemon.Command) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 85*time.Second)
		defer cancel()
		s, err := m.api.Command(ctx, c)
		return commandMsg{s, err}
	}
}

func (m *Model) leave(action string) tea.Cmd {
	m.remember()
	m.revision++
	u, rev := m.UI.Clone(), m.revision
	m.exiting = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		var err error
		if action == "background" || action == "login" {
			err = m.api.Detach(ctx, u, rev)
		} else {
			if err = m.api.SaveUI(ctx, u, rev); err == nil {
				err = m.api.Quit(ctx)
			}
		}
		return exitMsg{action, err}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	before := m.toastVersion
	_, cmd := m.update(msg)
	if before != m.toastVersion {
		cmd = tea.Batch(cmd, m.NotificationTimer())
	}
	return m, cmd
}
func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case detailsMsg:
		m.acceptDetails(msg)
		return m, nil
	case toastExpired:
		if uint64(msg) == m.toastVersion {
			m.toast = nil
			m.problem = ""
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.align()
		return m, m.dirty()
	case stateMsg:
		if msg.err != nil {
			m.problem = "Background unavailable: " + msg.err.Error()
			m.Report("error", "ipc", m.problem)
		} else if !msg.s.ObservedAt.Before(m.State.ObservedAt) {
			m.State = msg.s
			m.acceptLogs(msg.s.Logs)
			m.UI.QueueCursor = max(0, min(m.UI.QueueCursor, len(m.State.Queue)-1))
		}
		if m.exiting {
			return m, nil
		}
		return m, m.poll()
	case commandMsg:
		if msg.err != nil {
			m.problem = msg.err.Error()
			m.Report("error", "runtime", m.problem)
		} else {
			if !msg.s.ObservedAt.Before(m.State.ObservedAt) {
				m.State = msg.s
				m.acceptLogs(msg.s.Logs)
				m.UI.QueueCursor = max(0, min(m.UI.QueueCursor, len(m.State.Queue)-1))
			}
			m.problem = ""
		}
		return m, nil
	case pageMsg:
		if msg.generation != m.generation || msg.key != m.query().Key() {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			if m.ctx.Err() == nil && !errors.Is(msg.err, context.Canceled) {
				m.problem = msg.err.Error()
				m.Report("error", "runtime", m.problem)
			}
			return m, nil
		}
		if msg.append {
			m.items = append(m.items, msg.page.Items...)
		} else {
			m.items = msg.page.Items
		}
		if m.UI.Location.Page == "home" {
			m.tabs = msg.page.Tabs
			if msg.page.ActiveTab != "" {
				m.UI.Location.ID = msg.page.ActiveTab
				m.UI.HomeCategory = msg.page.ActiveTab
			}
		}
		m.next = msg.page.Next
		if m.next != "" {
			if m.nextSeen[m.next] {
				m.next = ""
				m.problem = "Source returned a repeated page; press R to refresh"
			} else {
				if m.nextSeen == nil {
					m.nextSeen = map[string]bool{}
				}
				m.nextSeen[m.next] = true
			}
		}
		if m.restoring {
			found := -1
			for i, v := range m.visible() {
				if (v.CursorID() == m.restore.ID || v.ID == m.restore.ID) && m.restore.ID != "" {
					found = i
					break
				}
			}
			if found >= 0 {
				m.cursor = found
				m.restoring = false
			} else if m.next != "" && m.restorePages < 100 && (m.restore.ID != "" || m.restore.Index >= len(m.visible())) {
				m.restorePages++
				return m, m.fetch(false, false)
			} else {
				m.cursor = max(0, min(m.restore.Index, len(m.visible())-1))
				m.restoring = false
			}
		}
		m.cursor = max(0, min(m.cursor, len(m.visible())-1))
		return m, m.dirty()
	case saveTick:
		if uint64(msg) == m.revision && !m.exiting {
			return m, m.persist()
		}
		return m, nil
	case savedMsg:
		if msg.err != nil && !m.exiting {
			m.problem = "Session save: " + msg.err.Error()
			m.Report("error", "session", m.problem)
		}
		return m, nil
	case exitMsg:
		if msg.err != nil {
			m.exiting = false
			m.problem = msg.err.Error()
			m.Report("error", "runtime", m.problem)
			m.prompt = false
			return m, nil
		}
		m.ExitAction = msg.action
		return m, tea.Quit
	case tea.PasteMsg:
		if m.mode != "" {
			m.draft += strings.ReplaceAll(msg.Content, "\n", " ")
		}
		return m, nil
	case tea.KeyPressMsg:
		if m.exiting {
			return m, nil
		}
		key := msg.String()
		if m.work != nil {
			return m, m.workKey(key)
		}
		if m.qualityDialog {
			switch key {
			case "esc":
				m.qualityDialog = false
			case "j", "down":
				m.qualityCursor = (m.qualityCursor + 1) % len(m.State.Source.Qualities)
			case "k", "up":
				m.qualityCursor = (m.qualityCursor + len(m.State.Source.Qualities) - 1) % len(m.State.Source.Qualities)
			case "enter":
				m.qualityDialog = false
				return m, m.command(daemon.Command{Op: "quality", Quality: m.State.Source.Qualities[m.qualityCursor].ID})
			}
			return m, nil
		}
		if key == "esc" && m.toast != nil {
			m.toast = nil
			m.problem = ""
			return m, nil
		}
		if m.mode != "" {
			return m, m.input(msg, key)
		}
		if m.prompt {
			switch key {
			case "q":
				return m, m.leave("background")
			case "Q":
				return m, m.leave("quit")
			case "esc":
				m.prompt = false
			}
			return m, nil
		}
		if m.help {
			if key == "esc" || key == "?" || key == "q" {
				m.help = false
			}
			return m, nil
		}
		return m, m.normal(key)
	}
	return m, nil
}
