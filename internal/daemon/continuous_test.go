package daemon

import (
	"context"
	"strings"
	"testing"
	"time"

	"lazymusic/internal/engine"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
)

type testStationEngine struct {
	*engine.Queue
	query              *music.Query
	starts, selections int
}

func (e *testStationEngine) StartContinuous(_ context.Context, q music.Query) error {
	e.starts++
	e.query = &q
	return e.Queue.Replace([]music.Track{{Source: music.Local, ID: "radio-song", Duration: time.Minute}}, 0, true)
}
func (e *testStationEngine) State() (music.State, []music.Track, int) {
	s, q, i := e.Queue.State()
	s.Continuous = e.query
	return s, q, i
}
func (e *testStationEngine) Select(i int) error {
	e.selections++
	_, q, _ := e.Queue.State()
	return e.Queue.Replace(q, i, true)
}

func TestContinuousCommandsReachTunerAndSurviveDetachAndSnapshot(t *testing.T) {
	dir := t.TempDir()
	opts := testOptions(dir)
	e := &testStationEngine{Queue: engine.NewQueue(session.Playback{})}
	opts.Factory = func(context.Context, session.Playback) (engine.Engine, error) { return e, nil }
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	c := serveTest(t, s, dir)
	ctx := context.Background()
	query := music.Query{Source: music.Local, Page: "opaque-station", ID: "personal", Term: "My station", Continuous: true}
	if _, err = c.Attach(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := c.Command(ctx, Command{Op: "replace", Query: &query})
	if err != nil || e.starts != 1 || state.Playback.Continuous == nil || !state.Playback.Playing {
		t.Fatalf("station command %+v %v", state, err)
	}
	if _, err = c.Command(ctx, Command{Op: "select", Index: 0}); err != nil || e.selections != 1 {
		t.Fatal("selection dropped station context")
	}
	if err = c.Detach(ctx, session.DefaultUI(), 1); err != nil {
		t.Fatal(err)
	}
	if !s.State().Playback.Playing {
		t.Fatal("detach stopped radio")
	}
	snapshot, err := session.Load(dir)
	if err != nil || snapshot.Playback.Continuous == nil || snapshot.Playback.Continuous.ID != "personal" {
		t.Fatalf("station not persisted: %v", err)
	}
	// Before native playback is created after a cold restart, state must still
	// expose the station context while remaining paused.
	reset, err := New(testOptions(dir))
	if err != nil {
		t.Fatal(err)
	}
	state = reset.State()
	if state.Playback.Playing || state.Playback.Continuous == nil || state.Playback.Continuous.Term != "My station" {
		t.Fatal("cold state lost station or started audio")
	}
}

func TestStreamErrorsCannotExposeSignedPathsInState(t *testing.T) {
	s, err := New(testOptions(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	s.lastError = "station stream: https://media.example/private-signature/audio/segment.aac?token=private-query"
	message := s.State().Error
	if strings.Contains(message, "private-signature") || strings.Contains(message, "private-query") || !strings.Contains(message, "media.example/[REDACTED]") {
		t.Fatal("state exposed signed media URL")
	}
}
