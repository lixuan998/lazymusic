package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"lazymusic/internal/catalog"
	"lazymusic/internal/daemon"
	"lazymusic/internal/engine"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
)

type pickAPI struct {
	*fakeAPI
	e     *engine.Queue
	calls int
	err   error
	pages map[string]catalog.Page
}

func (a *pickAPI) snapshot() daemon.State {
	s, q, i := a.e.State()
	return daemon.State{Source: music.Source{ID: "demo", Capabilities: music.Capabilities{Home: true, Library: true, Playlists: true, CatalogSearch: true}}, Playback: s, Queue: q, Index: i, ObservedAt: time.Now()}
}

func (a *pickAPI) Command(_ context.Context, c daemon.Command) (daemon.State, error) {
	a.calls++
	a.lastCommand = c
	if a.err != nil {
		return daemon.State{}, a.err
	}
	_, q, _ := a.e.State()
	if c.ExpectedQueue != "" && c.ExpectedQueue != music.QueueKey(q) {
		return daemon.State{}, errors.New("queue changed")
	}
	switch c.Op {
	case "move":
		if err := a.e.Move(c.Index, c.To); err != nil {
			return daemon.State{}, err
		}
	case "insert":
		if err := a.e.Insert(c.Tracks, c.To); err != nil {
			return daemon.State{}, err
		}
	case "select":
		if err := a.e.Replace(q, c.Index, true); err != nil {
			return daemon.State{}, err
		}
	default:
		return daemon.State{}, fmt.Errorf("unexpected command %s", c.Op)
	}
	return a.snapshot(), nil
}

func (a *pickAPI) Browse(_ context.Context, q catalog.Query) (catalog.Page, error) {
	return a.pages[q.Page+"/"+q.ID+"/"+q.Next], nil
}

func pickModel() (*Model, *pickAPI) {
	m, f := testModel()
	tracks := []music.Track{}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		tracks = append(tracks, music.Track{Source: "demo", ID: id, Title: id, Duration: time.Minute})
	}
	a := &pickAPI{fakeAPI: f, e: engine.NewQueue(session.Playback{Queue: tracks, Index: 2, Position: 17 * time.Second}), pages: map[string]catalog.Page{}}
	m.api = a
	m.State = a.snapshot()
	m.UI.Focus = 2
	return m, a
}

func queueOrder(q []music.Track) string {
	var out string
	for _, t := range q {
		out += t.ID
	}
	return out
}

func TestPickAndPlaceMovesOneQueueOccurrenceBeforeOrAfter(t *testing.T) {
	for _, tc := range []struct {
		from, target int
		key, order   string
		index        int
	}{
		{1, 3, "p", "acdbe", 3}, {1, 3, "P", "acbde", 2},
		{3, 1, "p", "abdce", 2}, {3, 1, "P", "adbce", 1},
		{0, 4, "p", "bcdea", 4}, {4, 0, "P", "eabcd", 0},
		{2, 2, "p", "abcde", 2}, {2, 2, "P", "abcde", 2},
		{1, 0, "p", "abcde", 1}, {1, 2, "P", "abcde", 1},
	} {
		for _, pickKey := range []string{"p", "P"} {
			t.Run(fmt.Sprintf("%d/%d/%s/%s", tc.from, tc.target, tc.key, pickKey), func(t *testing.T) {
				m, a := pickModel()
				m.UI.QueueCursor = tc.from
				m.normal(pickKey)
				if m.picked == nil || !m.picked.fromQueue || a.calls != 0 {
					t.Fatal("pickup modified playback or did not retain the occurrence")
				}
				if !strings.Contains(ansi.Strip(strings.Join(m.queueRows(10), "\n")), "◆") {
					t.Fatal("picked row has no visible marker")
				}
				m.UI.QueueCursor = tc.target
				cmd := m.normal(tc.key)
				if cmd != nil {
					if duplicate := m.normal(tc.key); duplicate != nil {
						t.Fatal("duplicate placement was submitted")
					}
					m.Update(cmd())
				}
				if m.picked != nil || queueOrder(m.State.Queue) != tc.order {
					t.Fatalf("wrong placement: %s", queueOrder(m.State.Queue))
				}
				if a.calls > 0 && (m.UI.QueueCursor != tc.index || a.lastCommand.Op != "move" || a.lastCommand.ExpectedQueue == "") {
					t.Fatal("move selection/guard was not retained")
				}
				if m.State.Playback.Track.ID != "c" || m.State.Playback.Position != 17*time.Second {
					t.Fatal("move changed the current song or position")
				}
			})
		}
	}
}

