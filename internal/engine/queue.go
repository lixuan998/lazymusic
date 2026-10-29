package engine

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"lazymusic/internal/music"
	"lazymusic/internal/session"
)

type Queue struct {
	mu   sync.Mutex
	s    music.State
	q    []music.Track
	idx  int
	last time.Time
}

func NewQueue(saved session.Playback) *Queue { d := &Queue{}; _ = d.Restore(saved); return d }
func (d *Queue) refresh() {
	now := time.Now()
	if d.s.Playing {
		d.s.Position += now.Sub(d.last)
		for len(d.q) > 0 && d.s.Track != nil && d.s.Track.Duration > 0 && d.s.Position >= d.s.Track.Duration {
			d.s.Position -= d.s.Track.Duration
			if d.s.RepeatMode == music.RepeatModeOne {
			} else if d.idx+1 < len(d.q) {
				d.idx++
			} else if d.s.RepeatMode == music.RepeatModeAll {
				d.idx = 0
			} else {
				d.s.Playing = false
				d.s.Position = 0
				break
			}
			d.selectTrack()
		}
	}
	d.last = now
}
func (d *Queue) selectTrack() {
	if len(d.q) == 0 {
		d.s.Track = nil
		return
	}
	t := d.q[d.idx]
	d.s.Track = &t
}
func (d *Queue) State() (music.State, []music.Track, int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.refresh()
	return d.s, session.CleanTracks(d.q), d.idx
}
func (d *Queue) Restore(s session.Playback) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.q = session.CleanTracks(s.Queue)
	d.idx = max(0, min(len(d.q)-1, s.Index))
	d.s = music.State{Position: s.Position, Volume: s.Volume, RepeatMode: s.Repeat, ShuffleMode: s.Shuffle}
	d.selectTrack()
	d.last = time.Now()
	return nil
}
func (d *Queue) Replace(q []music.Track, i int, play bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.q = session.CleanTracks(q)
	d.idx = max(0, min(len(q)-1, i))
	d.s.Position = 0
	d.s.Playing = play && len(q) > 0
	d.selectTrack()
	d.last = time.Now()
	return nil
}
func (d *Queue) Play() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.refresh()
	d.s.Playing = len(d.q) > 0
	return nil
}
func (d *Queue) Pause() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.refresh()
	d.s.Playing = false
	return nil
}
func (d *Queue) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.s.Playing = false
	d.s.Position = 0
	return nil
}
func (d *Queue) Next() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.q) == 0 {
		return nil
	}
	d.refresh()
	if d.s.ShuffleMode {
		d.idx = rand.Intn(len(d.q))
	} else {
		d.idx = (d.idx + 1) % len(d.q)
	}
	d.s.Position = 0
	d.s.Playing = true
	d.selectTrack()
	return nil
}
func (d *Queue) Previous() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.q) == 0 {
		return nil
	}
	d.refresh()
	if d.s.Position <= 3*time.Second {
		d.idx = (d.idx + len(d.q) - 1) % len(d.q)
	}
	d.s.Position = 0
	d.s.Playing = true
	d.selectTrack()
	return nil
}
func (d *Queue) Seek(pos time.Duration) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.refresh()
	d.s.Position = max(0, pos)
	if d.s.Track != nil {
		d.s.Position = min(d.s.Position, d.s.Track.Duration)
	}
	return nil
}
func (d *Queue) SetVolume(v float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.s.Volume = max(0, min(1, v))
	return nil
}
func (d *Queue) SetRepeat(v int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.s.RepeatMode = v
	return nil
}
func (d *Queue) SetShuffle(v bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.s.ShuffleMode = v
	return nil
}
func (d *Queue) Append(q []music.Track, next bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.refresh()
	if next && len(d.q) > 0 {
		i := d.idx + 1
		d.q = append(append(append([]music.Track{}, d.q[:i]...), q...), d.q[i:]...)
	} else {
		d.q = append(d.q, q...)
	}
	if d.s.Track == nil {
		d.idx = 0
		d.selectTrack()
	}
	return nil
}
func (d *Queue) Remove(i int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if i < 0 || i >= len(d.q) {
		return fmt.Errorf("queue index out of range")
	}
	d.refresh()
	current := i == d.idx
	d.q = append(d.q[:i], d.q[i+1:]...)
	if i < d.idx {
		d.idx--
	}
	d.idx = max(0, min(d.idx, len(d.q)-1))
	if current {
		d.s.Position = 0
		d.s.Playing = false
	}
	if len(d.q) == 0 {
		d.s.Playing = false
		d.s.Position = 0
	}
	d.selectTrack()
	return nil
}
func (d *Queue) Move(from, to int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if from < 0 || to < 0 || from >= len(d.q) || to >= len(d.q) {
		return fmt.Errorf("queue index out of range")
	}
	t := d.q[from]
	d.q = append(d.q[:from], d.q[from+1:]...)
	d.q = append(d.q, music.Track{})
	copy(d.q[to+1:], d.q[to:])
	d.q[to] = t
	if d.idx == from {
		d.idx = to
	} else if from < d.idx && to >= d.idx {
		d.idx--
	} else if from > d.idx && to <= d.idx {
		d.idx++
	}
	d.selectTrack()
	return nil
}
func (d *Queue) Clear() error { return d.Replace(nil, 0, false) }
func (d *Queue) Close() error { return d.Pause() }
