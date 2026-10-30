package tui

import (
	"context"
	"encoding/json"
	"time"

	tea "charm.land/bubbletea/v2"
	"lazymusic/internal/artwork"
	"lazymusic/internal/catalog"
)

const turntableView = 3
const soundViews = 4

type artworkState struct {
	key        string
	generation uint64
	cancel     context.CancelFunc
	loading    bool
	loaded     bool
	image      artwork.Image
}
type artworkMsg struct {
	key        string
	generation uint64
	image      artwork.Image
	err        error
}

func (m *Model) turntableVisible() bool {
	available := m.bodyHeight() - m.navHeight() - 2
	if m.bodyHeight() < 6 && m.UI.Focus == 5 {
		available = m.bodyHeight() - 2
	}
	return !m.exiting && m.ctx.Err() == nil && m.width >= 60 && m.height >= 16 && !m.HasModal() &&
		!m.UI.Zen && m.UI.Layout == "full" && m.UI.Visualizer == turntableView && available >= 7
}

func (m *Model) CancelArtwork() {
	a := &m.artwork
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.generation++
	a.loading = false
}

func (m *Model) syncArtwork() tea.Cmd {
	if !m.turntableVisible() || m.State.Playback.Track == nil {
		if m.artwork.loading {
			m.CancelArtwork()
		}
		return nil
	}
	t := m.State.Playback.Track
	source := t.Source
	if source == "" {
		source = m.State.Source.ID
	}
	if source != m.State.Source.ID || t.ID == "" {
		return nil
	}
	subject := catalog.Subject{Source: source, ID: t.ID, Kind: t.Kind, PlaybackID: t.PlaybackID}
	b, _ := json.Marshal(subject)
	key := string(b)
	a := &m.artwork
	if a.key != key {
		m.CancelArtwork()
		a.key, a.image, a.loaded = key, artwork.Image{}, false
	}
	if a.loaded || a.loading {
		return nil
	}
	provider, ok := m.api.(catalog.ArtworkProvider)
	if !ok {
		a.loaded = true
		return nil
	}
	ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
	a.cancel, a.loading = cancel, true
	generation := a.generation
	return func() tea.Msg {
		defer cancel()
		im, err := provider.Artwork(ctx, subject)
		return artworkMsg{key, generation, im, err}
	}
}

func (m *Model) acceptArtwork(msg artworkMsg) {
	a := &m.artwork
	if msg.generation != a.generation || msg.key != a.key || !a.loading {
		return
	}
	a.cancel, a.loading, a.loaded = nil, false, true
	if msg.err == nil && msg.image.Valid() {
		a.image = msg.image
	} else if msg.err != nil && m.ctx.Err() == nil {
		// Optional artwork must not interrupt playback or create repeated toasts.
		m.Report("info", "artwork", "Album artwork is unavailable")
	}
}
