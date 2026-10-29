package engine

import (
	"lazymusic/internal/music"
	"lazymusic/internal/session"
	"lazymusic/internal/vibez/player/browserless"
	"testing"
	"time"
)

func TestInsertPreservesCurrentSongPositionAndOrder(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, at := range []int{0, 1, 2, 3} {
			var e Engine
			if native {
				e = &Native{p: &browserless.Player{}}
			} else {
				e = NewQueue(session.Playback{})
			}
			original := []music.Track{{ID: "a", Duration: time.Minute}, {ID: "b", Duration: time.Minute}, {ID: "c", Duration: time.Minute}}
			if native {
				e.(*Native).p.RestoreTracks(toNativeTracks(original), 1, 17*time.Second)
			} else {
				e = NewQueue(session.Playback{Queue: original, Index: 1, Position: 17 * time.Second})
			}
			if err := e.Insert([]music.Track{{ID: "x"}, {ID: "y"}}, at); err != nil {
				t.Fatal(err)
			}
			state, queue, index := e.State()
			if len(queue) != 5 || queue[at].ID != "x" || queue[at+1].ID != "y" || state.Track == nil || state.Track.ID != "b" || state.Playing || state.Position != 17*time.Second || queue[index].ID != "b" {
				t.Fatalf("native=%v at=%d state=%+v queue=%v", native, at, state, queue)
			}
			if err := e.Insert([]music.Track{{ID: "bad"}}, 6); err == nil {
				t.Fatal("out-of-range insertion accepted")
			}
		}
	}
}

func TestInsertIntoEmptyQueueDoesNotStartAudio(t *testing.T) {
	for _, e := range []Engine{NewQueue(session.Playback{}), &Native{p: &browserless.Player{}}} {
		if err := e.Insert([]music.Track{{ID: "one"}, {ID: "two"}}, 0); err != nil {
			t.Fatal(err)
		}
		s, q, i := e.State()
		if len(q) != 2 || i != 0 || s.Playing || s.Loading || s.Track == nil || s.Track.ID != "one" {
			t.Fatal("insertion did not preserve paused empty-queue behavior")
		}
	}
}
