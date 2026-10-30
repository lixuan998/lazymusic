package engine

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"lazymusic/internal/account"
	"lazymusic/internal/eventlog"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
	applecatalog "lazymusic/internal/sources/apple"
	"lazymusic/internal/vibez/audioquality"
	vplayer "lazymusic/internal/vibez/player"
	"lazymusic/internal/vibez/player/browserless"
	"lazymusic/internal/vibez/provider"
	"lazymusic/internal/vibez/provider/apple"
)

type Native struct {
	events    chan eventlog.Entry
	p         *browserless.Player
	mu        sync.RWMutex
	lastError string
	done      chan struct{}
	closeOnce sync.Once
}

func NewNative(ctx context.Context, dir string, saved session.Playback) (Engine, error) {
	cfg, err := account.ReadConfig(dir)
	if err != nil {
		return nil, err
	}
	if cfg.AppleDeveloperToken == "" || cfg.AppleUserToken == "" {
		return nil, fmt.Errorf("not signed in; choose APPLE MUSIC in Login")
	}
	path := account.CDMPath(dir)
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() <= 100000 {
		return nil, fmt.Errorf("CDM missing; reconnect APPLE MUSIC in Login")
	}
	_ = os.Setenv("VIBEZ_CDM_PATH", path)
	p, err := browserless.New(cfg, apple.New(cfg))
	if err != nil {
		return nil, err
	}
	n := &Native{p: p, done: make(chan struct{}), events: make(chan eventlog.Entry, 128)}
	updates := p.Subscribe()
	p.OnSessionExpired = func() {
		n.mu.Lock()
		n.lastError = "Apple Music authorization expired; reconnect in Login"
		n.mu.Unlock()
	}
	go func() {
		defer close(n.events)
		for {
			select {
			case <-n.done:
				return
			case s, ok := <-updates:
				if !ok {
					return
				}
				for _, line := range s.Logs {
					level := "info"
					lower := strings.ToLower(line)
					if strings.Contains(lower, "error") || s.Error != "" {
						level = "error"
					} else if strings.Contains(lower, "warn") || strings.Contains(lower, "skip") {
						level = "warn"
					}
					select {
					case n.events <- eventlog.Entry{Level: level, Component: "apple", Message: line}:
					default:
					}
				}
				if s.Error != "" {
					n.mu.Lock()
					n.lastError = s.Error
					n.mu.Unlock()
				}
			}
		}
	}()
	if err = n.Restore(saved); err != nil {
		_ = n.Close()
		return nil, err
	}
	tuner := newContinuous(n, applecatalog.New(cfg).StationTracks)
	tuner.mu.Lock()
	if saved.Continuous != nil {
		q := *saved.Continuous
		tuner.query = &q
	}
	tuner.mu.Unlock()
	return tuner, nil
}

