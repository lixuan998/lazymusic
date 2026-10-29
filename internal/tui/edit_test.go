package tui

import (
	"context"
	"errors"
	"testing"

	"lazymusic/internal/catalog"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
)

type editingAPI struct {
	*fakeAPI
	actions  catalog.Actions
	mutation catalog.Mutation
	calls    int
	page     catalog.Page
}

func (a *editingAPI) Actions(context.Context, catalog.Subject) (catalog.Actions, error) {
	return a.actions, nil
}
func (a *editingAPI) Mutate(_ context.Context, m catalog.Mutation) (catalog.MutationResult, error) {
	a.mutation = m
	a.calls++
	return catalog.MutationResult{Message: "Accepted", Pending: true}, nil
}
func (a *editingAPI) Browse(context.Context, catalog.Query) (catalog.Page, error) { return a.page, nil }

func editModel() (*Model, *editingAPI) {
	m, f := testModel()
	a := &editingAPI{fakeAPI: f, actions: catalog.Actions{AddLibrary: true, Playlists: &catalog.Query{Source: "demo", Page: "destination"}}}
	m.api = a
	m.items = []catalog.Item{{Source: "demo", ID: "song", Kind: "songs", Title: "Song", Track: &music.Track{Source: "demo", ID: "song"}}}
	m.restoring = false
	return m, a
}

func TestAddMenuUsesActualWorkInContentQueueAndPlayer(t *testing.T) {
	for _, focus := range []int{1, 2, 3} {
		m, a := editModel()
		m.UI.Focus = focus
		m.State.Queue = []music.Track{{Source: "demo", ID: "queue"}}
		m.State.Playback.Track = &music.Track{Source: "demo", ID: "player"}
		cmd := m.normal("a")
		m.Update(cmd())
		for i, c := range m.edit.choices {
			if c.op == catalog.AddLibrary {
				m.edit.cursor = i
			}
		}
		_, cmd = m.Update(decodeKey(t, "\r"))
		if !m.mutationBusy {
			t.Fatal("write not marked in flight")
		}
		_, duplicate := m.Update(decodeKey(t, "\r"))
		if duplicate != nil {
			t.Fatal("repeated Enter submitted a duplicate")
		}
		m.Update(cmd())
		want := map[int]string{1: "song", 2: "queue", 3: "player"}[focus]
		if a.calls != 1 || a.mutation.Subject.ID != want || a.mutation.Op != catalog.AddLibrary {
			t.Fatalf("wrong mutation: %+v", a.mutation)
		}
		if a.lastCommand.Op != "" || m.UI.Focus != focus {
			t.Fatal("library write changed playback or focus")
		}
	}
}

func TestPlaylistPickerFiltersReadonlyAndForeignSources(t *testing.T) {
	m, a := editModel()
	a.page = catalog.Page{Items: []catalog.Item{{ID: "read-only", Source: "demo"}, {ID: "foreign", Source: "other", Writable: true}, {ID: "editable", Source: "demo", Writable: true, Title: "Editable"}}}
	m.Update(m.normal("a")())
	for i, c := range m.edit.choices {
		if c.op == "playlist" {
			m.edit.cursor = i
		}
	}
	_, cmd := m.Update(decodeKey(t, "\r"))
	m.Update(cmd())
	if len(m.edit.playlists) != 1 || m.edit.playlists[0].ID != "editable" {
		t.Fatal("picker included noneditable destination")
	}
	_, cmd = m.Update(decodeKey(t, "\r"))
	m.Update(cmd())
	if a.mutation.PlaylistID != "editable" || a.mutation.Subject.ID != "song" {
		t.Fatalf("wrong playlist addition: %+v", a.mutation)
	}
}

func TestCreatePlaylistWithEmptyListAndTypedVimKeys(t *testing.T) {
	m, a := editModel()
	m.State.Source.Capabilities.PlaylistCreate = true
	m.UI.Location = session.Location{Page: "playlists", Title: "Playlists"}
	m.items = nil
	m.normal("a")
	for _, r := range "静夜 qdj" {
		m.Update(key(string(r)))
	}
	_, cmd := m.Update(decodeKey(t, "\r"))
	m.Update(cmd())
	if a.mutation.Op != catalog.CreatePlaylist || a.mutation.Name != "静夜 qdj" || m.prompt {
		t.Fatalf("name treated as shortcuts: %+v", a.mutation)
	}
}

func TestCanceledAddDialogIgnoresLateReply(t *testing.T) {
	m, _ := editModel()
	m.normal("a")
	generation := m.editGeneration
	m.Update(decodeKey(t, "\x1b[27u"))
	m.Update(editOptionsMsg{generation: generation, err: errors.New("late")})
	m.Update(playlistPageMsg{generation: generation, err: errors.New("late")})
	if m.edit != nil || m.HasToast() {
		t.Fatal("canceled lookup reopened dialog or reported a late error")
	}
}

func TestUnsupportedLibraryRemovalIsExplainedAndNeverSent(t *testing.T) {
	m, a := editModel()
	m.UI.Location.Page = "library"
	a.actions = catalog.Actions{RemovalNote: "Deletion unavailable"}
	m.Update(m.normal("d")())
	if m.edit == nil || m.edit.phase != "info" || m.edit.note != "Deletion unavailable" {
		t.Fatal("missing deletion limitation")
	}
	m.Update(decodeKey(t, "\r"))
	if a.calls != 0 {
		t.Fatal("unsupported deletion was submitted")
	}
}
