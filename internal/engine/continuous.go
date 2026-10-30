package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"lazymusic/internal/eventlog"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
	"lazymusic/internal/video"
)

// Continuous owns an endless source above the ordinary native song engine.
// Fetches run outside the control lock. A generation and cancellation prevent
// a slow old station from appending to a new queue or resuming after Pause.
type Continuous struct {
	Engine
	mu                        sync.Mutex
	fetch                     func(context.Context, music.Query) ([]music.Track, error)
	query                     *music.Query
	wanted, refilling, closed bool
	generation                uint64
	cancel                    context.CancelFunc
	retryAt                   time.Time
	problem                   string
	done                      chan struct{}
}

func newContinuous(e Engine, fetch func(context.Context, music.Query) ([]music.Track, error)) *Continuous {
	c := &Continuous{Engine: e, fetch: fetch, done: make(chan struct{})}
	go func() {
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-c.done:
				return
			case <-tick.C:
				c.refill()
			}
		}
	}()
	return c
}
func (c *Continuous) resetLocked() {
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.generation++
	c.query = nil
	c.wanted = false
	c.refilling = false
	c.problem = ""
	c.retryAt = time.Time{}
}
func (c *Continuous) State() (music.State, []music.Track, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, q, i := c.Engine.State()
	if c.query != nil {
		v := *c.query
		s.Continuous = &v
	}
	if c.problem != "" && s.Error == "" {
		s.Error = c.problem
	}
	return s, q, i
}
func (c *Continuous) StartContinuous(ctx context.Context, q music.Query) error {
	q.Next, q.Refresh = "", false
	if !q.Continuous {
		return fmt.Errorf("selection is not continuous")
	}
	tracks, err := c.fetch(ctx, q)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(tracks) == 0 {
		return fmt.Errorf("station returned no playable tracks")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("player is closed")
	}
	c.resetLocked()
	if err = c.Engine.Replace(tracks, 0, true); err != nil {
		return err
	}
	if len(tracks) == 1 && (tracks[0].MediaType == "episode" || tracks[0].MediaType == "live") {
		return nil
	}
	c.query = &q
	c.wanted = true
	return nil
}
func (c *Continuous) refill() {
	c.mu.Lock()
	if c.closed || c.query == nil || !c.query.Continuous || !c.wanted || c.refilling || time.Now().Before(c.retryAt) {
		c.mu.Unlock()
		return
	}
	s, q, i := c.Engine.State()
	if s.Error != "" || len(q)-i > 4 {
		c.mu.Unlock()
		return
	}
	query, gen := *c.query, c.generation
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	c.cancel = cancel
	c.refilling = true
	c.mu.Unlock()
	go func() {
		defer cancel()
		tracks, err := c.fetch(ctx, query)
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed || gen != c.generation {
			return
		}
		if err == nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		c.refilling = false
		c.cancel = nil
		if err != nil || len(tracks) == 0 {
			c.problem = "Station refill failed; retrying"
			if err != nil {
				c.problem += " · " + err.Error()
			}
			c.retryAt = time.Now().Add(5 * time.Second)
			return
		}
		c.problem = ""
		s, q, i := c.Engine.State()
		if len(q)+len(tracks) > 10000 {
			c.problem = "Station queue reached 10000 tracks; clear it to continue"
			c.wanted = false
			return
		}
		if err = c.Engine.Append(tracks, false); err != nil {
			c.problem = err.Error()
			return
		}
		// If the buffer ended during a slow fetch, advance into the new batch.
		// wanted is cleared by Pause/Stop, so neither can be undone here.
		if c.wanted && !s.Playing && !s.Loading && s.Error == "" && i >= len(q)-1 {
			_ = c.Engine.Next()
		}
		// Retain a useful history while bounding long-running station sessions.
		for i > 200 {
			if err = c.Engine.Remove(0); err != nil {
				break
			}
			i--
		}
	}()
}
func (c *Continuous) Restore(p session.Playback) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resetLocked()
	if err := c.Engine.Restore(p); err != nil {
		return err
	}
	if p.Continuous != nil {
		q := *p.Continuous
		c.query = &q
	}
	return nil
}
func (c *Continuous) Replace(q []music.Track, i int, play bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resetLocked()
	return c.Engine.Replace(q, i, play)
}
func (c *Continuous) Play() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wanted = c.query != nil
	return c.Engine.Play()
}
func (c *Continuous) Pause() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wanted = false
	return c.Engine.Pause()
}
func (c *Continuous) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wanted = false
	return c.Engine.Stop()
}
func (c *Continuous) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resetLocked()
	return c.Engine.Clear()
}
func (c *Continuous) Append(q []music.Track, next bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Engine.Append(q, next)
}
func (c *Continuous) Insert(q []music.Track, i int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Engine.Insert(q, i)
}
func (c *Continuous) Remove(i int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, _, current := c.Engine.State()
	if current == i {
		c.wanted = false
	}
	return c.Engine.Remove(i)
}
func (c *Continuous) Move(i, j int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Engine.Move(i, j)
}
func (c *Continuous) Next() error {
	c.mu.Lock()
	c.wanted = c.query != nil
	c.mu.Unlock()
	c.refill()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Engine.Next()
}
func (c *Continuous) Previous() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wanted = c.query != nil
	return c.Engine.Previous()
}
func (c *Continuous) Select(i int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, q, _ := c.Engine.State()
	c.wanted = c.query != nil
	return c.Engine.Replace(q, i, true)
}
func (c *Continuous) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	c.resetLocked()
	close(c.done)
	return c.Engine.Close()
}
func (c *Continuous) SetQuality(q string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.Engine.(interface{ SetQuality(string) error }); ok {
		return e.SetQuality(q)
	}
	return fmt.Errorf("quality selection unavailable")
}
func (c *Continuous) SetEqualizer(q music.Equalizer) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.Engine.(interface{ SetEqualizer(music.Equalizer) error }); ok {
		return e.SetEqualizer(q)
	}
	return fmt.Errorf("equalizer unavailable")
}
func (c *Continuous) Events() <-chan eventlog.Entry {
	if e, ok := c.Engine.(interface{ Events() <-chan eventlog.Entry }); ok {
		return e.Events()
	}
	return nil
}
func (c *Continuous) Seek(p time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Engine.Seek(p)
}
func (c *Continuous) SetRepeat(v int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Engine.SetRepeat(v)
}
func (c *Continuous) SetShuffle(v bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Engine.SetShuffle(v)
}
func (c *Continuous) SetVolume(v float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Engine.SetVolume(v)
}

func (c *Continuous) VideoFrame(ctx context.Context, id string) (video.Frame, error) {
	if provider, ok := c.Engine.(video.Provider); ok {
		return provider.VideoFrame(ctx, id)
	}
	return video.Frame{}, nil
}
