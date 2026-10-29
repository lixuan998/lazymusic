// Package engine adapts playback to durable sessions independently of a TTY.
package engine

import (
	"context"
	"time"

	"lazymusic/internal/music"
	"lazymusic/internal/session"
)

type Engine interface {
	State() (music.State, []music.Track, int)
	Replace([]music.Track, int, bool) error
	Restore(session.Playback) error
	Play() error
	Pause() error
	Stop() error
	Next() error
	Previous() error
	Seek(time.Duration) error
	SetVolume(float64) error
	SetRepeat(int) error
	SetShuffle(bool) error
	Append([]music.Track, bool) error
	Insert([]music.Track, int) error
	Remove(int) error
	Move(int, int) error
	Clear() error
	Close() error
}

type Factory func(context.Context, session.Playback) (Engine, error)

func Capture(e Engine) session.Playback {
	s, q, i := e.State()
	return session.Playback{Queue: session.CleanTracks(q), Index: i, Position: s.Position, Volume: s.Volume, Repeat: s.RepeatMode, Shuffle: s.ShuffleMode, WasPlaying: s.Playing}
}
