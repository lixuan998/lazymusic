package tui

import (
	"context"
	"errors"
	"lazymusic/internal/catalog"
	"lazymusic/internal/music"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

type inspectAPI struct {
	*fakeAPI
	subject catalog.Subject
	details catalog.Details
}

func (a *inspectAPI) Inspect(_ context.Context, subject catalog.Subject) (catalog.Details, error) {
	a.subject = subject
	return a.details, nil
}

func TestWorkActionsUseActualFocusedWorkAndNeverClearQueue(t *testing.T) {
	for _, focus := range []int{1, 2, 3} {
		for _, action := range []string{"o", "c"} {
			m, f := testModel()
			a := &inspectAPI{fakeAPI: f, details: catalog.Details{Title: "Work"}}
			m.api = a
			m.UI.Focus = focus
			m.items = []catalog.Item{{ID: "content", Kind: "songs", Title: "Content"}}
			m.State.Queue = []music.Track{{ID: "queue", PlaybackID: "catalog-queue", Title: "Queue"}}
			m.State.Playback.Track = &music.Track{ID: "playing", Title: "Player"}
			cmd := m.normal(action)
			if cmd == nil || m.work == nil || !m.HasModal() {
				t.Fatalf("focus %d action %s did not open work dialog", focus, action)
			}
			m.Update(cmd())
			want := map[int]string{1: "content", 2: "queue", 3: "playing"}[focus]
			if a.subject.ID != want || a.subject.Credits != (action == "c") || f.lastCommand.Op != "" || m.UI.Focus != focus {
				t.Fatalf("focus %d action %s used wrong work: %+v", focus, action, a.subject)
			}
		}
	}
	m, f := testModel()
	m.UI.Focus = 2
	cmd := m.normal("C")
	cmd()
	if f.lastCommand.Op != "clear" {
		t.Fatal("uppercase C no longer clears queue")
	}
}

func TestWorkJumpUsesOpaqueBackendQueryAndReturnsToHistory(t *testing.T) {
	m, f := testModel()
	q := catalog.Query{Source: m.State.Source.ID, Page: "opaque-artist", Section: "albums", ID: "second-artist"}
	a := &inspectAPI{fakeAPI: f, details: catalog.Details{Links: []catalog.Item{
		{Title: "Album", Kind: "albums", Open: &q}, {Title: "Other artist", Kind: "artists", Open: &q},
	}}}
	m.api = a
	m.UI.Layout = "queue"
	m.UI.Focus = 2
	m.State.Queue = []music.Track{{ID: "selected", Title: "Selected"}}
	before := m.UI.Location
	m.Update(m.normal("o")())
	m.Update(key("j"))
	m.Update(key("l"))
	if m.work != nil || m.UI.Focus != 1 || m.UI.Layout != "content" || m.UI.Location.Page != "opaque-artist" || m.UI.Location.Title != "Other artist" || f.lastCommand.Op != "" {
		t.Fatal("work link changed playback, opened wrong target, or left content hidden")
	}
	m.normal("h")
	if m.UI.Location != before {
		t.Fatal("work jump lost original location")
	}
}

func TestClosedWorkDialogIgnoresLateResultsAndErrors(t *testing.T) {
	m, _ := testModel()
	m.items = []catalog.Item{{ID: "song", Title: "Song"}}
	m.inspectWork("related")
	generation := m.inspectGeneration
	m.Update(decodeKey(t, "\x1b[27u"))
	for _, err := range []error{nil, context.Canceled, errors.New("late failure")} {
		m.Update(detailsMsg{generation: generation, err: err})
		if m.work != nil || m.HasToast() || m.UI.Focus != 1 {
			t.Fatal("late metadata response reopened a dialog or created a spurious error")
		}
	}
}

func TestCreditsModalScrollsAndFitsSmallTerminal(t *testing.T) {
	m, _ := testModel()
	m.width, m.height = 60, 16
	m.work = &workDialog{mode: "credits", title: "中文作品"}
	for i := 0; i < 80; i++ {
		m.work.details.Credits = append(m.work.details.Credits, catalog.Credit{Group: "Production", Name: "A long contributor name with 中文 characters", Roles: []string{"Recording engineer", "Producer"}})
	}
	m.workKey("G")
	view := m.View().Content
	if len(strings.Split(view, "\n")) > m.height || m.work.scroll == 0 {
		t.Fatal("credits cannot scroll or exceed terminal height")
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > m.width {
			t.Fatal("credits exceed terminal width")
		}
	}
}
