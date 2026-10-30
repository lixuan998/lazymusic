package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) actionEnabled(action string) bool {
	if action == "" {
		return false
	}
	switch action {
	case "query":
		return m.UI.Location.Page == "search"
	case "scope":
		return m.UI.Location.Page == "search" && m.State.Source.Capabilities.CatalogSearch
	case "category_next", "category_previous":
		return m.UI.Focus != 1 || len(m.tabs) > 0 || m.UI.Location.Page == "home" || m.UI.Location.Page == "library" || m.UI.Location.Page == "search"
	case "eq_toggle", "eq_reset", "eq_preset":
		return m.State.Source.Capabilities.Equalizer && (m.UI.Zen || m.UI.Visualizer == 2)
	case "open", "back", "up", "down":
		if m.UI.Focus == 5 {
			return m.State.Source.Capabilities.Equalizer && (m.UI.Zen || m.UI.Visualizer == 2)
		}
	case "remove":
		return m.UI.Focus == 2 || m.UI.Location.Page == "library" || m.UI.Location.Page == "playlists" || m.UI.Location.Scope == "library"
	case "cancel_pick":
		return m.picked != nil && !m.picked.placing
	case "undo_placement":
		return !m.placementBusy && !(m.picked != nil && m.picked.placing)
	}
	return true
}

func (m *Model) helpDescription(d keyDefinition) string {
	switch d.action {
	case "undo_placement":
		if m.State.Placement != nil && m.State.Placement.Undone {
			return "Redo last pick & place (u toggles)"
		}
		return "Undo last pick & place (u toggles)"
	case "pick_after", "pick_before":
		if m.UI.Focus == 2 && m.picked != nil {
			if d.action == "pick_before" {
				return "Place picked selection before cursor"
			}
			return "Place picked selection after cursor"
		}
		return "Pick selected song / collection"
	case "add":
		if m.UI.Focus == 1 && m.UI.Location.Page == "playlists" && m.State.Source.Capabilities.PlaylistCreate {
			return "Create playlist"
		}
	case "remove":
		if m.UI.Focus == 2 {
			return "Remove song from playback queue"
		}
		return "Remove library item (source limits apply)"
	case "category_next", "category_previous":
		noun := "category"
		if m.UI.Focus == 0 || m.UI.Focus == 5 {
			noun = "visualization"
		}
		if m.UI.Focus == 3 {
			noun = "playback mode"
		}
		if d.action == "category_previous" {
			return "Previous " + noun
		}
		return "Next " + noun
	case "open":
		if m.UI.Focus == 5 {
			return "Next EQ frequency band"
		}
	case "back":
		if m.UI.Focus == 1 && m.picked != nil {
			return "Back / Esc cancels picked selection"
		}
		if m.UI.Focus == 5 {
			return "Previous EQ frequency band"
		}
	case "up":
		if m.UI.Focus == 5 {
			return "EQ gain +1 dB"
		}
	case "down":
		if m.UI.Focus == 5 {
			return "EQ gain -1 dB"
		}
	}
	return d.description
}

func (m *Model) helpLines() []string {
	w := min(86, m.width-4) - 4
	var rows []string
	scope := m.keyScope()
	for _, group := range []string{scope, "Global"} {
		if len(rows) > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, accent.Bold(true).Render(group))
		for _, d := range keyDefinitions {
			if !inScope(d, group) || !m.actionEnabled(d.action) || len(m.keys[d.action]) == 0 {
				continue
			}
			bindings := strings.Join(m.keys[d.action], " / ")
			// Alternate terminal encodings for the same physical shortcut are aliases.
			if d.action == "zen" && slices.Equal(m.keys[d.action], d.bindings) {
				bindings = "Ctrl++ / Ctrl+-"
			}
			if d.action == "layout_next" && slices.Equal(m.keys[d.action], d.bindings) {
				bindings = m.keys[d.action][0]
			}
			if d.action == "layout_previous" && slices.Equal(m.keys[d.action], d.bindings) {
				bindings = m.keys[d.action][0]
			}
			label := bindings + "  " + m.helpDescription(d)
			for _, line := range strings.Split(ansi.Wrap(label, w, ""), "\n") {
				rows = append(rows, line)
			}
		}
	}
	return rows
}

func (m *Model) helpKey(key string) {
	if key == "esc" || key == "q" || m.keys.ActionFor("Global", key) == "help" {
		m.help = false
		return
	}
	action := m.keys.ActionFor("Dialog", key)
	step := 1
	if action == "page_up" || action == "page_down" {
		step = max(1, m.height-8) / 2
	}
	switch action {
	case "down", "page_down":
		m.helpScroll += step
	case "up", "page_up":
		m.helpScroll = max(0, m.helpScroll-step)
	case "top":
		m.helpScroll = 0
	case "bottom":
		m.helpScroll = 1 << 20
	}
	if key == "g" {
		if m.gPrefix {
			m.helpScroll = 0
			m.gPrefix = false
		} else {
			m.gPrefix = true
		}
	} else {
		m.gPrefix = false
	}
}

func (m *Model) helpView() string {
	rows := m.helpLines()
	available := max(1, m.height-7)
	m.helpScroll = min(m.helpScroll, max(0, len(rows)-available))
	end := min(len(rows), m.helpScroll+available)
	visible := append([]string(nil), rows[m.helpScroll:end]...)
	visible = append(visible, "", muted.Render(fmt.Sprintf("j/k scroll · Ctrl-u/d page · ?/Esc close · %d/%d", end, len(rows))))
	modal := box("Keys · "+m.keyScope()+" + Global", visible, min(86, m.width-4), len(visible)+2, true)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
}
