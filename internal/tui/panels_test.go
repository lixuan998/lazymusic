package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"lazymusic/internal/daemon"
	"lazymusic/internal/eventlog"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
	"strings"
	"testing"
	"time"
)

func TestFooterTracksEveryPageAndFocusedPanel(t *testing.T) {
	for _, page := range []string{"home", "library", "playlists", "search", "login"} {
		m, _ := testModel()
		m.UI.NavPage = page
		m.UI.Focus = 1
		v := ansi.Strip(m.View().Content)
		if !strings.Contains(v, " "+strings.ToUpper(page)+"  ") || strings.Contains(v, " NORMAL ") {
			t.Fatalf("wrong footer for %s", page)
		}
	}
	for focus, label := range map[int]string{2: "QUEUE", 3: "PLAYER", 4: "LOG"} {
		m, _ := testModel()
		m.UI.Focus = focus
		m.UI.ShowLogs = focus == 4
		if !strings.Contains(ansi.Strip(m.View().Content), " "+label+"  ") {
			t.Fatal(label)
		}
	}
}
func TestCommaPeriodOnlyActOnFocusedPanel(t *testing.T) {
	m, api := testModel()
	m.UI.Location = session.Location{Page: "library", Section: "songs"}
	m.UI.Focus = 3
	cmd := m.normal(".")
	if cmd == nil {
		t.Fatal("missing playback mode command")
	}
	cmd()
	if api.lastCommand.Op != "playback-mode" || api.lastCommand.ModeStep != 1 || m.UI.Location.Section != "songs" {
		t.Fatal("player key changed content")
	}
	cmd = m.normal(",")
	cmd()
	if api.lastCommand.ModeStep != -1 {
		t.Fatal("previous mode failed")
	}
	m.UI.Focus = 0
	m.normal(".")
	if m.UI.Visualizer != 1 || m.UI.Location.Section != "songs" {
		t.Fatal("navigation animation changed content")
	}
	m.UI.Focus = 1
	m.normal(".")
	if m.UI.Location.Section != "albums" {
		t.Fatal("content category failed")
	}
}
func TestLayoutsAndLogsFitTerminal(t *testing.T) {
	for _, layout := range []string{"full", "content", "queue"} {
		for _, logs := range []bool{false, true} {
			for _, size := range [][2]int{{60, 16}, {100, 28}, {180, 50}} {
				m, _ := testModel()
				m.UI.Layout = layout
				m.UI.ShowLogs = logs
				m.SetSize(size[0], size[1])
				lines := strings.Split(m.View().Content, "\n")
				if len(lines) != size[1] {
					t.Fatalf("%s logs=%v height=%d want%d", layout, logs, len(lines), size[1])
				}
				for _, line := range lines {
					if ansi.StringWidth(line) > size[0] {
						t.Fatal("view exceeds terminal width")
					}
				}
			}
		}
	}
	m, _ := testModel()
	m.normal("+")
	if m.UI.Layout != "content" || m.UI.Focus != 1 {
		t.Fatal("content layout")
	}
	m.normal("+")
	if m.UI.Layout != "queue" || m.UI.Focus != 2 {
		t.Fatal("queue layout")
	}
	m.normal("_")
	if m.UI.Layout != "content" {
		t.Fatal("previous layout")
	}
}
func TestRuntimeErrorToastDismissesButRemainsInLog(t *testing.T) {
	m, _ := testModel()
	now := time.Now()
	entries := []eventlog.Entry{{ID: 1, Time: now, Level: "error", Component: "apple", Message: "simultaneous playback denied"}}
	m.Update(stateMsg{s: daemon.State{ObservedAt: now, Logs: entries}})
	if !m.HasToast() {
		t.Fatal("runtime error did not notify")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.HasToast() || len(m.allLogs()) != 1 {
		t.Fatal("dismiss removed log or kept toast")
	}
	m.Update(stateMsg{s: daemon.State{ObservedAt: now.Add(time.Second), Logs: entries}})
	if m.HasToast() {
		t.Fatal("old error reappeared")
	}
	m.normal("@")
	if !m.UI.ShowLogs || m.UI.Focus != 4 {
		t.Fatal("log did not open")
	}
	m.normal("@")
	if m.UI.ShowLogs {
		t.Fatal("log did not close")
	}
}
func TestQualitySelectionUsesAdvertisedIDs(t *testing.T) {
	m, api := testModel()
	m.State.Source.Qualities = []music.Quality{{ID: "standard", Name: "64"}, {ID: "high", Name: "256"}}
	m.State.PreferredQuality = "high"
	m.normal("b")
	if !m.qualityDialog || m.qualityCursor != 1 {
		t.Fatal("current preferred quality not selected")
	}
	m.Update(key("k"))
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cmd()
	if api.lastCommand.Op != "quality" || api.lastCommand.Quality != "standard" {
		t.Fatal("selection not passed to backend")
	}
	m.State.Source.Qualities = nil
	m.normal("b")
	if m.qualityDialog {
		t.Fatal("offered fake local transcoding")
	}
}
func TestEmptySearchInvalidatesInflightResponse(t *testing.T) {
	m, _ := testModel()
	m.UI.Location = session.Location{Page: "search", Section: "songs", Query: "old"}
	m.fetch(true, false)
	old := m.generation
	m.UI.Location.Query = ""
	m.fetch(true, false)
	if m.generation <= old || m.loading || m.restoring {
		t.Fatal("empty query left old request alive")
	}
}
func TestHistoricalLogsDoNotReopenDismissedToast(t *testing.T) {
	f := &fakeAPI{}
	m := New(context.Background(), f, daemon.State{UI: session.DefaultUI(), Logs: []eventlog.Entry{{ID: 9, Level: "warn", Message: "old"}}}, nil)
	m.acceptLogs(m.State.Logs)
	if m.toast != nil {
		t.Fatal("historical warning reopened")
	}
}

func TestToastExpiresWithoutStealingFocusOrClearingNewerNotice(t *testing.T) {
	m, _ := testModel()
	m.UI.Focus = 0
	m.Report("error", "test", "first")
	first := m.toastVersion
	m.Report("warn", "test", "second")
	latest := m.toastVersion
	m.Update(toastExpired(first))
	if !m.HasToast() {
		t.Fatal("old timer removed new notification")
	}
	m.Update(toastExpired(latest))
	if m.HasToast() || m.UI.Focus != 0 || len(m.allLogs()) != 2 {
		t.Fatal("auto dismiss lost logs or changed focus")
	}
}
func TestCanceledPageIsNotAnError(t *testing.T) {
	m, _ := testModel()
	m.generation = 2
	m.Update(pageMsg{generation: 2, key: m.query().Key(), err: fmt.Errorf("request: %w", context.Canceled)})
	if m.HasToast() || len(m.localLogs) > 0 || m.problem != "" {
		t.Fatal("normal page cancellation produced an error")
	}
}
func TestSplitSoundZenResizeAndReset(t *testing.T) {
	m, api := testModel()
	m.State.Source.Capabilities.Equalizer = true
	m.height = 40
	m.width = 140
	if !strings.Contains(ansi.Strip(m.View().Content), "6 Pulse") {
		t.Fatal("missing lower split")
	}
	m.normal("6")
	m.normal(".")
	m.normal(".")
	if m.UI.Visualizer != 2 || m.UI.Focus != 5 {
		t.Fatal("equalizer view not selected")
	}
	m.normal("l")
	cmd := m.normal("k")
	cmd()
	if m.UI.EQBand != 1 || api.lastCommand.Op != "equalizer" || api.lastCommand.EQBand != 1 || api.lastCommand.EQDelta != 1 {
		t.Fatal("equalizer controls did not dispatch")
	}
	before := m.navWidth()
	m.normal("ctrl+right")
	if m.navWidth() <= before {
		t.Fatal("resize did not change width")
	}
	m.normal("ctrl+-")
	if !m.UI.Zen {
		t.Fatal("zen not opened")
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Rhythm") || !strings.Contains(view, "Equalizer") || strings.Contains(view, "Navigation") {
		t.Fatal("zen does not show the requested panels")
	}
	m.normal("ctrl+=")
	if m.UI.Zen || m.UI.Layout != "full" || m.UI.NavWidth != 0 || m.UI.NavHeight != 0 {
		t.Fatal("default layout not restored")
	}
}
func TestResizedAndZenLayoutsAlwaysFit(t *testing.T) {
	for _, size := range [][2]int{{60, 16}, {100, 28}, {180, 50}} {
		for _, zen := range []bool{false, true} {
			for _, logs := range []bool{false, true} {
				m, _ := testModel()
				m.SetSize(size[0], size[1])
				m.UI.Zen = zen
				m.UI.ShowLogs = logs
				m.State.Source.Capabilities.Equalizer = true
				m.UI.Visualizer = 2
				for _, focus := range []int{0, 1, 2, 3, 4, 5} {
					m.UI.Focus = focus
					for range 20 {
						m.normal("ctrl+right")
						m.normal("ctrl+down")
					}
				}
				lines := strings.Split(m.View().Content, "\n")
				if len(lines) != size[1] {
					t.Fatalf("zen=%v logs=%v size=%v got height%d", zen, logs, size, len(lines))
				}
				for _, line := range lines {
					if ansi.StringWidth(line) > size[0] {
						t.Fatal("resized view exceeds terminal")
					}
				}
			}
		}
	}
}

func TestEnhancedKeyboardZenAndResetBindings(t *testing.T) {
	m, _ := testModel()
	m.Update(tea.KeyPressMsg{Code: '+', Mod: tea.ModCtrl | tea.ModShift})
	if !m.UI.Zen {
		t.Fatal("Ctrl+Shift+plus did not enter Zen")
	}
	m.Update(tea.KeyPressMsg{Code: '=', Mod: tea.ModCtrl})
	if m.UI.Zen || m.UI.Layout != "full" {
		t.Fatal("Ctrl+= did not reset layout")
	}
}
