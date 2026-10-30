package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"lazymusic/internal/daemon"
	"lazymusic/internal/music"
)

type placementUndoneMsg struct {
	generation uint64
	source     string
	state      daemon.State
	err        error
}

func (m *Model) undoPlacement() tea.Cmd {
	if m.placementBusy || (m.picked != nil && m.picked.placing) {
		return nil
	}
	p := m.State.Placement
	if p == nil {
		m.Report("warn", "queue", "No pick & place to undo")
		return nil
	}
	if !p.Available {
		m.Report("warn", "queue", "Queue changed; the last placement can no longer be undone")
		return nil
	}
	m.CancelPick()
	m.placementBusy = true
	generation, source, api, parent := m.pickGeneration, m.State.Source.ID, m.api, m.ctx
	command := daemon.Command{Op: "undo-placement", PlacementID: p.ID, PlacementRevision: p.Revision}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 85*time.Second)
		defer cancel()
		state, err := api.Command(ctx, command)
		return placementUndoneMsg{generation, source, state, err}
	}
}

func (m *Model) acceptUndonePlacement(msg placementUndoneMsg) tea.Cmd {
	if !m.placementBusy || msg.generation != m.pickGeneration || msg.source != m.State.Source.ID {
		return nil
	}
	m.placementBusy = false
	if msg.err != nil {
		m.Report("error", "queue", msg.err.Error())
		return nil
	}
	if !msg.state.ObservedAt.Before(m.State.ObservedAt) {
		m.State = msg.state
		m.acceptLogs(msg.state.Logs)
	}
	if p := msg.state.Placement; p != nil && music.QueueKey(m.State.Queue) == music.QueueKey(msg.state.Queue) {
		m.UI.QueueCursor = max(0, min(p.Cursor, len(m.State.Queue)-1))
	}
	return m.dirty()
}