func TestContentPickCopiesSongAndPlacesAtQueueCursor(t *testing.T) {
	for _, before := range []bool{false, true} {
		m, a := pickModel()
		m.UI.Focus = 1
		m.items = []catalog.Item{{Source: "demo", ID: "x", Title: "Picked song", Track: &music.Track{Source: "demo", ID: "x", Title: "Picked song"}}}
		m.normal("p")
		m.items[0].Track.ID = "later-selection"
		m.UI.Focus, m.UI.QueueCursor = 2, 1
		key, want := "p", "abxcde"
		if before {
			key, want = "P", "axbcde"
		}
		cmd := m.normal(key)
		if cmd == nil {
			t.Fatal("missing placement")
		}
		m.Update(cmd())
		if queueOrder(m.State.Queue) != want || a.lastCommand.Op != "insert" || m.State.Playback.Track.ID != "c" {
			t.Fatal("content pickup did not insert its captured song at the target")
		}
	}
}

func TestContentArtistPickExpandsNestedAlbumsAndAllPages(t *testing.T) {
	m, a := pickModel()
	m.UI.Focus = 1
	q := catalog.Query{Source: "demo", Page: "opaque-artist", ID: "artist"}
	m.items = []catalog.Item{{Source: "demo", ID: "artist", Title: "Artist", Play: &q}}
	a.pages["opaque-artist/artist/"] = catalog.Page{Items: []catalog.Item{{Play: &catalog.Query{Page: "opaque-album", ID: "album"}}}}
	a.pages["opaque-album/album/"] = catalog.Page{Items: []catalog.Item{{Track: &music.Track{Source: "demo", ID: "x"}}}, Next: "second"}
	a.pages["opaque-album/album/second"] = catalog.Page{Items: []catalog.Item{{Track: &music.Track{Source: "demo", ID: "y"}}}}
	resolve := m.normal("P")
	if m.picked == nil || !m.picked.loading || a.calls != 0 {
		t.Fatal("collection pickup did not start a separate read")
	}
	m.UI.Focus, m.UI.QueueCursor = 2, 1
	if m.normal("p") != nil {
		t.Fatal("unresolved collection was partially inserted")
	}
	m.Update(resolve())
	if m.picked == nil || m.picked.loading || len(m.picked.tracks) != 2 {
		t.Fatal("collection lost tracks or pagination")
	}
	cmd := m.normal("p")
	m.Update(cmd())
	if queueOrder(m.State.Queue) != "abxycde" || a.calls != 1 || len(a.lastCommand.Tracks) != 2 {
		t.Fatal("collection was not inserted as one ordered group")
	}
}

func TestPickCancelFailureAndLateResults(t *testing.T) {
	m, a := pickModel()
	m.normal("p")
	m.normal("esc")
	if m.picked != nil || a.calls != 0 {
		t.Fatal("cancel changed queue")
	}
	m.normal("p")
	m.UI.QueueCursor = 3
	a.err = errors.New("fixture placement failure")
	cmd := m.normal("p")
	m.Update(cmd())
	if m.picked != nil || queueOrder(m.State.Queue) != "abcde" || m.UI.Focus != 2 {
		t.Fatal("failure retained a repeatable write or changed queue/focus")
	}
	m.normal("p")
	m.Update(key("esc"))
	if m.picked != nil {
		t.Fatal("notification blocked pick cancellation")
	}
	m.Update(pickResolvedMsg{generation: m.pickGeneration - 1, tracks: []music.Track{{ID: "late"}}})
	if m.picked != nil {
		t.Fatal("late collection result restored canceled pickup")
	}
	m.normal("p")
	a.e.Append([]music.Track{{ID: "added"}}, false)
	m.Update(stateMsg{s: a.snapshot()})
	if m.picked != nil {
		t.Fatal("changed queue left an ambiguous picked occurrence")
	}
}

