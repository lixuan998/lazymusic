package daemon

import (
	"context"
	"reflect"
	"testing"
	"time"

	"lazymusic/internal/music"
	"lazymusic/internal/session"
)

func placementService(t *testing.T) (*Service, []music.Track) {
	t.Helper()
	s, err := New(testOptions(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	q := []music.Track{{Source: music.Local, ID: "same", Title: "first", Duration: time.Minute}, {Source: music.Local, ID: "same", Title: "second", Duration: time.Minute}, {Source: music.Local, ID: "third", Duration: time.Minute}, {Source: music.Local, ID: "fourth", Duration: time.Minute}}
	if err = s.execute(context.Background(), Command{Op: "replace", Tracks: q, Index: 1}); err != nil {
		t.Fatal(err)
	}
	if err = s.execute(context.Background(), Command{Op: "seek", Position: 17 * time.Second}); err != nil {
		t.Fatal(err)
	}
	return s, q
}
func undoCurrent(s *Service) error {
	p := s.State().Placement
	return s.execute(context.Background(), Command{Op: "undo-placement", PlacementID: p.ID, PlacementRevision: p.Revision})
}

func TestQueuePlacementUndoRedoTogglesUntilNextPlacement(t *testing.T) {
	s, original := placementService(t)
	ctx := context.Background()
	place := Command{Op: "move", Placement: true, Index: 0, To: 3, ExpectedQueue: music.QueueKey(original)}
	if err := s.execute(ctx, place); err != nil {
		t.Fatal(err)
	}
	placed := s.State()
	id := placed.Placement.ID
	if placed.Placement.Undone || !placed.Placement.Available || placed.Playback.Track.Title != "second" {
		t.Fatal("placement lost current duplicate occurrence")
	}
	for step := 0; step < 4; step++ {
		if err := undoCurrent(s); err != nil {
			t.Fatal(err)
		}
		state := s.State()
		want := placed.Queue
		if step%2 == 0 {
			want = original
		}
		if !reflect.DeepEqual(state.Queue, want) || state.Placement.ID != id || state.Placement.Revision != uint64(step+1) || state.Placement.Undone != (step%2 == 0) {
			t.Fatal("u did not alternate undo/redo")
		}
		if !state.Playback.Playing || state.Playback.Track.Title != "second" || state.Playback.Position < 17*time.Second {
			t.Fatal("undo rewound or interrupted playback")
		}
	}
	old := s.State().Placement
	state := s.State()
	if err := s.execute(ctx, Command{Op: "move", Placement: true, Index: 0, To: 1, ExpectedQueue: music.QueueKey(state.Queue)}); err != nil {
		t.Fatal(err)
	}
	if s.State().Placement.ID == id {
		t.Fatal("next placement kept the old history")
	}
	before := s.State().Queue
	if err := s.execute(ctx, Command{Op: "undo-placement", PlacementID: old.ID, PlacementRevision: old.Revision}); err == nil {
		t.Fatal("stale placement undo accepted")
	}
	if !reflect.DeepEqual(before, s.State().Queue) {
		t.Fatal("stale request changed the queue")
	}
}

func TestContentBatchPlacementUndoRedoAndCurrentRemoval(t *testing.T) {
	s, original := placementService(t)
	ctx := context.Background()
	batch := []music.Track{{Source: music.Local, ID: "same", Title: "inserted occurrence"}, {Source: music.Local, ID: "episode", MediaType: "episode", Kind: "stations"}}
	if err := s.execute(ctx, Command{Op: "insert", Placement: true, Tracks: batch, To: 1, ExpectedQueue: music.QueueKey(original)}); err != nil {
		t.Fatal(err)
	}
	if err := undoCurrent(s); err != nil {
		t.Fatal(err)
	}
	state := s.State()
	if !reflect.DeepEqual(state.Queue, original) || state.Index != 1 || !state.Playback.Playing || state.Playback.Track.Title != "second" {
		t.Fatal("batch undo interrupted the original track")
	}
	if err := undoCurrent(s); err != nil {
		t.Fatal(err)
	}
	state = s.State()
	if len(state.Queue) != 6 || state.Queue[1].Title != "inserted occurrence" || state.Queue[2].MediaType != "episode" {
		t.Fatal("redo lost collection order/metadata")
	}
	// Playing an inserted occurrence is a separate playback action. Undoing
	// its insertion stops that occurrence and leaves the next survivor paused.
	if err := s.execute(ctx, Command{Op: "select", Index: 1}); err != nil {
		t.Fatal(err)
	}
	if err := undoCurrent(s); err != nil {
		t.Fatal(err)
	}
	state = s.State()
	if !reflect.DeepEqual(state.Queue, original) || state.Playback.Playing || state.Playback.Position != 0 || state.Playback.Track.Title != "second" {
		t.Fatal("undo resumed or removed the wrong duplicate")
	}
	if err := undoCurrent(s); err != nil {
		t.Fatal(err)
	}
	if s.State().Playback.Playing {
		t.Fatal("redo restarted playback")
	}
}

func TestPlacementRevisionRejectsUnknownOutcomeRetriesAndSurvivesRestore(t *testing.T) {
	s, original := placementService(t)
	ctx := context.Background()
	if err := s.execute(ctx, Command{Op: "move", Placement: true, Index: 0, To: 3, ExpectedQueue: music.QueueKey(original)}); err != nil {
		t.Fatal(err)
	}
	p := s.State().Placement
	command := Command{Op: "undo-placement", PlacementID: p.ID, PlacementRevision: p.Revision}
	if err := s.execute(ctx, command); err != nil {
		t.Fatal(err)
	}
	if err := s.execute(ctx, command); err == nil {
		t.Fatal("retry of an already applied undo became redo")
	}
	if !s.State().Placement.Undone {
		t.Fatal("duplicate request changed history")
	}
	restored, err := New(testOptions(s.opts.Dir))
	if err != nil {
		t.Fatal(err)
	}
	state := restored.State()
	if state.Playback.Playing || state.Placement == nil || !state.Placement.Undone || !state.Placement.Available {
		t.Fatal("cold restore lost placement history or started audio")
	}
	if err = undoCurrent(restored); err != nil {
		t.Fatal(err)
	}
	if restored.State().Placement.Undone || restored.State().Playback.Playing {
		t.Fatal("redo after restart did not remain paused")
	}
	saved, err := session.Load(s.opts.Dir)
	if err != nil || saved.Placement == nil || saved.Placement.Undone {
		t.Fatal("redo was not saved")
	}
}

func TestOtherQueueEditsInvalidatePlacementEvenIfOrderReturnsToSameIDs(t *testing.T) {
	s, original := placementService(t)
	ctx := context.Background()
	if err := s.execute(ctx, Command{Op: "move", Placement: true, Index: 0, To: 3, ExpectedQueue: music.QueueKey(original)}); err != nil {
		t.Fatal(err)
	}
	placed := s.State().Queue
	if err := s.execute(ctx, Command{Op: "replace", Tracks: placed}); err != nil {
		t.Fatal(err)
	}
	if s.State().Placement.Available {
		t.Fatal("new occurrences with identical IDs resurrected old history")
	}
	if err := undoCurrent(s); err == nil {
		t.Fatal("undid placement after queue replacement")
	}
	if !reflect.DeepEqual(s.State().Queue, placed) {
		t.Fatal("invalid undo mutated the new queue")
	}
}

func TestPlacementSurvivesTailAppendsAndUnsuccessfulPlacements(t *testing.T) {
	s, original := placementService(t)
	ctx := context.Background()
	batch := []music.Track{{Source: music.Local, ID: "inserted"}}
	if err := s.execute(ctx, Command{Op: "insert", Placement: true, Tracks: batch, To: 1, ExpectedQueue: music.QueueKey(original)}); err != nil {
		t.Fatal(err)
	}
	id := s.State().Placement.ID
	if err := undoCurrent(s); err != nil {
		t.Fatal(err)
	}
	tail := music.Track{Source: music.Local, ID: "appended"}
	if err := s.execute(ctx, Command{Op: "append", Tracks: []music.Track{tail}}); err != nil {
		t.Fatal(err)
	}
	// Radio refill also appends directly to the engine outside commandMu.
	if err := s.engine.Append([]music.Track{tail}, false); err != nil {
		t.Fatal(err)
	}
	if !s.State().Placement.Available {
		t.Fatal("tail append invalidated indexed placement")
	}
	state := s.State()
	if err := s.execute(ctx, Command{Op: "move", Placement: true, Index: 0, To: 0, ExpectedQueue: music.QueueKey(state.Queue)}); err != nil {
		t.Fatal(err)
	}
	if err := s.execute(ctx, Command{Op: "insert", Placement: true, Tracks: batch, To: 1, ExpectedQueue: "stale"}); err == nil {
		t.Fatal("stale placement accepted")
	}
	if s.State().Placement.ID != id || !s.State().Placement.Undone {
		t.Fatal("no-op or failed placement replaced redo history")
	}
	if err := undoCurrent(s); err != nil {
		t.Fatal(err)
	}
	state = s.State()
	if len(state.Queue) != 7 || state.Queue[1].ID != "inserted" || state.Queue[5].ID != tail.ID || state.Queue[6].ID != tail.ID || state.Index != 2 || !state.Playback.Playing {
		t.Fatal("redo lost appended tracks or interrupted playback")
	}
}

func TestPlacementIntoEmptyQueueCanUndoToEmptyAndRedoPaused(t *testing.T) {
	s, err := New(testOptions(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	batch := []music.Track{{Source: music.Local, ID: "one"}, {Source: music.Local, ID: "two"}}
	if err = s.execute(ctx, Command{Op: "insert", Placement: true, Tracks: batch, To: 0, ExpectedQueue: music.QueueKey(nil)}); err != nil {
		t.Fatal(err)
	}
	for step := 0; step < 4; step++ {
		if err = undoCurrent(s); err != nil {
			t.Fatal(err)
		}
		state := s.State()
		wantLen := 0
		if step%2 == 1 {
			wantLen = 2
		}
		if len(state.Queue) != wantLen || state.Playback.Playing || state.Index != 0 || !state.Placement.Available {
			t.Fatal("empty queue undo/redo failed or started playback")
		}
	}
}
