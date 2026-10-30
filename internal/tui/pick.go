package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"lazymusic/internal/catalog"
	"lazymusic/internal/daemon"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
)

type pickedSelection struct {
	source, title, itemID, origin, queueKey string
	fromQueue, loading, placing             bool
	index                                   int
	tracks                                  []music.Track
}

type pickResolvedMsg struct {
	generation uint64
	tracks     []music.Track
	err        error
}

type queuePlacedMsg struct {
	generation uint64
	state      daemon.State
	index      int
	err        error
}

// CancelPick releases transient selection/loading state. Submitted queue writes
// have their own lifetime and are never canceled or retried by this method.
func (m *Model) CancelPick() {
	if m.pickCancel != nil {
		m.pickCancel()
		m.pickCancel = nil
	}
	m.pickGeneration++
	m.picked = nil
}

func (m *Model) cancelPicked() tea.Cmd {
	if m.picked != nil && !m.picked.placing {
		m.CancelPick()
	}
	return nil
}

func (m *Model) pickOrPlace(before bool) tea.Cmd {
	if m.picked != nil && m.picked.placing {
		return nil
	}
	if m.UI.Focus == 2 && m.picked != nil {
		return m.placePicked(before)
	}
	p := &pickedSelection{source: m.State.Source.ID}
	if m.UI.Focus == 2 {
		i := m.UI.QueueCursor
		if i < 0 || i >= len(m.State.Queue) {
			return nil
		}
		p.fromQueue, p.index, p.title = true, i, m.State.Queue[i].Title
		p.queueKey = music.QueueKey(m.State.Queue)
	} else if m.UI.Focus == 1 && !m.LoginSelected() {
		items := m.visible()
		if m.cursor < 0 || m.cursor >= len(items) {
			return nil
		}
		item := items[m.cursor]
		if item.Track == nil && item.Play == nil {
			if item.PlaybackNote != "" {
				m.Report("warn", "catalog", item.PlaybackNote)
			}
			return nil
		}
		if item.Source != "" && item.Source != p.source {
			m.Report("warn", "queue", "This selection belongs to another music source")
			return nil
		}
		p.title, p.itemID, p.origin = item.Title, item.CursorID(), m.query().Key()
		if item.Track != nil {
			p.tracks = session.CleanTracks([]music.Track{*item.Track})
		}
		m.CancelPick()
		m.picked = p
		if item.Track != nil {
			return nil
		}
		q := *item.Play
		if q.Source == "" {
			q.Source = p.source
		}
		p.loading = true
		generation := m.pickGeneration
		ctx, cancel := context.WithTimeout(m.ctx, 90*time.Second)
		m.pickCancel = cancel
		return func() tea.Msg {
			defer cancel()
			tracks, err := catalog.All(ctx, m.api, q)
			return pickResolvedMsg{generation, tracks, err}
		}
	} else {
		return nil
	}
	m.CancelPick()
	m.picked = p
	return nil
}

func (m *Model) acceptPicked(msg pickResolvedMsg) {
	if m.picked == nil || msg.generation != m.pickGeneration || !m.picked.loading {
		return
	}
	m.picked.loading = false
	m.pickCancel = nil
	if msg.err != nil || len(msg.tracks) == 0 {
		m.CancelPick()
		if msg.err != nil && !errors.Is(msg.err, context.Canceled) {
			m.Report("error", "queue", msg.err.Error())
		} else if msg.err == nil {
			m.Report("warn", "queue", "No playable songs in this collection")
		}
		return
	}
	m.picked.tracks = session.CleanTracks(msg.tracks)
}

func (m *Model) validatePickedQueue() {
	if m.picked != nil && m.picked.fromQueue && !m.picked.placing && m.picked.queueKey != music.QueueKey(m.State.Queue) {
		m.CancelPick()
		m.Report("warn", "queue", "Queue changed; pick the song again")
	}
}

func (m *Model) placePicked(before bool) tea.Cmd {
	p := m.picked
	if p == nil || p.loading || p.placing {
		return nil
	}
	if p.source != m.State.Source.ID {
		m.CancelPick()
		return nil
	}
	m.validatePickedQueue()
	if m.picked == nil {
		return nil
	}
	m.alignQueue()
	to := m.UI.QueueCursor
	command := daemon.Command{ExpectedQueue: music.QueueKey(m.State.Queue)}
	if p.fromQueue {
		command.Op, command.Index = "move", p.index
		if before && p.index < to {
			to--
		}
		if !before && p.index > to {
			to++
		}
		if p.index == m.UI.QueueCursor || p.index == to {
			m.CancelPick()
			return nil
		}
	} else {
		command.Op, command.Tracks = "insert", session.CleanTracks(p.tracks)
		if !before && len(m.State.Queue) > 0 {
			to++
		}
	}
	command.To = to
	p.placing = true
	generation, api, parent := m.pickGeneration, m.api, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 85*time.Second)
		defer cancel()
		state, err := api.Command(ctx, command)
		return queuePlacedMsg{generation, state, to, err}
	}
}

func (m *Model) acceptPlaced(msg queuePlacedMsg) tea.Cmd {
	if m.picked == nil || !m.picked.placing || msg.generation != m.pickGeneration {
		return nil
	}
	if msg.err != nil {
		// A transport/save failure can have an unknown outcome. End this
		// gesture so another key cannot accidentally submit it a second time.
		m.CancelPick()
		m.Report("error", "queue", msg.err.Error())
		return nil
	}
	current := music.QueueKey(m.State.Queue)
	if !msg.state.ObservedAt.Before(m.State.ObservedAt) {
		m.State = msg.state
		m.acceptLogs(msg.state.Logs)
		current = music.QueueKey(msg.state.Queue)
	}
	m.CancelPick()
	if current == music.QueueKey(msg.state.Queue) {
		m.UI.QueueCursor = msg.index
	}
	return m.dirty()
}

func (m *Model) pickedStatus() string {
	p := m.picked
	if p == nil {
		return ""
	}
	if p.placing {
		return "Placing · " + plain(p.title)
	}
	if p.loading {
		return "Picking collection · " + plain(p.title) + " · Esc cancel"
	}
	count := len(p.tracks)
	if p.fromQueue {
		count = 1
	}
	return fmt.Sprintf("Picked · %s · %d track(s) · Queue: p after / P before · Esc cancel", plain(p.title), count)
}