func (n *Native) State() (music.State, []music.Track, int) {
	s, _ := n.p.GetState()
	n.mu.RLock()
	s.Error = n.lastError
	n.mu.RUnlock()
	s.Logs = nil
	q, index := fromNativeTracks(n.p.Queue()), n.p.QueueIndex()
	if s.Track != nil && index >= 0 && index < len(q) && q[index].ID == s.Track.ID {
		q[index] = fromNativeTrack(*s.Track)
	}
	return music.State{Track: trackPtr(s.Track), Playing: s.Playing, Loading: s.Loading, Position: s.Position, Volume: s.Volume, Bitrate: s.Bitrate, RepeatMode: s.RepeatMode, ShuffleMode: s.ShuffleMode, Error: s.Error, SkippedID: s.SkippedID}, q, index
}
func (n *Native) Restore(s session.Playback) error {
	if s.Index >= 0 && s.Index < len(s.Queue) && s.Queue[s.Index].MediaType == "live" {
		s.Position = 0
	}
	if err := n.SetEqualizer(s.Equalizer); err != nil {
		return err
	}
	n.p.RestoreTracks(toNativeTracks(s.Queue), s.Index, s.Position)
	_ = n.p.SetVolume(1)
	_ = n.p.SetRepeat(s.Repeat)
	if err := n.p.SetShuffle(s.Shuffle); err != nil {
		return err
	}
	if s.Quality != "" {
		rate, err := audioquality.Parse(s.Quality)
		if err != nil {
			return err
		}
		return n.p.SetAudioBitrate(rate)
	}
	return nil
}
func (n *Native) clearError() { n.mu.Lock(); n.lastError = ""; n.mu.Unlock() }
func (n *Native) Replace(q []music.Track, i int, play bool) error {
	n.clearError()
	n.p.RestoreTracks(toNativeTracks(q), i, 0)
	if play && len(q) > 0 {
		return n.p.Play()
	}
	return nil
}
func (n *Native) Play() error {
	if len(n.p.Queue()) == 0 {
		return fmt.Errorf("queue is empty; choose a song first")
	}
	n.clearError()
	return n.p.Play()
}
func (n *Native) Pause() error { return n.p.Pause() }
func (n *Native) Stop() error {
	_, q, i := n.State()
	_ = n.p.Stop()
	n.p.RestoreTracks(toNativeTracks(q), i, 0)
	_ = n.p.SetVolume(1)
	return nil
}
func (n *Native) Next() error { n.clearError(); return n.p.Next() }
func (n *Native) Previous() error {
	n.clearError()
	s, q, i := n.State()
	if s.Track != nil && s.Track.MediaType == "live" {
		if i <= 0 {
			return fmt.Errorf("live radio cannot restart; choose another queued item")
		}
		return n.Replace(q, i-1, true)
	}
	return n.p.Previous()
}
func (n *Native) Seek(pos time.Duration) error {
	n.clearError()
	s, q, i := n.State()
	if s.Track != nil && s.Track.MediaType == "live" {
		return fmt.Errorf("this is a live stream; seeking is unavailable")
	}
	if !s.Playing && !s.Loading {
		n.p.RestoreTracks(toNativeTracks(q), i, pos)
		return nil
	}
	return n.p.Seek(pos)
}
func (n *Native) SetVolume(v float64) error { return n.p.SetVolume(max(0, min(1, v))) }
func (n *Native) SetRepeat(v int) error     { return n.p.SetRepeat(v) }
func (n *Native) SetShuffle(v bool) error   { return n.p.SetShuffle(v) }
func (n *Native) Append(q []music.Track, next bool) error {
	n.p.AppendTracks(toNativeTracks(q), next)
	return nil
}
func (n *Native) Remove(i int) error {
	_, q, current := n.State()
	if i < 0 || i >= len(q) {
		return fmt.Errorf("queue index out of range")
	}
	if i != current {
		return n.p.RemoveFromQueue(i)
	}
	q = append(q[:i], q[i+1:]...)
	// Removing the current song selects its successor paused, cancels any
	// pending load, and prevents Play from resuming the removed stream.
	n.p.RestoreTracks(toNativeTracks(q), min(i, len(q)-1), 0)
	return nil
}
func (n *Native) Move(a, b int) error { return n.p.MoveInQueue(a, b) }
func (n *Native) Insert(q []music.Track, at int) error {
	return n.p.InsertTracks(toNativeTracks(q), at)
}
func (n *Native) Clear() error { _ = n.p.Stop(); n.p.RestoreTracks(nil, 0, 0); return nil }
func (n *Native) Close() error {
	var err error
	n.closeOnce.Do(func() { close(n.done); _ = n.p.Stop(); err = n.p.Close() })
	return err
}

func fromNativeTrack(t provider.Track) music.Track {
	return music.Track{Kind: t.Kind, MediaType: t.MediaType, Source: music.Apple, ID: t.ID, PlaybackID: t.CatalogID, Title: t.Title, Artist: t.Artist, Album: t.Album, Duration: t.Duration, Genres: t.Genres, AudioVariants: t.AudioVariants}
}
func fromNativeTracks(q []provider.Track) []music.Track {
	out := make([]music.Track, len(q))
	for i, t := range q {
		out[i] = fromNativeTrack(t)
	}
	return out
}
func toNativeTracks(q []music.Track) []provider.Track {
	out := make([]provider.Track, len(q))
	for i, t := range q {
		out[i] = provider.Track{Kind: t.Kind, MediaType: t.MediaType, ID: t.ID, CatalogID: t.PlaybackID, Title: t.Title, Artist: t.Artist, Album: t.Album, Duration: t.Duration}
	}
	return out
}
func trackPtr(t *provider.Track) *music.Track {
	if t == nil {
		return nil
	}
	v := fromNativeTrack(*t)
	return &v
}

func (n *Native) Events() <-chan eventlog.Entry { return n.events }

func (n *Native) SetQuality(id string) error {
	rate, err := audioquality.Parse(id)
	if err != nil {
		return err
	}
	s, q, i := n.State()
	if err = n.p.SetAudioBitrate(rate); err != nil {
		return err
	}
	if s.Track != nil && s.Track.MediaType == "live" {
		s.Position = 0
	}
	n.p.RestoreTracks(toNativeTracks(q), i, s.Position)
	if s.Playing {
		return n.Play()
	}
	return nil
}

func (n *Native) SetEqualizer(eq music.Equalizer) error {
	if err := eq.Validate(); err != nil {
		return err
	}
	bands := vplayer.DefaultEQBands()
	g := eq.Effective()
	for i := range bands {
		bands[i].Gain = g[i]
	}
	return n.p.SetEqualizer(bands)
}
