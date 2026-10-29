package engine

import (
	"context"
	"fmt"
	"lazymusic/internal/audio"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
	localfiles "lazymusic/internal/sources/local"
	"sync"
	"time"
)

type Local struct {
	mu                    sync.Mutex
	d                     *Queue
	out                   audio.Output
	root, loaded, problem string
	playing               bool
	done, stopped         chan struct{}
	once                  sync.Once
}

func NewLocal(_ context.Context, dir string, saved session.Playback) (Engine, error) {
	cfg, err := localfiles.ReadConfig(dir)
	if err != nil {
		return nil, err
	}
	out, err := audio.New()
	if err != nil {
		return nil, err
	}
	if err = out.SetEqualizer(saved.Equalizer.Effective()); err != nil {
		out.Close()
		return nil, err
	}
	l := &Local{d: NewQueue(saved), out: out, root: cfg.Root, done: make(chan struct{}), stopped: make(chan struct{})}
	go func() {
		defer close(l.stopped)
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-l.done:
				return
			case <-tick.C:
				l.mu.Lock()
				l.syncLocked()
				l.mu.Unlock()
			}
		}
	}()
	return l, nil
}
func (l *Local) syncLocked() {
	if l.loaded == "" {
		return
	}
	s := l.out.Status()
	l.d.s.Position = s.Position
	if s.Duration > 0 && len(l.d.q) > 0 {
		l.d.q[l.d.idx].Duration = s.Duration
		l.d.selectTrack()
	}
	if s.Error != "" {
		l.problem = s.Error
		l.playing = false
		l.out.Pause()
		return
	}
	if s.Ended && l.playing {
		l.playing = false
		l.loaded = ""
		l.d.s.Position = 0
		if l.d.s.RepeatMode == music.RepeatModeOne {
			_ = l.playLocked()
		} else if l.d.idx+1 < len(l.d.q) || l.d.s.RepeatMode == music.RepeatModeAll || l.d.s.ShuffleMode {
			_ = l.d.Next()
			l.d.s.Playing = false
			_ = l.playLocked()
		}
	}
}
func (l *Local) State() (music.State, []music.Track, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	s, q, i := l.d.State()
	s.Playing = l.playing
	s.Error = l.problem
	return s, q, i
}
func (l *Local) playLocked() error {
	if len(l.d.q) == 0 {
		return fmt.Errorf("queue is empty; choose a song first")
	}
	t := l.d.q[l.d.idx]
	if t.Source != "" && t.Source != music.Local {
		return fmt.Errorf("LOCAL cannot play a track from %s", t.Source)
	}
	if l.loaded != t.ID {
		path, err := localfiles.Resolve(l.root, t.ID)
		if err != nil {
			l.problem = err.Error()
			l.playing = false
			return err
		}
		if err = l.out.Open(path); err != nil {
			l.problem = err.Error()
			l.loaded = ""
			l.playing = false
			return err
		}
		l.loaded = t.ID
		l.out.SetVolume(1)
		l.out.Seek(l.d.s.Position)
	}
	if err := l.out.Play(); err != nil {
		l.problem = err.Error()
		l.playing = false
		return err
	}
	l.playing = true
	l.problem = ""
	return nil
}
func (l *Local) Play() error { l.mu.Lock(); defer l.mu.Unlock(); return l.playLocked() }
func (l *Local) Pause() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	l.out.Pause()
	l.playing = false
	return nil
}
func (l *Local) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out.Stop()
	l.playing = false
	l.loaded = ""
	return l.d.Stop()
}
func (l *Local) Restore(p session.Playback) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out.Stop()
	l.loaded = ""
	l.playing = false
	if err := l.out.SetEqualizer(p.Equalizer.Effective()); err != nil {
		return err
	}
	return l.d.Restore(p)
}
func (l *Local) Replace(q []music.Track, i int, play bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out.Stop()
	l.loaded = ""
	l.playing = false
	_ = l.d.Replace(q, i, false)
	if play && len(q) > 0 {
		return l.playLocked()
	}
	return nil
}
func (l *Local) Next() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out.Stop()
	l.loaded = ""
	l.playing = false
	_ = l.d.Next()
	l.d.s.Playing = false
	return l.playLocked()
}
func (l *Local) Previous() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	l.out.Stop()
	l.loaded = ""
	l.playing = false
	_ = l.d.Previous()
	l.d.s.Playing = false
	return l.playLocked()
}
func (l *Local) Seek(p time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	p = max(0, p)
	if l.d.s.Track != nil && l.d.s.Track.Duration > 0 {
		p = min(p, l.d.s.Track.Duration)
	}
	l.d.s.Position = p
	if l.loaded != "" {
		l.out.Seek(p)
	}
	return nil
}
func (l *Local) SetVolume(v float64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out.SetVolume(v)
	return l.d.SetVolume(v)
}
func (l *Local) SetRepeat(v int) error   { l.mu.Lock(); defer l.mu.Unlock(); return l.d.SetRepeat(v) }
func (l *Local) SetShuffle(v bool) error { l.mu.Lock(); defer l.mu.Unlock(); return l.d.SetShuffle(v) }
func (l *Local) Append(q []music.Track, next bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.d.Append(q, next)
}
func (l *Local) Move(a, b int) error { l.mu.Lock(); defer l.mu.Unlock(); return l.d.Move(a, b) }
func (l *Local) Remove(i int) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	if i == l.d.idx {
		l.out.Stop()
		l.loaded = ""
		l.playing = false
	}
	return l.d.Remove(i)
}
func (l *Local) Clear() error { return l.Replace(nil, 0, false) }
func (l *Local) Close() error {
	l.once.Do(func() {
		close(l.done)
		<-l.stopped
		l.mu.Lock()
		defer l.mu.Unlock()
		l.out.Close()
		l.playing = false
	})
	return nil
}

func (l *Local) SetEqualizer(eq music.Equalizer) error {
	if err := eq.Validate(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.out.SetEqualizer(eq.Effective())
}
