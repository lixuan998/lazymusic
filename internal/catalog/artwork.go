package catalog

import (
	"context"
	"lazymusic/internal/artwork"
)

// ArtworkProvider resolves a source-owned identity into bounded cover pixels.
// Missing art is an empty image, not a playback error. Implementations must not
// accept a caller-supplied URL or expose authorization material to image hosts.
type ArtworkProvider interface {
	Artwork(context.Context, Subject) (artwork.Image, error)
}
