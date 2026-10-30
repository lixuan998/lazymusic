package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"lazymusic/internal/catalog"
	"lazymusic/internal/daemon"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
)

func editorialSource() music.Source {
	return music.Source{ID: "example", Capabilities: music.Capabilities{Home: true, Library: true, Playlists: true, CatalogSearch: true}, Pages: []music.SourcePage{
		{ID: "new", Title: "New", Query: music.Query{Source: "example", Page: "feed", Term: "new-feed", Section: "songs"}},
		{ID: "radio", Title: "Radio", Query: music.Query{Source: "example", Page: "feed", Term: "radio-feed", Section: "songs"}},
	}}
}

func TestSourcePagesUseOpaqueQueriesAndRestoreIndependentCategories(t *testing.T) {
	m, api := testModel()
	m.State.Source = editorialSource()
	m.restoring = false
	m.UI.NavIndex = m.findNav("radio")
	runTestCommand(m, m.activateNav())
	if m.UI.NavPage != "radio" || m.query().Page != "feed" || m.query().Term != "radio-feed" {
		t.Fatal("UI guessed a source-specific page")
	}
	a := catalog.Tab{ID: "stations", Title: "Stations", Query: m.query()}
	a.Query.ID = a.ID
	b := a
	b.ID = "moods"
	b.Title = "Find your mood"
	b.Query.ID = b.ID
	m.Update(pageMsg{key: m.query().Key(), generation: m.generation, page: catalog.Page{Tabs: []catalog.Tab{a, b}, ActiveTab: a.ID}})
	runTestCommand(m, m.category(true))
	m.Update(pageMsg{key: m.query().Key(), generation: m.generation, page: catalog.Page{Tabs: []catalog.Tab{a, b}, ActiveTab: b.ID}})
	if !strings.Contains(ansi.Strip(m.homeTabBar()), "Find your mood") {
		t.Fatal("generic feed has no category tabs")
	}
	m.UI.NavIndex = m.findNav("new")
	m.activateNav()
	if len(m.tabs) != 0 || m.UI.Location.ID != "" {
		t.Fatal("category tabs leaked into a different page")
	}
	m.UI.NavIndex = m.findNav("radio")
	m.activateNav()
	if m.UI.Location.ID != "moods" || m.query().Term != "radio-feed" {
		t.Fatal("lost feed's last selected category")
	}
	m.remember()
	restored := New(context.Background(), api, daemon.State{Source: m.State.Source, UI: m.UI}, nil)
	if restored.UI.NavIndex != restored.findNav("radio") || restored.UI.Location.ID != "moods" || restored.query().Term != "radio-feed" {
		t.Fatal("source page session did not restore")
	}
}

func TestNavigationMigratesIndicesWhenSourcePagesAreInserted(t *testing.T) {
	_, api := testModel()
	for _, tc := range []struct {
		page     string
		index    int
		selected string
	}{
		{"library", 1, "library"}, {"home", 4, "login"}, {"search", 3, "search"},
	} {
		u := session.DefaultUI()
		u.NavPage = tc.page
		u.Location.Page = tc.page
		u.NavIndex = tc.index
		m := New(context.Background(), api, daemon.State{Source: editorialSource(), UI: u}, nil)
		if m.UI.NavIndex != m.findNav(tc.selected) || m.UI.Location.Page != tc.page {
			t.Fatalf("legacy index changed page or opened Login: %+v", m.UI)
		}
	}
}

func TestSourceWithoutExtraPagesHasNoEditorialNavigation(t *testing.T) {
	m, _ := testModel()
	m.State.Source = music.Source{ID: music.Local, Capabilities: music.Capabilities{Library: true, Folders: true}}
	for _, n := range m.navItems() {
		if n.page == "home" || n.page == "new" || n.page == "radio" {
			t.Fatalf("unsupported local navigation: %s", n.page)
		}
	}
}

func TestUnsupportedResourceIsVisibleAndExplainsPlaybackLimitWithoutMovingFocus(t *testing.T) {
	for _, action := range []string{"enter", "y", "p", "l"} {
		t.Run(action, func(t *testing.T) {
			m, api := testModel()
			m.UI.Focus = 1
			m.restoring = false
			m.items = []catalog.Item{{ID: "station", Kind: "stations", Title: "Personal station", PlaybackNote: "Station playback is not connected yet"}}
			if len(m.visible()) != 1 {
				t.Fatal("unsupported resource hidden")
			}
			runTestCommand(m, m.normal(action))
			if api.lastCommand.Op != "" || m.UI.Focus != 1 || m.toast == nil || !strings.Contains(m.toast.Message, "Station playback") {
				t.Fatalf("unsupported item action: focus=%d toast=%+v command=%+v", m.UI.Focus, m.toast, api.lastCommand)
			}
		})
	}
}

func TestEnterTunesStationWithoutTryingToOpenAChildPage(t *testing.T) {
	m, api := testModel()
	m.UI.Focus = 1
	m.restoring = false
	q := music.Query{Source: "demo", Page: "opaque-station", ID: "personal", Term: "Personal station", Continuous: true}
	m.items = []catalog.Item{{Source: "demo", ID: "personal", Kind: "stations", Title: "Personal station", Play: &q}}
	state := m.State
	runTestCommand(m, m.normal("enter"))
	if api.lastCommand.Op != "replace" || api.lastCommand.Query == nil || !api.lastCommand.Query.Continuous || api.lastCommand.Query.ID != "personal" {
		t.Fatal("Enter did not tune station")
	}
	before := api.lastCommand
	m.State = state
	runTestCommand(m, m.normal("p"))
	if m.picked != nil || (api.lastCommand.Op != before.Op || api.lastCommand.Query != before.Query) || m.toast == nil {
		t.Fatal("station pick pretended to expand an endless collection")
	}
}
