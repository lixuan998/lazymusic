package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"lazymusic/internal/catalog"
	"lazymusic/internal/engine"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
	"lazymusic/internal/volume"
)

func serveTest(t *testing.T, s *Service, dir string) *Client {
	t.Helper()
	path, err := SocketPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: s.Handler()}
	go server.Serve(ln)
	t.Cleanup(func() { server.Close(); _ = os.Remove(path) })
	c, err := NewClient(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestDetachPreservesLivePlayerAndColdRestorePauses(t *testing.T) {
	dir := t.TempDir()
	s, err := New(testOptions(dir))
	if err != nil {
		t.Fatal(err)
	}
	c := serveTest(t, s, dir)
	ctx := context.Background()
	if _, err = c.Attach(ctx); err != nil {
		t.Fatal(err)
	}
	tracks := []music.Track{{Source: music.Local, ID: "one", Duration: time.Minute}, {Source: music.Local, ID: "two", Duration: time.Minute}}
	if _, err = c.Command(ctx, Command{Op: "replace", Tracks: tracks}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Command(ctx, Command{Op: "seek", Position: 20 * time.Second}); err != nil {
		t.Fatal(err)
	}
	u := session.DefaultUI()
	u.Location = session.Location{Page: "search", Section: "songs", Scope: "library", Query: "晨光"}
	u.Focus = 2
	u.QueueCursor = 1
	if err = c.Detach(ctx, u, 10); err != nil {
		t.Fatal(err)
	}
	before, _ := c.State(ctx)
	time.Sleep(50 * time.Millisecond)
	after, _ := c.State(ctx)
	if !after.Playback.Playing || after.Playback.Position <= before.Playback.Position {
		t.Fatal("detach stopped playback")
	}
	a, err := c.Attach(ctx)
	if err != nil || a.State.UI.Location.Query != "晨光" || a.State.UI.Focus != 2 {
		t.Fatalf("lost UI: %+v %v", a, err)
	}
	if err = c.Quit(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := New(testOptions(dir))
	if err != nil {
		t.Fatal(err)
	}
	r := restored.State()
	if r.Playback.Playing || r.Playback.Position < 20*time.Second || len(r.Queue) != 2 || r.UI.QueueCursor != 1 {
		t.Fatalf("cold restore=%+v", r)
	}
}

func TestStaleUIUpdatesCannotOverwriteNewerView(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(testOptions(dir))
	c := serveTest(t, s, dir)
	ctx := context.Background()
	_, _ = c.Attach(ctx)
	newer := session.DefaultUI()
	newer.Location.Title = "newer"
	older := session.DefaultUI()
	older.Location.Title = "older"
	if err := c.SaveUI(ctx, newer, 20); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveUI(ctx, older, 19); err != nil {
		t.Fatal(err)
	}
	state, _ := c.State(ctx)
	if state.UI.Location.Title != "newer" {
		t.Fatal("stale view overwrote latest view")
	}
}

func TestSecondFrontendCannotStealLiveSession(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(testOptions(dir))
	one := serveTest(t, s, dir)
	_, err := one.Attach(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	two, _ := NewClient(dir)
	defer two.Close()
	if _, err = two.Attach(context.Background()); err == nil {
		t.Fatal("two frontends acquired UI ownership")
	}
}

func TestProfileLockIsExclusive(t *testing.T) {
	dir := t.TempDir()
	one, err := acquireLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer one.Close()
	if two, err := acquireLock(dir); err == nil {
		two.Close()
		t.Fatal("duplicate daemon lock acquired")
	}
}

func TestQuitRejectsLateSavesAndCommands(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(testOptions(dir))
	c := serveTest(t, s, dir)
	ctx := context.Background()
	_, _ = c.Attach(ctx)
	u := session.DefaultUI()
	u.SearchQuery = "saved before quit"
	if err := c.SaveUI(ctx, u, 1); err != nil {
		t.Fatal(err)
	}
	if err := c.Quit(ctx); err != nil {
		t.Fatal(err)
	}
	u.SearchQuery = "late response"
	if err := c.SaveUI(ctx, u, 2); err == nil {
		t.Fatal("accepted a save after shutdown began")
	}
	if _, err := c.Command(ctx, Command{Op: "play"}); err == nil {
		t.Fatal("accepted playback after shutdown began")
	}
	saved, err := session.Load(dir)
	if err != nil || saved.UI.SearchQuery != "saved before quit" {
		t.Fatalf("shutdown snapshot changed: %v", err)
	}
}

func TestCrossSourceQueueAndBrowseAreRejected(t *testing.T) {
	s, _ := New(testOptions(t.TempDir()))
	if err := s.execute(context.Background(), Command{Op: "replace", Tracks: []music.Track{{Source: "apple", ID: "same-id"}}}); err == nil {
		t.Fatal("mixed source entered the queue")
	}
	if _, err := s.browse(context.Background(), catalog.Query{Source: "apple", Page: "home"}, false); err == nil {
		t.Fatal("routed another source to active provider")
	}
}

// Inject a silent queue and mixer: unit tests never change the host audio device.
type testBrowser struct{}

func (testBrowser) Browse(context.Context, catalog.Query) (catalog.Page, error) {
	return catalog.Page{}, nil
}

type testMixer struct{ level float64 }

func (m *testMixer) Read(context.Context) (float64, bool, error) { return m.level, false, nil }
func (m *testMixer) Write(_ context.Context, v float64) error    { m.level = v; return nil }
func testOptions(dir string) Options {
	return Options{Dir: dir, Backend: music.Local, Browser: testBrowser{}, Mixer: volume.WithDriver(&testMixer{level: .5}), Factory: func(_ context.Context, p session.Playback) (engine.Engine, error) { return engine.NewQueue(p), nil }}
}

func TestModeCycleAndSystemVolumeAreSavedWithoutAppGain(t *testing.T) {
	dir := t.TempDir()
	opts := testOptions(dir)
	mixer := &testMixer{level: .2}
	opts.Mixer = volume.WithDriver(mixer)
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, want := range []struct {
		repeat  int
		shuffle bool
	}{{1, false}, {2, false}, {0, true}, {0, false}} {
		if err = s.execute(ctx, Command{Op: "playback-mode", ModeStep: 1}); err != nil {
			t.Fatal(err)
		}
		got := s.State()
		if got.Playback.RepeatMode != want.repeat || got.Playback.ShuffleMode != want.shuffle {
			t.Fatal("wrong mode", got.Playback)
		}
	}
	if err = s.execute(ctx, Command{Op: "volume", Volume: .7}); err != nil {
		t.Fatal(err)
	}
	if s.State().Playback.Volume != .7 || mixer.level != .7 {
		t.Fatal("volume did not use OS driver")
	}
	state, _, _ := s.engine.State()
	if state.Volume == .7 {
		t.Fatal("system volume also changed application gain")
	}
}
func TestUnsupportedQualityIsRejected(t *testing.T) {
	s, err := New(testOptions(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.execute(context.Background(), Command{Op: "quality", Quality: "lossless"}); err == nil {
		t.Fatal("unsupported quality accepted")
	}
}

type canceledBrowser struct{}

func (canceledBrowser) Browse(context.Context, catalog.Query) (catalog.Page, error) {
	return catalog.Page{}, fmt.Errorf("Apple request: %w", context.Canceled)
}
func TestCanceledNavigationRequestIsNotLoggedAsError(t *testing.T) {
	opts := testOptions(t.TempDir())
	opts.Browser = canceledBrowser{}
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/v1/browse?q=%7B%22page%22%3A%22library%22%7D", nil)
	s.Handler().ServeHTTP(httptest.NewRecorder(), req)
	for _, e := range s.logs.Entries() {
		if e.Level == "error" || strings.Contains(e.Message, "context canceled") {
			t.Fatal("canceled browse generated error log", e.Message)
		}
	}
}

type equalizedQueue struct {
	*engine.Queue
	eq music.Equalizer
}

func (e *equalizedQueue) SetEqualizer(eq music.Equalizer) error { e.eq = eq; return nil }
func TestEqualizerIsAppliedAndSurvivesColdRestore(t *testing.T) {
	dir := t.TempDir()
	opts := testOptions(dir)
	var player *equalizedQueue
	opts.Factory = func(_ context.Context, p session.Playback) (engine.Engine, error) {
		player = &equalizedQueue{Queue: engine.NewQueue(p), eq: p.Equalizer}
		return player, nil
	}
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.execute(context.Background(), Command{Op: "equalizer", EQBand: 3, EQDelta: 4}); err != nil {
		t.Fatal(err)
	}
	if player.eq.Gains[3] != 4 || s.State().Equalizer.Gains[3] != 4 {
		t.Fatal("equalizer not applied")
	}
	restored, err := New(opts)
	if err != nil || restored.State().Equalizer.Gains[3] != 4 {
		t.Fatal("equalizer not saved", err)
	}
}
