package tui

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
)

const rhythmFrameInterval = time.Second / 30
const rhythmBands = 24

type rhythmFrame struct {
	generation uint64
	at         time.Time
}

// Animation is frontend-only. Playback position, polling and session revisions
// do not drive its clock, so seeking or a slow IPC response cannot jerk a frame.
type rhythmAnimation struct {
	initialized, running bool
	generation           uint64
	last                 time.Time
	phase, energy, fade  float64
	mode, target         int
	levels, peaks        [rhythmBands]float64
}

func (m *Model) rhythmVisible() bool {
	return !m.exiting && m.ctx.Err() == nil && m.width >= 60 && m.height >= 16 && !m.HasModal() &&
		(m.UI.Zen || m.UI.Layout == "full" && m.UI.Visualizer != 2)
}

func (m *Model) rhythmPlaying() bool {
	s := m.State.Playback
	return s.Playing && !s.Loading && s.Status != "error"
}

func (a *rhythmAnimation) moving(playing bool) bool {
	if playing || a.energy > .002 || a.mode != a.target || a.fade < 1 {
		return true
	}
	for i := range a.levels {
		if a.levels[i] > .002 || a.peaks[i] > .002 {
			return true
		}
	}
	return false
}

func (m *Model) syncRhythm(now time.Time) tea.Cmd {
	a := &m.rhythm
	target := 0
	if m.UI.Visualizer == 1 {
		target = 1
	}
	if !a.initialized {
		a.initialized, a.mode, a.target, a.fade = true, target, target, 1
	}
	a.target = target
	if !m.rhythmVisible() {
		a.last = time.Time{}
		if a.running {
			a.running = false
			a.generation++
		}
		if !m.rhythmPlaying() {
			a.energy = 0
			a.levels, a.peaks = [rhythmBands]float64{}, [rhythmBands]float64{}
		}
		return nil
	}
	if !a.moving(m.rhythmPlaying()) {
		if a.running {
			a.running = false
			a.generation++
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
	a.generation++
	generation := a.generation
	return tea.Tick(rhythmFrameInterval, func(at time.Time) tea.Msg {
		return rhythmFrame{generation: generation, at: at}
	})
}

func (a *rhythmAnimation) advance(now time.Time, playing bool) {
	dt := math.Max(0, math.Min(.1, now.Sub(a.last).Seconds()))
	a.last = now
	target, tau := 0., .21
	if playing {
		target, tau = 1, .12
	}
	a.energy += (target - a.energy) * (1 - math.Exp(-dt/tau))
	if !playing && a.energy < .002 {
		a.energy = 0
	}
	if a.energy > 0 {
		a.phase += dt * (.25 + .75*a.energy)
	}
	if a.mode != a.target {
		a.fade = math.Max(0, a.fade-dt*12)
		if a.fade == 0 {
			a.mode = a.target
		}
	} else {
		a.fade = math.Min(1, a.fade+dt*8)
	}
	for i := range a.levels {
		x := float64(i) / (rhythmBands - 1)
		envelope := .4 + .6*math.Pow(math.Sin(math.Pi*x), .6)
		beat := .43 + .23*math.Sin(a.phase*2.8+x*7) + .17*math.Sin(a.phase*4.3-x*11) + .10*math.Cos(a.phase*1.4+x*19)
		level := a.energy * envelope * math.Max(.08, math.Min(.96, beat))
		tau := .07
		if level < a.levels[i] {
			tau = .16
		}
		a.levels[i] += (level - a.levels[i]) * (1 - math.Exp(-dt/tau))
		decay := .23
		if !playing {
			decay = .9
		}
		a.peaks[i] = math.Max(a.levels[i], a.peaks[i]-dt*decay)
		if !playing && a.levels[i] < .002 {
			a.levels[i] = 0
		}
		if a.peaks[i] < .002 {
			a.peaks[i] = 0
		}
	}
}
