package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"lazymusic/internal/catalog"
	"lazymusic/internal/daemon"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
)

type fakeAPI struct {
	detached, quit bool
	saved          session.UI
	lastCommand    daemon.Command
}

func (f *fakeAPI) State(context.Context) (daemon.State, error) { return daemon.State{}, nil }
func (f *fakeAPI) Browse(context.Context, catalog.Query) (catalog.Page, error) {
	return catalog.Page{}, nil
}
func (f *fakeAPI) Command(_ context.Context, c daemon.Command) (daemon.State, error) {
	f.lastCommand = c
	return daemon.State{}, nil
}
func (f *fakeAPI) SaveUI(_ context.Context, u session.UI, _ uint64) error { f.saved = u; return nil }
func (f *fakeAPI) Detach(_ context.Context, u session.UI, _ uint64) error {
	f.saved = u
	f.detached = true
	return nil
}
func (f *fakeAPI) Quit(context.Context) error { f.quit = true; return nil }
func key(s string) tea.KeyPressMsg            { return tea.KeyPressMsg{Text: s, Code: []rune(s)[0]} }
func testModel() (*Model, *fakeAPI) {
	f := &fakeAPI{}
	return New(context.Background(), f, daemon.State{Backend: "demo", UI: session.DefaultUI()}, nil), f
}

func TestQuitPromptBackgroundAndFullQuit(t *testing.T) {
	for _, choice := range []string{"q", "Q"} {
		m, api := testModel()
		_, cmd := m.Update(key("q"))
		if cmd != nil || !m.prompt || api.detached || api.quit {
			t.Fatal("first q must only open prompt")
		}
		_, cmd = m.Update(key(choice))
		if cmd == nil {
			t.Fatal("missing exit command")
		}
		msg := cmd()
		_, _ = m.Update(msg)
		if choice == "q" && (!api.detached || api.quit) {
			t.Fatal("q did not detach")
		}
		if choice == "Q" && (!api.quit || api.detached) {
			t.Fatal("Q did not quit service")
		}
	}
}

func TestQIsTextWhileEditing(t *testing.T) {
	m, api := testModel()
	m.mode = "query"
	_, _ = m.Update(key("q"))
	if m.prompt || m.draft != "q" || api.detached {
		t.Fatal("typing q triggered quit")
	}
}

func TestStaleSearchResponseDoesNotReplaceCurrentResults(t *testing.T) {
	m, _ := testModel()
	m.UI.Location = session.Location{Page: "search", Section: "songs", Scope: "library", Query: "new"}
	m.generation = 3
	_, _ = m.Update(pageMsg{key: catalog.Query{Page: "search", Section: "songs", Scope: "apple", Term: "old"}.Key(), generation: 2, page: catalog.Page{Items: []catalog.Item{{ID: "old"}}}})
	if len(m.items) != 0 {
		t.Fatal("stale results replaced new scope/query")
	}
}

func TestViewFitsTerminalAndIncludesContext(t *testing.T) {
	for _, width := range []int{60, 99, 100, 140} {
		m, _ := testModel()
		m.width = width
		m.height = 28
		m.items = []catalog.Item{{ID: "1", Title: "中文曲目 · A long title", Subtitle: "Artist"}}
		m.restoring = false
		v := m.View().Content
		lines := strings.Split(v, "\n")
		if len(lines) != 28 {
			t.Fatalf("width %d: height %d", width, len(lines))
		}
		for i, line := range lines {
			if got := ansi.StringWidth(line); got > width {
				t.Fatalf("width %d line %d=%d", width, i, got)
			}
		}
		if !strings.Contains(ansi.Strip(v), "中文曲目") {
			t.Fatal("missing Unicode title")
		}
	}
}

func TestVimTopBottomUseFocusedPanel(t *testing.T) {
	m, _ := testModel()
	m.State.Queue = fixtureTracks()
	m.UI.Focus = 2
	m.UI.QueueCursor = 2
	_, _ = m.Update(key("g"))
	_, _ = m.Update(key("g"))
	if m.UI.QueueCursor != 0 {
		t.Fatal("gg did not move queue cursor")
	}
	m.UI.Focus = 0
	_, _ = m.Update(key("G"))
	if m.UI.NavIndex != len(m.navItems())-1 {
		t.Fatal("G did not move navigation to last entry")
	}
}

