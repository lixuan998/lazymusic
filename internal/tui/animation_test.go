package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func stepRhythm(m *Model, at time.Time) {
	m.Update(rhythmFrame{generation: m.rhythm.generation, at: at})
}

func TestRhythmAnimatesBetweenPollsWithoutSavingSession(t *testing.T) {
	m, _ := testModel()
	m.State.Playback.Playing = true
	now := time.Now()
	if m.syncRhythm(now) == nil {
		t.Fatal("playing rhythm did not start a frame timer")
	}
	revision := m.revision
	frames := map[string]bool{}
	for i := 1; i <= 30; i++ {
		stepRhythm(m, now.Add(time.Duration(i)*rhythmFrameInterval))
		frames[strings.Join(m.rhythmRows(0, 24, 18), "\n")] = true
	}
	if len(frames) < 20 || m.State.Playback.Position != 0 || m.revision != revision {
		t.Fatal("animation is tied to playback polling, or writes session revisions")
	}
	_, cmd := m.Update(rhythmFrame{generation: m.rhythm.generation, at: now.Add(31 * rhythmFrameInterval)})
	if cmd == nil {
		t.Fatal("animation loop stopped while playing")
	}
	if _, ok := cmd().(rhythmFrame); !ok {
		t.Fatal("a frame scheduled an IPC or persistence command")
	}
}

func TestRhythmEasesToRestAndStopsScheduling(t *testing.T) {
	m, _ := testModel()
	m.State.Playback.Playing = true
	now := time.Now()
	m.syncRhythm(now)
	for i := 1; i <= 30; i++ {
		stepRhythm(m, now.Add(time.Duration(i)*rhythmFrameInterval))
	}
	energy := m.rhythm.energy
	phase := m.rhythm.phase
	m.State.Playback.Position = 10 * time.Minute
	stepRhythm(m, now.Add(31*rhythmFrameInterval))
	if m.rhythm.phase-phase > .05 {
		t.Fatal("seeking jumped the animation clock")
	}
	m.State.Playback.Playing = false
	stepRhythm(m, now.Add(32*rhythmFrameInterval))
	if m.rhythm.energy <= 0 || m.rhythm.energy >= energy {
		t.Fatal("pause did not ease the amplitude down")
	}
	for i := 33; i <= 140 && m.rhythm.running; i++ {
		stepRhythm(m, now.Add(time.Duration(i)*rhythmFrameInterval))
	}
	if m.rhythm.running || m.rhythm.energy != 0 {
		t.Fatal("resting visualization still schedules frames")
	}
}

func TestHiddenRhythmCancelsFramesAndResumesWithoutJumping(t *testing.T) {
	for _, hide := range []string{"content", "queue", "equalizer", "modal", "small"} {
		m, _ := testModel()
		m.State.Playback.Playing = true
		now := time.Now()
		m.syncRhythm(now)
		stale := m.rhythm.generation
		switch hide {
		case "content", "queue":
			m.UI.Layout = hide
		case "equalizer":
			m.UI.Visualizer = 2
		case "modal":
			m.help = true
		case "small":
			m.width = 40
		}
		if cmd := m.syncRhythm(now); cmd != nil || m.rhythm.running {
			t.Fatalf("%s still animates a hidden panel", hide)
		}
		m.Update(rhythmFrame{generation: stale, at: now.Add(time.Minute)})
		if m.rhythm.phase != 0 {
			t.Fatal("stale frame changed hidden animation")
		}
		m.UI.Layout, m.UI.Visualizer, m.help, m.width = "full", 0, false, 100
		m.syncRhythm(now.Add(time.Hour))
		stepRhythm(m, now.Add(time.Hour+rhythmFrameInterval))
		if m.rhythm.phase > .05 {
			t.Fatal("restoring a hidden panel jumped forward")
		}
	}
}

func TestImmediateStopInvalidatesPendingFrame(t *testing.T) {
	m, _ := testModel()
	now := time.Now()
	m.State.Playback.Playing = true
	m.syncRhythm(now)
	stale := m.rhythm.generation
	m.State.Playback.Playing = false
	if m.syncRhythm(now) != nil || m.rhythm.running {
		t.Fatal("stop before first frame did not cancel the timer")
	}
	m.State.Playback.Playing = true
	m.syncRhythm(now)
	m.Update(rhythmFrame{generation: stale, at: now.Add(rhythmFrameInterval)})
	if m.rhythm.energy != 0 {
		t.Fatal("stale frame from an earlier play command jumped the amplitude")
	}
}

func TestWaveUsesSubcellMotionAndModeChangeSettles(t *testing.T) {
	m, _ := testModel()
	m.State.Playback.Playing = true
	now := time.Now()
	m.syncRhythm(now)
	m.UI.Visualizer = 1
	m.syncRhythm(now)
	for i := 1; i <= 30; i++ {
		stepRhythm(m, now.Add(time.Duration(i)*rhythmFrameInterval))
	}
	if m.rhythm.mode != 1 || m.rhythm.fade != 1 {
		t.Fatal("mode transition did not complete")
	}
	frames := map[string]bool{}
	for i := 31; i <= 50; i++ {
		stepRhythm(m, now.Add(time.Duration(i)*rhythmFrameInterval))
		frame := ansi.Strip(strings.Join(m.rhythmRows(1, 22, 14), "\n"))
		frames[frame] = true
		if !strings.ContainsFunc(frame, func(r rune) bool { return r > 0x2800 && r <= 0x28ff }) {
			t.Fatal("wave is not rendered at braille resolution")
		}
	}
	if len(frames) < 16 {
		t.Fatal("wave still moves in coarse polling steps")
	}
}

func TestRhythmFitsSmallSidebarAndLargeZenSizes(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {2, 2}, {5, 3}, {14, 5}, {18, 16}, {24, 30}, {80, 26}, {120, 50}} {
		for _, mode := range []int{0, 1} {
			m, _ := testModel()
			m.rhythm = rhythmAnimation{initialized: true, energy: 1, fade: 1, mode: mode, phase: 2}
			for i := range m.rhythm.levels {
				m.rhythm.levels[i], m.rhythm.peaks[i] = .5, .9
			}
			rows := m.rhythmRows(mode, size[0], size[1])
			if len(rows) != size[1] {
				t.Fatal("wrong rhythm height")
			}
			for _, row := range rows {
				if ansi.StringWidth(row) != size[0] {
					t.Fatalf("mode=%d size=%v: rhythm exceeds panel width", mode, size)
				}
			}
		}
	}
}

func BenchmarkRhythmWave(b *testing.B) {
	m, _ := testModel()
	m.rhythm = rhythmAnimation{initialized: true, energy: 1, fade: 1, mode: 1, phase: 2}
	for b.Loop() {
		m.rhythmRows(1, 80, 26)
	}
}
