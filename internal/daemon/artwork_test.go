package daemon

import (
	"context"
	"testing"

	"lazymusic/internal/artwork"
	"lazymusic/internal/catalog"
)

type artworkBrowser struct {
	testBrowser
	calls int
}

func (b *artworkBrowser) Artwork(context.Context, catalog.Subject) (artwork.Image, error) {
	b.calls++
	return artwork.Image{Width: 1, Height: 1, RGB: []byte{120, 20, 40}}, nil
}

func TestArtworkIPCIsSourceScopedCachedAndDoesNotStartPlayback(t *testing.T) {
	dir := t.TempDir()
	b := &artworkBrowser{}
	opts := testOptions(dir)
	opts.Browser = b
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	c := serveTest(t, s, dir)
	for range 2 {
		im, err := c.Artwork(context.Background(), catalog.Subject{Source: "local", ID: "song"})
		if err != nil || !im.Valid() || s.State().EngineReady {
			t.Fatal("artwork IPC started audio or lost pixels", err)
		}
	}
	if b.calls != 1 {
		t.Fatal("same cover was fetched twice")
	}
	if _, err := c.Artwork(context.Background(), catalog.Subject{Source: "apple", ID: "song"}); err == nil {
		t.Fatal("accepted another source")
	}
	if _, err := c.Artwork(context.Background(), catalog.Subject{Source: "local"}); err == nil {
		t.Fatal("accepted empty identity")
	}
}