func TestRemovingLastQueueRowKeepsSelectionInRange(t *testing.T) {
	m, _ := testModel()
	m.UI.QueueCursor = 2
	_, _ = m.Update(commandMsg{s: daemon.State{ObservedAt: time.Now(), Queue: fixtureTracks()[:2]}})
	if m.UI.QueueCursor != 1 {
		t.Fatal("queue selection points beyond the remaining rows")
	}
}

func TestHierarchyAndDirectionalFocusAreSeparate(t *testing.T) {
	m, _ := testModel()
	q := catalog.Query{Source: "demo", Page: "detail", Section: "songs", Kind: "albums", ID: "album"}
	m.items = []catalog.Item{{ID: "album", Title: "Album", Open: &q, Play: &q}}
	m.restoring = false
	m.normal("l")
	if m.UI.Location.Page != "detail" || m.UI.Focus != 1 {
		t.Fatal("l did not enter child in the content panel")
	}
	m.normal("h")
	if m.UI.Location.Page != "home" || m.UI.Focus != 1 {
		t.Fatal("h did not return to parent")
	}
	m.normal("ctrl+l")
	if m.UI.Focus != 2 {
		t.Fatal("Ctrl-l did not focus right panel")
	}
	m.normal("ctrl+j")
	if m.UI.Focus != 3 {
		t.Fatal("Ctrl-j did not focus bottom player")
	}
	m.normal("ctrl+k")
	if m.UI.Focus != 2 {
		t.Fatal("Ctrl-k did not restore the upper panel")
	}
	m.normal("ctrl+h")
	if m.UI.Focus != 1 {
		t.Fatal("Ctrl-h did not focus left panel")
	}
}
func TestHomeActionsUseBackendProvidedQueries(t *testing.T) {
	for _, op := range []string{"play-selection", "append", "play-next"} {
		m, api := testModel()
		q := catalog.Query{Source: "demo", Page: "opaque-collection", Kind: "opaque", ID: "same-id"}
		m.items = []catalog.Item{{ID: "same-id", Title: "Recommendation", Play: &q}}
		cmd := m.selectItem(op)
		if cmd == nil {
			t.Fatal("missing playable action")
		}
		cmd()
		if api.lastCommand.Query == nil || api.lastCommand.Query.Source != "demo" || api.lastCommand.Query.Page != "opaque-collection" {
			t.Fatal("UI invented a provider URL or kind")
		}
	}
}

func TestLocalCapabilitiesHideOnlinePagesAndSearchScopes(t *testing.T) {
	f := &fakeAPI{}
	m := New(context.Background(), f, daemon.State{Backend: "local", Source: music.Source{ID: "local", Name: "LOCAL", Capabilities: music.Capabilities{Library: true, Folders: true}}, UI: session.DefaultUI()}, nil)
	if m.UI.Location.Page != "library" || m.UI.SearchScope != "library" {
		t.Fatal("LOCAL inherited Apple landing page or scope")
	}
	for _, n := range m.navItems() {
		if n.page == "home" || n.page == "playlists" {
			t.Fatal("LOCAL shows an unsupported page")
		}
	}
}
func TestLegacyCursorRestoresAfterSourceQualifiedQueryMigration(t *testing.T) {
	f := &fakeAPI{}
	u := session.DefaultUI()
	q := catalog.Query{Page: "home", Section: "songs"}
	u.Cursors[q.Key()] = session.Cursor{Index: 40, Offset: 30, ID: "saved"}
	m := New(context.Background(), f, daemon.State{Backend: "demo", UI: u}, nil)
	if m.restore.Index != 40 || m.restore.Offset != 30 {
		t.Fatal("source prefix discarded the old cursor")
	}
}

