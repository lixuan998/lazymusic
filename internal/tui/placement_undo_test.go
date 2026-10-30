package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"lazymusic/internal/daemon"
	"lazymusic/internal/music"
)

type undoAPI struct {
	fakeAPI
	state daemon.State
	calls int
	fail  bool
}

func (a *undoAPI) Command(_ context.Context, c daemon.Command) (daemon.State, error) {
	a.calls++
	a.lastCommand = c
	if a.fail {
		return daemon.State{}, errors.New("unknown transport outcome")
	}
	p := *a.state.Placement
	p.Undone = !p.Undone
	p.Revision++
	a.state.Placement = &p
	a.state.ObservedAt = time.Now()
	return a.state, nil
}
func undoModel() (*Model, *undoAPI) {
	m, _ := testModel()
	m.State.Placement = &music.PlacementStatus{ID: "placement", Available: true}
	m.UI.Focus = 2
	a := &undoAPI{state: m.State}
	m.api = a
	return m, a
}

func TestUInContentAndQueueTogglesUndoRedoAndWaitsForReply(t *testing.T) {
	for _, focus := range []int{1, 2} {
		m, a := undoModel()
		m.UI.Focus = focus
		for step := 0; step < 4; step++ {
			cmd := m.normal("u")
			if cmd == nil || !m.placementBusy {
				t.Fatal("u did not submit an undo")
			}
			if m.normal("u") != nil || m.normal("p") != nil {
				t.Fatal("submitted duplicate undo or placement")
			}
			m.Update(cmd())
			if m.placementBusy || m.State.Placement.Undone != (step%2 == 0) || a.calls != step+1 || m.UI.Focus != focus {
				t.Fatal("u did not alternate while preserving focus")
			}
			if a.lastCommand.PlacementID != "placement" || a.lastCommand.PlacementRevision != uint64(step) {
				t.Fatal("undo omitted its history revision guard")
			}
			want := "Undo last"
			if step%2 == 0 {
				want = "Redo last"
			}
			if !strings.Contains(strings.Join(m.helpLines(), "\n"), want) {
				t.Fatal("context help did not change undo/redo description")
			}
		}
	}
}

func TestUndoFailureDoesNotGuessHistoryOrReplayRequest(t *testing.T) {
	m, a := undoModel()
	a.fail = true
	cmd := m.normal("u")
	m.Update(cmd())
	if m.placementBusy || m.State.Placement.Undone || a.calls != 1 || m.toast == nil {
		t.Fatal("failed request changed history or stayed busy")
	}
	m.mode = "query"
	m.Update(key("u"))
	if m.draft != "u" || a.calls != 1 {
		t.Fatal("typing u triggered queue undo")
	}
}

func TestUndoDoesNotActOnOtherPanelsOrInvalidHistory(t *testing.T) {
	for _, focus := range []int{0, 3, 4, 5} {
		m, a := undoModel()
		m.UI.Focus = focus
		if m.normal("u") != nil || a.calls != 0 {
			t.Fatal("u escaped its panel scope")
		}
	}
	m, a := undoModel()
	m.State.Placement.Available = false
	if m.normal("u") != nil || a.calls != 0 || m.toast == nil {
		t.Fatal("stale placement was submitted")
	}
}
