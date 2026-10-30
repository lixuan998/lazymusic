package catalog

import (
	"context"
	"lazymusic/internal/lyrics"
)

// LyricsProvider is optional. An empty document means synchronized lyrics are
// unavailable. UI and IPC never receive source URLs or authorization material.
type LyricsProvider interface {
	Lyrics(context.Context, Subject) (lyrics.Document, error)
}