func TestHomeRestoresRepeatedAlbumWithinItsOriginalGroup(t *testing.T) {
	m, _ := testModel()
	m.generation = 1
	first := catalog.Item{Source: "demo", Kind: "albums", ID: "same", Group: "Discover", Title: "Album"}
	second := first
	second.Group = "Listen Again"
	m.restore = session.Cursor{Index: 1, ID: second.CursorID()}
	m.restoring = true
	_, _ = m.Update(pageMsg{key: m.query().Key(), generation: 1, page: catalog.Page{Items: []catalog.Item{first, second}}})
	if m.cursor != 1 {
		t.Fatal("Home restored the duplicate album in a different recommendation group")
	}
}

func TestHomeCategoryTabsShowOnlySelectedCategoryAndRememberCursors(t *testing.T) {
	m, _ := testModel()
	m.restoring = false
	a := catalog.Tab{ID: "one", Title: "专属推荐歌单", Query: catalog.Query{Source: "demo", Page: "home", Section: "songs", ID: "one"}}
	b := catalog.Tab{ID: "two", Title: "最近播放", Query: catalog.Query{Source: "demo", Page: "home", Section: "songs", ID: "two"}}
	m.tabs = []catalog.Tab{a, b}
	m.UI.Location.ID = "one"
	m.items = []catalog.Item{{ID: "first"}, {ID: "second"}}
	m.cursor = 1
	m.category(true)
	if m.UI.Location.ID != "two" || len(m.items) != 0 {
		t.Fatal("category switch kept another category's recommendations")
	}
	m.category(false)
	if m.restore.Index != 1 {
		t.Fatal("switching categories lost the prior category cursor")
	}
	m.UI.Location.ID = "two"
	if !strings.Contains(ansi.Strip(m.homeTabBar()), "最近播放") {
		t.Fatal("active category is not visible in top tabs")
	}
}
func TestLoginActionKeepsPlayerAttachedInSameFrame(t *testing.T) {
	m, api := testModel()
	cmd := m.runCommand("login")
	if cmd == nil {
		t.Fatal("missing in-app Login transition")
	}
	m.Update(cmd())
	if api.detached || api.quit || m.ExitAction != "" || !m.LoginSelected() || m.UI.Focus != 1 {
		t.Fatal("Login transition stopped the player instead of opening source selection")
	}
}

func TestMovingOntoLoginNeverOpensItUntilConfirmed(t *testing.T) {
	for _, movement := range []string{"j", "k", "G"} {
		for _, confirm := range []string{"l", "enter"} {
			m, api := testModel()
			m.UI.Focus = 0
			m.UI.NavPage = "home"
			m.UI.NavIndex = len(m.navItems()) - 2
			if movement == "k" {
				m.UI.NavIndex = 0
			}
			before := m.UI.Location
			m.normal(movement)
			if m.exiting || api.detached || m.UI.Location != before || m.UI.NavIndex != len(m.navItems())-1 {
				t.Fatalf("%s opened Login while only moving the cursor", movement)
			}
			cmd := m.normal(confirm)
			if cmd == nil || m.exiting || m.UI.Focus != 1 {
				t.Fatalf("%s did not explicitly open Login", confirm)
			}
			runTestCommand(m, cmd)
			if api.detached || api.quit || m.ExitAction != "" || !m.LoginSelected() || m.UI.Focus != 1 {
				t.Fatal("confirmed Login did not detach safely")
			}
		}
	}
}
func TestRestoreKeepsUnconfirmedLoginSelectionOnExistingPage(t *testing.T) {
	m, api := testModel()
	m.UI.NavPage = "home"
	m.UI.NavIndex = len(m.navItems()) - 1
	m.UI.Focus = 0
	restored := New(context.Background(), api, daemon.State{Backend: "demo", UI: m.UI}, nil)
	if restored.UI.NavIndex != len(restored.navItems())-1 || restored.UI.Location.Page != "home" || restored.exiting {
		t.Fatal("restore opened or lost unconfirmed Login selection")
	}
}

func runTestCommand(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			runTestCommand(m, child)
		}
		return
	}
	m.Update(msg)
}

func fixtureTracks() []music.Track {
	return []music.Track{{ID: "one", Duration: time.Minute}, {ID: "two", Duration: time.Minute}, {ID: "three", Duration: time.Minute}}
}
