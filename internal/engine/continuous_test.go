package engine

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"lazymusic/internal/music"
	"lazymusic/internal/session"
)

func stationQuery() music.Query {
	return music.Query{Source: "example", Page: "station", ID: "personal", Term: "Personal station", Continuous: true}
}
func stationBatch(prefix string) []music.Track {
	var tracks []music.Track
	for i := 0; i < 6; i++ {
		tracks = append(tracks, music.Track{Source: "example", ID: fmt.Sprintf("%s%d", prefix, i), Duration: time.Minute})
	}
	return tracks
}
func awaitStation(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("station transition timed out")
}

func TestStationRefillsWithoutReplacingPlayingTrackAndColdRestorePauses(t *testing.T) {
	var calls atomic.Int32
	c := newContinuous(NewQueue(session.Playback{}), func(context.Context, music.Query) ([]music.Track, error) {
		return stationBatch(fmt.Sprint(calls.Add(1))), nil
	})
	defer c.Close()
	if err := c.StartContinuous(context.Background(), stationQuery()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		c.Next()
	}
	c.refill()
	awaitStation(t, func() bool { _, q, _ := c.State(); return len(q) >= 12 })
	s, q, i := c.State()
	if s.Continuous == nil || s.Continuous.ID != "personal" || !s.Playing || i != 3 || s.Track.ID != q[3].ID {
		t.Fatalf("refill replaced stream: %+v index=%d", s, i)
	}
	saved := Capture(c)
	r := newContinuous(NewQueue(session.Playback{}), func(context.Context, music.Query) ([]music.Track, error) {
		t.Error("cold restore fetched radio")
		return nil, nil
	})
	defer r.Close()
	if err := r.Restore(saved); err != nil {
		t.Fatal(err)
	}
	rs, rq, ri := r.State()
	if rs.Playing || rs.Continuous == nil || len(rq) != len(q) || ri != i {
		t.Fatal("cold restore lost station or started audio")
	}
}

func TestLateStationRefillCannotResumePausedAudioOrPolluteReplacedQueue(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(fmt.Sprint(replace), func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			c := newContinuous(NewQueue(session.Playback{}), func(ctx context.Context, q music.Query) ([]music.Track, error) {
				if calls.Add(1) > 1 {
					close(started)
					<-release
				}
				return stationBatch("batch"), nil
			})
			defer c.Close()
			if err := c.StartContinuous(context.Background(), stationQuery()); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 3; i++ {
				c.Next()
			}
			c.refill()
			<-started
			if replace {
				c.Replace([]music.Track{{ID: "replacement", Duration: time.Minute}}, 0, false)
			} else {
				c.Pause()
			}
			close(release)
			awaitStation(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); return !c.refilling })
			s, q, _ := c.State()
			if s.Playing {
				t.Fatal("late refill resumed a paused player")
			}
			if replace && (len(q) != 1 || q[0].ID != "replacement" || s.Continuous != nil) {
				t.Fatal("old station polluted replacement queue")
			}
		})
	}
}

func TestStationFetchTimeoutClearsInflightAndAllowsRetry(t *testing.T) {
	var calls atomic.Int32
	c := newContinuous(NewQueue(session.Playback{}), func(ctx context.Context, q music.Query) ([]music.Track, error) {
		if calls.Add(1) > 1 {
			return nil, context.DeadlineExceeded
		}
		return stationBatch("initial"), nil
	})
	defer c.Close()
	c.StartContinuous(context.Background(), stationQuery())
	for i := 0; i < 3; i++ {
		c.Next()
	}
	c.refill()
	awaitStation(t, func() bool { s, _, _ := c.State(); return s.Error != "" })
	c.mu.Lock()
	stuck := c.refilling
	c.mu.Unlock()
	if stuck {
		t.Fatal("failed refill remained inflight forever")
	}
	if err := c.Replace(stationBatch("ordinary"), 0, false); err != nil {
		t.Fatal(err)
	}
	s, _, _ := c.State()
	if s.Continuous != nil || s.Error != "" {
		t.Fatal("ordinary queue kept station state")
	}
}

func TestRadioEpisodeIsFiniteAndDoesNotRefillAsAnEndlessStation(t *testing.T) {
	var calls atomic.Int32
	c := newContinuous(NewQueue(session.Playback{}), func(context.Context, music.Query) ([]music.Track, error) {
		calls.Add(1)
		return []music.Track{{ID: "episode", MediaType: "episode", Duration: time.Hour}}, nil
	})
	defer c.Close()
	if err := c.StartContinuous(context.Background(), stationQuery()); err != nil {
		t.Fatal(err)
	}
	c.refill()
	c.Pause()
	c.Play()
	c.refill()
	s, q, _ := c.State()
	if calls.Load() != 1 || len(q) != 1 || s.Continuous != nil {
		t.Fatal("finite radio episode started endless refills")
	}
}
