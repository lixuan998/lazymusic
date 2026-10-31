package tui

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
)

const recordFrameInterval = time.Second / 30
const recordRevolution = 12 * time.Second

type recordFrame struct {
	generation uint64
	at         time.Time
}
type recordAnimation struct {
	key        string
	angle      float64
	last       time.Time
	running    bool
	generation uint64
}

func (m *Model) recordCoverVisible() bool {
	return m.State.Playback.Track != nil && m.State.Playback.Status != "stopped"
}

func (m *Model) recordKey() string {
	if !m.recordCoverVisible() {
		return m.State.Source.ID + "|stopped"
	}
	return m.State.Source.ID + "|" + m.State.Playback.Track.ID
}

func (m *Model) syncRecord(now time.Time) tea.Cmd {
	a := &m.record
	key := m.recordKey()
	if a.key != key {
		a.key, a.angle, a.last = key, 0, time.Time{}
		a.generation++
		a.running = false
	}
	if !m.turntableVisible() || !m.recordCoverVisible() || !m.artwork.image.Valid() || !m.rhythmPlaying() {
		if a.running {
			a.generation++
			a.running = false
		}
		a.last = time.Time{}
		return nil
	}
	if a.running {
		return nil
	}
	a.running = true
	if a.last.IsZero() {
		a.last = now
	}
	generation := a.generation
	interval := recordFrameInterval
	if m.graphics.protocol == "sixel" {
		interval = time.Second / 10
	}
	if m.videoVisible() {
		interval = time.Second / 8
	}
	return tea.Tick(interval, func(at time.Time) tea.Msg { return recordFrame{generation, at} })
}

func (m *Model) advanceRecord(msg recordFrame) {
	a := &m.record
	if msg.generation != a.generation || !a.running {
		return
	}
	a.running = false
	if m.rhythmPlaying() && m.turntableVisible() && m.recordCoverVisible() {
		dt := math.Max(0, math.Min(.15, msg.at.Sub(a.last).Seconds()))
		a.angle = math.Mod(a.angle+dt*2*math.Pi/recordRevolution.Seconds(), 2*math.Pi)
	}
	a.last = msg.at
}
