package daemon

import (
	"context"
	"lazymusic/internal/music"
	"testing"
	"time"
)

func TestGuardedQueueInsertIsOrderedAndDoesNotInterruptPlayback(t *testing.T) {
	s, err := New(testOptions(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	q := []music.Track{{Source: music.Local, ID: "a", Duration: time.Minute}, {Source: music.Local, ID: "b", Duration: time.Minute}}
	if err = s.execute(ctx, Command{Op: "replace", Tracks: q, Index: 1}); err != nil {
		t.Fatal(err)
	}
	if err = s.execute(ctx, Command{Op: "seek", Position: 17 * time.Second}); err != nil {
		t.Fatal(err)
	}
	before := s.State()
	insert := Command{Op: "insert", To: 1, Tracks: []music.Track{{Source: music.Local, ID: "x"}, {Source: music.Local, ID: "y"}}, ExpectedQueue: music.QueueKey(before.Queue)}
	if err = s.execute(ctx, insert); err != nil {
		t.Fatal(err)
	}
	after := s.State()
	if len(after.Queue) != 4 || after.Queue[1].ID != "x" || after.Queue[2].ID != "y" || after.Index != 3 || !after.Playback.Playing || after.Playback.Track.ID != "b" || after.Playback.Position < 17*time.Second {
		t.Fatalf("placement disrupted playback: %+v", after)
	}
	if err = s.execute(ctx, insert); err == nil {
		t.Fatal("stale insertion snapshot accepted")
	}
	if len(s.State().Queue) != 4 {
		t.Fatal("stale insertion partially changed queue")
	}
	if err = s.execute(ctx, Command{Op: "move", Index: 0, To: 3, ExpectedQueue: insert.ExpectedQueue}); err == nil {
		t.Fatal("stale move snapshot accepted")
	}
	for _, op := range []string{"select", "remove"} {
		if err = s.execute(ctx, Command{Op: op, Index: 0, ExpectedQueue: insert.ExpectedQueue}); err == nil {
			t.Fatalf("stale %s snapshot accepted", op)
		}
	}
}
