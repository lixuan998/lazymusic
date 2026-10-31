package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"lazymusic/internal/termimage"
)

const graphicsProbeAttempts = 3
const graphicsProbeTimeout = 1500 * time.Millisecond

type graphicsQueryWrite struct{ attempt uint64 }
type graphicsQuerySent struct{ attempt uint64 }
type graphicsTimeout struct{ attempt uint64 }

// The capability query lives independently of artwork uploads. Cached artwork
// can arrive before a terminal reply, and must never delete its routing ID.
func (g *graphicsState) probeID() int {
	if g.id == termimage.MaxID {
		return termimage.MinID
	}
	return g.id + 1
}

func (m *Model) syncGraphicsProbe() tea.Cmd {
	g := &m.graphics
	if g.disabled || g.ready() || g.checked && (g.protocol != "sixel" || g.queryAttempts >= graphicsProbeAttempts) {
		return nil
	}
	if !g.queryPending {
		g.queried, g.queryPending, g.querySent, g.queryTimed = g.requested != "sixel", true, false, false
		g.queryAttempts++
		g.queryGeneration++
		attempt := g.queryGeneration
		return func() tea.Msg { return graphicsQueryWrite{attempt} }
	}
	// Start the timeout after Raw output has been processed, rather than while
	// the request is still queued behind other frontend/Neovim work.
	if g.querySent && !g.queryTimed {
		g.queryTimed = true
		attempt := g.queryGeneration
		return tea.Tick(graphicsProbeTimeout, func(time.Time) tea.Msg { return graphicsTimeout{attempt} })
	}
	return nil
}

// InheritGraphicsSupport carries only verified support across source changes
// within the same terminal program. Artwork IDs and upload state stay separate.
func (m *Model) InheritGraphicsSupport(previous *Model) {
	if previous != nil && previous.graphics.ready() && !m.graphics.disabled {
		m.graphics.replied, m.graphics.known = true, true
		m.graphics.protocol = previous.graphics.protocol
		m.graphics.sixelSupported = previous.graphics.sixelSupported
		m.graphics.cellWidth, m.graphics.cellHeight = previous.graphics.cellWidth, previous.graphics.cellHeight
	}
}
