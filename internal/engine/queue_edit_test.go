package engine

import (
	"testing"
	"time"

	"lazymusic/internal/audio"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
	"lazymusic/internal/vibez/player/browserless"
)

type placementAudio struct {
	stops, opens int
	position     time.Duration
}

func (a *placementAudio) Open(string) error            { a.opens++; return nil }
func (a *placementAudio) Play() error                  { return nil }
func (a *placementAudio) Pause()                       {}
func (a *placementAudio) Stop()                        { a.stops++ }
func (a *placementAudio) Seek(p time.Duration)         { a.position = p }
func (a *placementAudio) SetVolume(float64)            {}
func (a *placementAudio) SetEqualizer([]float64) error { return nil }
func (a *placementAudio) Status() audio.Status {
	return audio.Status{Position: a.position, Duration: time.Minute}
}
func (a *placementAudio) Close() {}

func TestQueueEditorsRemoveInsertedRangeWithoutReloadingSurvivingAudio(t *testing.T) {
	for _, kind := range []string{"queue", "native", "local"} {
		t.Run(kind, func(t *testing.T) {
			q := []music.Track{{ID: "a", Duration: time.Minute}, {ID: "b", Duration: time.Minute}, {ID: "c", Duration: time.Minute}}
			var e Engine
			out := &placementAudio{position: 17 * time.Second}
			switch kind {
			case "queue":
				e = NewQueue(session.Playback{Queue: q, Index: 1, Position: 17 * time.Second})
				e.Play()
			case "native":
				n := &Native{p: &browserless.Player{}}
				n.p.RestoreTracks(toNativeTracks(q), 1, 17*time.Second)
				e = n
			case "local":
				e = &Local{d: NewQueue(session.Playback{Queue: q, Index: 1, Position: 17 * time.Second}), out: out, loaded: "b", playing: true}
			}
			editor := e.(QueueEditor)
			_, original, _ := e.State()
			batch := []music.Track{{ID: "b", Title: "inserted duplicate"}, {ID: "x"}}
			result, err := editor.EditQueue(music.QueueEdit{Op: "insert", To: 0, Tracks: batch, ExpectedQueue: music.QueueKey(original)})
			if err != nil {
				t.Fatal(err)
			}
			result, err = editor.EditQueue(music.QueueEdit{Op: "remove-range", Index: 0, Count: 2, ExpectedQueue: result.QueueKey})
			if err != nil {
				t.Fatal(err)
			}
			s, after, index := e.State()
			if len(after) != 3 || index != 1 || s.Track == nil || s.Track.ID != "b" || s.Position < 17*time.Second || result.RemovedCurrent {
				t.Fatal("undo lost the surviving playing occurrence")
			}
			if kind != "native" && !s.Playing {
				t.Fatal("undo paused surviving audio")
			}
			if out.stops != 0 || out.opens != 0 {
				t.Fatal("local decoder was reloaded")
			}
			result, err = editor.EditQueue(music.QueueEdit{Op: "remove-range", Index: 1, Count: 1, ExpectedQueue: result.QueueKey})
			if err != nil {
				t.Fatal(err)
			}
			s, after, index = e.State()
			if !result.RemovedCurrent || s.Playing || s.Loading || s.Position != 0 || s.Track == nil || s.Track.ID != "c" || index != 1 || len(after) != 2 {
				t.Fatal("removing current occurrence did not select a paused successor")
			}
			if kind == "local" && out.stops != 1 {
				t.Fatal("removed local audio was not stopped")
			}
		})
	}
}