func TestQueueEnterStillPlaysAndEmptyQueueAcceptsContent(t *testing.T) {
	m, a := pickModel()
	m.UI.QueueCursor = 3
	cmd := m.normal("enter")
	m.Update(cmd())
	if a.lastCommand.Op != "select" || m.State.Playback.Track.ID != "d" || m.picked != nil {
		t.Fatal("Enter no longer plays the selected queue song")
	}
	a.e.Clear()
	m.State = a.snapshot()
	m.UI.Focus = 1
	m.items = []catalog.Item{{ID: "x", Track: &music.Track{ID: "x"}}}
	m.normal("P")
	m.UI.Focus = 2
	cmd = m.normal("P")
	m.Update(cmd())
	if len(m.State.Queue) != 1 || m.State.Queue[0].ID != "x" || m.State.Playback.Playing {
		t.Fatal("empty queue placement started playback or lost picked song")
	}
}

func TestCollectionPlaybackUsesControlEnterAndKeepsPickKeysLocal(t *testing.T) {
	m, a := pickModel()
	m.UI.Focus = 1
	m.items = []catalog.Item{{ID: "album", Title: "Album", Play: &catalog.Query{Source: "demo", Page: "collection"}}}
	cmd := m.normal(KeyString(decodeKey(t, "\x1b[13;5u")))
	if cmd == nil {
		t.Fatal("Ctrl+Enter does not play the selected collection")
	}
	// Inspect dispatch without executing a playback request in this fake engine.
	if m.picked != nil || a.calls != 0 {
		t.Fatal("collection playback shortcut picked or submitted early")
	}
	for _, focus := range []int{0, 3, 4} {
		m.UI.Focus = focus
		if m.normal("p") != nil || m.picked != nil {
			t.Fatal("pick leaked into another panel")
		}
	}
}

func TestLatePlacementCannotReplaceANewerQueueSnapshot(t *testing.T) {
	m, a := pickModel()
	m.UI.QueueCursor = 1
	m.normal("p")
	m.UI.QueueCursor = 3
	cmd := m.normal("p")
	reply := cmd()
	a.e.Append([]music.Track{{ID: "new"}}, false)
	latest := a.snapshot()
	m.Update(stateMsg{s: latest})
	m.Update(reply)
	if music.QueueKey(m.State.Queue) != music.QueueKey(latest.Queue) || len(m.State.Queue) != 6 || m.picked != nil {
		t.Fatal("late placement overwrote the newer queue")
	}
}

type cancellablePickAPI struct {
	*pickAPI
	started chan struct{}
}

func (a *cancellablePickAPI) Browse(ctx context.Context, _ catalog.Query) (catalog.Page, error) {
	close(a.started)
	<-ctx.Done()
	return catalog.Page{}, ctx.Err()
}

func TestCancelCollectionStopsReadAndIgnoresItsLateReply(t *testing.T) {
	m, a := pickModel()
	api := &cancellablePickAPI{pickAPI: a, started: make(chan struct{})}
	m.api = api
	m.UI.Focus = 1
	m.items = []catalog.Item{{ID: "collection", Play: &catalog.Query{Page: "collection"}}}
	cmd := m.normal("p")
	defer m.CancelPick()
	replies := make(chan any, 1)
	go func() { replies <- cmd() }()
	select {
	case <-api.started:
	case <-time.After(time.Second):
		t.Fatal("collection did not begin")
	}
	m.Update(key("esc"))
	select {
	case reply := <-replies:
		m.Update(reply)
	case <-time.After(time.Second):
		t.Fatal("collection read ignored cancellation")
	}
	if m.picked != nil || m.HasToast() || a.calls != 0 {
		t.Fatal("canceled collection caused a late error, pickup or write")
	}
}
