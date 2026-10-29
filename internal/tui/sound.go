package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"fmt"
	"lazymusic/internal/daemon"
	"lazymusic/internal/music"
	"strings"
)

func (m *Model) navWidth() int {
	w := m.UI.NavWidth
	if w == 0 {
		w = 22
	}
	reserve := 24
	if m.width >= 100 {
		reserve += 20
	}
	return min(max(18, w), max(18, m.width-reserve))
}
func (m *Model) queueWidth() int {
	w := m.UI.QueueWidth
	if w == 0 {
		w = max(24, min(34, m.width/4))
	}
	return min(max(20, w), max(20, m.width-m.navWidth()-24))
}
func (m *Model) navHeight() int {
	h := m.UI.NavHeight
	if h == 0 {
		h = max(len(m.navItems())+2, m.bodyHeight()/3)
	}
	return min(max(3, h), max(3, m.bodyHeight()-3))
}
func (m *Model) soundTitle() string {
	if m.UI.Visualizer == 2 {
		return "6 Equalizer"
	}
	if m.UI.Visualizer == 1 {
		return "6 Wave"
	}
	return "6 Pulse"
}
func (m *Model) soundRows(w, h int) []string {
	if m.UI.Visualizer == 2 {
		return m.equalizerRows(w, h)
	}
	return m.visualizerRows(w, h)
}
func (m *Model) equalizerRows(w, h int) []string {
	if !m.State.Source.Capabilities.Equalizer {
		return []string{muted.Render("No equalizer for this source")}
	}
	eq := m.State.Equalizer
	state := "EQ ON"
	if eq.Bypassed {
		state = "BYPASSED"
	}
	rows := []string{muted.Render(fmt.Sprintf("%s · %s Hz · %+.0f dB", state, music.EQLabels[m.UI.EQBand], eq.Gains[m.UI.EQBand]))}
	if h < 5 {
		return rows
	}
	count := min(10, max(1, w/4))
	start := max(0, min(m.UI.EQBand-count/2, 10-count))
	cell := max(4, w/count)
	chartH := max(2, h-4)
	for row := 0; row < chartH; row++ {
		var line strings.Builder
		for band := start; band < start+count; band++ {
			pos := int((12-eq.Gains[band])/24*float64(chartH-1) + .5)
			zero := (chartH - 1) / 2
			ch := "│"
			if row == zero {
				ch = "┼"
			}
			if row >= min(pos, zero) && row <= max(pos, zero) {
				ch = "┃"
			}
			if row == pos {
				ch = "●"
			}
			label := strings.Repeat(" ", (cell-1)/2) + ch + strings.Repeat(" ", cell-1-(cell-1)/2)
			style := muted
			if !eq.Bypassed {
				style = good
			}
			if band == m.UI.EQBand && m.UI.Focus == 5 {
				style = accent.Bold(true)
			}
			line.WriteString(style.Render(label))
		}
		rows = append(rows, line.String())
	}
	var labels strings.Builder
	for band := start; band < start+count; band++ {
		label := fit(music.EQLabels[band], cell)
		if band == m.UI.EQBand {
			label = accent.Render(label)
		} else {
			label = muted.Render(label)
		}
		labels.WriteString(label)
	}
	rows = append(rows, labels.String(), muted.Render("h/l band · j/k gain · e bypass · r flat"))
	return rows
}
func (m *Model) equalizerKey(action, key string) (tea.Cmd, bool) {
	switch action {
	case "open", "back":
		delta := 1
		if action == "back" {
			delta = -1
		}
		m.UI.EQBand = (m.UI.EQBand + delta + 10) % 10
		return m.dirty(), true
	case "up", "down":
		if !m.State.Source.Capabilities.Equalizer {
			return nil, true
		}
		delta := 1.
		if action == "down" {
			delta = -1
		}
		return m.command(daemon.Command{Op: "equalizer", EQBand: m.UI.EQBand, EQDelta: delta}), true
	case "eq_toggle":
		return m.command(daemon.Command{Op: "equalizer", EQAction: "toggle"}), true
	case "eq_reset":
		return m.command(daemon.Command{Op: "equalizer", EQPreset: "flat"}), true
	case "eq_preset":
		presets := []string{"flat", "bass", "vocal", "treble"}
		next := 0
		for i, name := range presets {
			eq, _ := music.EQPreset(name)
			if eq.Gains == m.State.Equalizer.Gains {
				next = (i + 1) % len(presets)
				break
			}
		}
		return m.command(daemon.Command{Op: "equalizer", EQPreset: presets[next]}), true
	}
	return nil, false
}
func (m *Model) zenBody(w, h int) string {
	eqW := m.UI.ZenEQWidth
	if eqW == 0 {
		eqW = w / 2
	}
	eqW = min(max(20, eqW), w-20)
	kind := m.UI.Visualizer
	if kind == 2 {
		kind = 0
	}
	rhythm := m.rhythmRows(kind, w-eqW-4, h-2)
	name := "Pulse"
	if kind == 1 {
		name = "Wave"
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		box("Rhythm · "+name, rhythm, w-eqW, h, false),
		box("6 Equalizer", m.equalizerRows(eqW-4, h-2), eqW, h, m.UI.Focus == 5))
}
func (m *Model) resize(action string) tea.Cmd {
	delta := 1
	if action == "resize_up" || action == "resize_left" {
		delta = -1
	}
	if action == "resize_left" || action == "resize_right" {
		if m.UI.Zen {
			m.UI.ZenEQWidth = min(max(20, m.zenEQWidth()+delta*2), m.width-20)
		} else {
			switch m.UI.Focus {
			case 0, 5:
				m.UI.NavWidth = m.navWidth() + delta*2
			case 2:
				m.UI.QueueWidth = m.queueWidth() + delta*2
			case 1:
				m.UI.NavWidth = m.navWidth() - delta*2
			}
		}
	} else {
		switch m.UI.Focus {
		case 0:
			m.UI.NavHeight = m.navHeight() + delta
		case 5:
			if m.UI.Zen {
				m.UI.PlayerHeight = m.playerHeight() - delta
			} else {
				m.UI.NavHeight = m.navHeight() - delta
			}
		case 3:
			m.UI.PlayerHeight = m.playerHeight() + delta
		case 4:
			m.UI.LogHeight = m.logHeight() + delta
		default:
			m.UI.PlayerHeight = m.playerHeight() - delta
		}
	}
	m.UI.NavWidth = min(max(0, m.UI.NavWidth), max(18, m.width-24))
	m.UI.QueueWidth = min(max(0, m.UI.QueueWidth), max(20, m.width-42))
	if m.UI.NavHeight != 0 {
		m.UI.NavHeight = min(max(3, m.UI.NavHeight), max(3, m.bodyHeight()-3))
	}
	if m.UI.PlayerHeight != 0 {
		m.UI.PlayerHeight = min(max(4, m.UI.PlayerHeight), max(4, m.height/2))
	}
	if m.UI.LogHeight != 0 {
		m.UI.LogHeight = min(max(3, m.UI.LogHeight), max(3, m.height/3))
	}
	return m.dirty()
}
func (m *Model) zenEQWidth() int {
	if m.UI.ZenEQWidth > 0 {
		return m.UI.ZenEQWidth
	}
	return m.width / 2
}
