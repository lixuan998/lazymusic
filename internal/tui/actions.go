package tui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"lazymusic/internal/catalog"
	"lazymusic/internal/daemon"
	"lazymusic/internal/music"
	"strconv"
	"strings"
	"time"
	"unicode"
)

func (m *Model) input(msg tea.KeyPressMsg, key string) tea.Cmd {
	switch key {
	case "esc", "ctrl+c":
		m.mode = ""
		m.draft = ""
		return nil
	case "enter":
		mode, text := m.mode, strings.TrimSpace(m.draft)
		m.mode = ""
		m.draft = ""
		if mode == "filter" {
			m.UI.Filter = text
			m.cursor = 0
			m.offset = 0
			return m.dirty()
		}
		if mode == "query" {
			m.UI.SearchQuery = text
			m.UI.NavIndex = m.findNav("search")
			m.UI.Focus = 1
			return m.activateNav()
		}
		return m.runCommand(text)
	case "backspace":
		r := []rune(m.draft)
		if len(r) > 0 {
			m.draft = string(r[:len(r)-1])
		}
	case "ctrl+u":
		m.draft = ""
	default:
		if msg.Text != "" && !msg.Mod.Contains(tea.ModCtrl) && !msg.Mod.Contains(tea.ModAlt) {
			m.draft += msg.Text
		} else if msg.Mod == 0 && unicode.IsPrint(msg.Code) {
			m.draft += string(msg.Code)
		}
	}
	return nil
}

