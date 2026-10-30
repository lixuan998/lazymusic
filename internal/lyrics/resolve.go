package lyrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"lazymusic/internal/music"
)

// Resolve prefers the exact LRCLIB lookup, then uses a separately identified
// public lyric catalog when the first host is unavailable or has no timing.
// Credentials and audio never cross into either public metadata request.
func Resolve(ctx context.Context, track music.Track, transport http.RoundTripper) (Document, error) {
	firstCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	first, firstErr := Lookup(firstCtx, track, transport)
	cancel()
	if len(first.Lines) > 0 {
		return first, nil
	}
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	second, secondErr := lookupNetEase(ctx, track, transport)
	if len(second.Lines) > 0 {
		return second, nil
	}
	if secondErr == nil || firstErr == nil {
		return Document{}, nil
	}
	return Document{}, fmt.Errorf("synchronized lyrics providers are unavailable")
}
