package daemon

import (
	"context"
	"testing"
	"time"

	"lazymusic/internal/catalog"
	"lazymusic/internal/lyrics"
)

type lyricsBrowser struct {
	testBrowser
	calls int
}

func (b *lyricsBrowser) Lyrics(context.Context, catalog.Subject) (lyrics.Document, error) {
	b.calls++
	return lyrics.Document{Lines: []lyrics.Line{{Start: time.Second, Text: "Example line"}}, Provider: "test"}, nil
}
func TestLyricsIPCIsBoundedCachedAndSourceScoped(t *testing.T) {
	dir := t.TempDir()
	b := &lyricsBrowser{}
	opts := testOptions(dir)
	opts.Browser = b
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	c := serveTest(t, s, dir)
	for range 2 {
		d, err := c.Lyrics(context.Background(), catalog.Subject{Source: "local", ID: "song"})
		if err != nil || len(d.Lines) != 1 || s.State().EngineReady {
			t.Fatal("lyrics lost or started audio", err)
		}
	}
	if b.calls != 1 {
		t.Fatal("lyrics cache missed")
	}
	for _, subject := range []catalog.Subject{{Source: "apple", ID: "song"}, {Source: "local"}} {
		if _, err = c.Lyrics(context.Background(), subject); err == nil {
			t.Fatal("accepted invalid source or identity")
		}
	}
}