func (m *Model) normal(key string) tea.Cmd {
	if key == "g" && m.keys.ActionFor(m.keyScope(), "gg") == "top" {
		if m.gPrefix {
			m.gPrefix = false
			return m.move("top")
		}
		m.gPrefix = true
		return nil
	}
	m.gPrefix = false
	action := m.keys.ActionFor(m.keyScope(), key)
	if !m.actionEnabled(action) {
		return nil
	}
	if m.picked != nil && m.picked.placing && m.UI.Focus == 2 {
		switch action {
		case "remove", "clear", "queue_up", "queue_down", "activate":
			return nil
		}
	}
	if strings.HasPrefix(action, "focus_") && len(action) == len("focus_1") {
		panel := action[len(action)-1]
		if panel != '6' {
			m.UI.Zen = false
		}
		if panel == '6' && !m.UI.Zen {
			m.UI.Layout = "full"
		}
		m.UI.Focus = int(panel - '1')
		if m.UI.Focus == 4 {
			m.UI.ShowLogs = true
		}
		if m.UI.Focus < 3 {
			if m.UI.Focus == 0 {
				m.UI.Layout = "full"
			} else if m.UI.Layout != "full" {
				if m.UI.Focus == 1 {
					m.UI.Layout = "content"
				} else {
					m.UI.Layout = "queue"
				}
			}
		}
		if m.UI.Focus < 3 || m.UI.Focus == 5 {
			m.UI.LastMainFocus = m.UI.Focus
		}
		return m.dirty()
	}
	if m.UI.Focus == 5 && (m.UI.Visualizer == 2 || m.UI.Zen) {
		if cmd, handled := m.equalizerKey(action, key); handled {
			return cmd
		}
	}
	switch action {
	case "related", "credits":
		return m.inspectWork(action)
	case "pick_after", "pick_before":
		return m.pickOrPlace(action == "pick_before")
	case "cancel_pick":
		return m.cancelPicked()
	case "zen":
		m.UI.Zen = !m.UI.Zen
		if m.UI.Zen {
			m.UI.Focus = 5
		} else if m.UI.Layout == "queue" {
			m.UI.Focus = 2
		} else {
			m.UI.Focus = 1
		}
		return m.dirty()
	case "layout_reset":
		m.UI.Zen = false
		m.UI.Layout = "full"
		m.UI.NavWidth = 0
		m.UI.QueueWidth = 0
		m.UI.NavHeight = 0
		m.UI.PlayerHeight = 0
		m.UI.LogHeight = 0
		m.UI.ZenEQWidth = 0
		m.UI.ShowLogs = false
		m.UI.Focus = 1
		return m.dirty()
	case "resize_up", "resize_down", "resize_left", "resize_right":
		return m.resize(action)
	case "login":
		return m.OpenLogin()
	case "leave":
		m.prompt = true
		return nil
	case "help":
		m.help = true
		m.helpScroll = 0
		return nil
	case "command":
		m.mode = "command"
		m.draft = ""
		return nil
	case "query":
		m.mode = "query"
		m.draft = m.UI.SearchQuery
		return nil
	case "filter":
		m.UI.Focus = 1
		m.mode = "filter"
		m.draft = m.UI.Filter
		return nil
	case "left", "right", "panel_down", "panel_up", "focus_next", "focus_previous":
		return m.focus(action)
	case "down", "up", "page_down", "page_up", "top", "bottom":
		return m.move(action)
	case "layout_next", "layout_previous":
		return m.cycleLayout(action == "layout_next")
	case "logs":
		m.UI.ShowLogs = !m.UI.ShowLogs
		if m.UI.ShowLogs {
			m.UI.Focus = 4
			m.logOffset = 0
		} else {
			m.UI.Focus = 1
		}
		return m.dirty()
	case "quality":
		if len(m.State.Source.Qualities) == 0 {
			m.notice = "This source plays its original file quality"
			return nil
		}
		m.qualityCursor = 0
		for i, q := range m.State.Source.Qualities {
			if q.ID == m.State.PreferredQuality {
				m.qualityCursor = i
			}
		}
		m.qualityDialog = true
		return nil
	case "category_next", "category_previous":
		if m.UI.Focus == 0 || m.UI.Focus == 5 {
			step := 1
			if action == "category_previous" {
				step = -1
			}
			m.UI.Visualizer = (m.UI.Visualizer + step + 3) % 3
			return m.dirty()
		}
		if m.UI.Focus == 3 {
			step := 1
			if action == "category_previous" {
				step = -1
			}
			return m.command(daemon.Command{Op: "playback-mode", ModeStep: step})
		}
		if m.UI.Focus == 1 {
			return m.category(action == "category_next")
		}
		return nil
	case "scope":
		if m.UI.Location.Page == "search" && m.State.Source.Capabilities.CatalogSearch {
			if m.UI.SearchScope == "library" {
				m.UI.SearchScope = "catalog"
			} else {
				m.UI.SearchScope = "library"
			}
			l := m.UI.Location
			l.Scope = m.UI.SearchScope
			return m.navigate(l, false)
		}
	case "open", "activate":
		if m.UI.Focus == 0 {
			m.UI.Focus = 1
			if m.UI.NavPage == "search" && m.UI.SearchQuery == "" {
				m.mode = "query"
				m.draft = ""
			}
			return tea.Batch(m.activateNav(), m.dirty())
		}
		if m.UI.Focus == 3 {
			return m.command(daemon.Command{Op: "toggle"})
		}
		if action == "open" {
			return m.openChild()
		}
		return m.selectItem("replace")
	case "back":
		if key == "esc" && m.picked != nil {
			return m.cancelPicked()
		}
		if m.UI.Focus != 1 {
			return nil
		}
		if m.UI.Filter != "" {
			m.UI.Filter = ""
			return m.dirty()
		}
		if len(m.UI.History) > 0 {
			l := m.UI.History[len(m.UI.History)-1]
			m.UI.History = m.UI.History[:len(m.UI.History)-1]
			return m.navigate(l, false)
		}
		return nil
	case "toggle":
		return m.command(daemon.Command{Op: "toggle"})
	case "playback_mode":
		return m.command(daemon.Command{Op: "playback-mode", ModeStep: 1})
	case "stop":
		return m.command(daemon.Command{Op: "stop"})
	case "next":
		return m.command(daemon.Command{Op: "next"})
	case "previous":
		return m.command(daemon.Command{Op: "previous"})
	case "seek_forward":
		return m.command(daemon.Command{Op: "seek", Delta: 10 * time.Second})
	case "seek_backward":
		return m.command(daemon.Command{Op: "seek", Delta: -10 * time.Second})
	case "volume_up":
		return m.command(daemon.Command{Op: "volume", Volume: min(1, m.State.Playback.Volume+0.05)})
	case "volume_down":
		return m.command(daemon.Command{Op: "volume", Volume: max(0, m.State.Playback.Volume-0.05)})
	case "repeat", "shuffle":
		return m.command(daemon.Command{Op: action})
	case "enqueue":
		return m.selectItem("append")
	case "play_next":
		return m.selectItem("play-next")
	case "play_selection":
		return m.selectItem("play-selection")
	case "queue_focus_playing":
		return m.focusPlayingQueue()
	case "remove":
		if m.UI.Focus == 2 {
			return m.command(daemon.Command{Op: "remove", Index: m.UI.QueueCursor, ExpectedQueue: music.QueueKey(m.State.Queue)})
		}
		return m.openEdit("remove")
	case "add":
		if m.mutationBusy {
			m.Report("warn", "catalog", "A library change is still in progress")
			return nil
		}
		if m.UI.Focus == 1 && m.UI.Location.Page == "playlists" && m.State.Source.Capabilities.PlaylistCreate {
			m.closeEdit()
			m.edit = &editDialog{phase: "name", operation: catalog.CreatePlaylist, title: "New playlist", origin: m.query().Key()}
			return nil
		}
		return m.openEdit("add")
	case "clear":
		if m.UI.Focus == 2 {
			return m.command(daemon.Command{Op: "clear"})
		}
	case "queue_up", "queue_down":
		if m.UI.Focus == 2 {
			to := m.UI.QueueCursor + 1
			if action == "queue_up" {
				to = m.UI.QueueCursor - 1
			}
			if to >= 0 && to < len(m.State.Queue) {
				from := m.UI.QueueCursor
				m.UI.QueueCursor = to
				return tea.Batch(m.command(daemon.Command{Op: "move", Index: from, To: to, ExpectedQueue: music.QueueKey(m.State.Queue)}), m.dirty())
			}
		}
	case "refresh":
		m.restore = m.cursorFor(m.query())
		m.restoring = true
		return m.fetch(true, true)
	}
	return nil
}

func (m *Model) runCommand(text string) tea.Cmd {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return nil
	}
	switch parts[0] {
	case "login", "sources":
		return m.OpenLogin()
	case "q", "quit":
		m.prompt = true
		return nil
	case "detach":
		return m.leave("background")
	case "play", "pause", "stop", "next", "previous", "clear":
		return m.command(daemon.Command{Op: parts[0]})
	case "seek":
		if len(parts) == 2 {
			seconds, err := strconv.ParseFloat(parts[1], 64)
			if err == nil {
				return m.command(daemon.Command{Op: "seek", Position: time.Duration(seconds * float64(time.Second))})
			}
		}
	case "vol":
		if len(parts) == 2 {
			v, err := strconv.ParseFloat(parts[1], 64)
			if err == nil {
				return m.command(daemon.Command{Op: "volume", Volume: v / 100})
			}
		}
	}
	m.problem = fmt.Sprintf("Unknown command: %s", text)
	return nil
}
