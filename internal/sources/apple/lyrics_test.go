package apple

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"lazymusic/internal/catalog"
)

func TestAppleLyricsResolveTheWorkAndUseSeparatePublicRequests(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/catalog/cn/songs/catalog-id" {
			t.Error("wrong lyric song", r.URL.Path)
		}
		w.Write([]byte(`{"data":[{"id":"catalog-id","attributes":{"name":"Example","artistName":"Example Artist","albumName":"Example Album","durationInMillis":120000}}]}`))
	})
	upstream := http.DefaultTransport
	calls := 0
	c.http.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() != "lrclib.net" {
			return upstream.RoundTrip(r)
		}
		calls++
		if r.Header.Get("Authorization") != "" || r.Header.Get("Music-User-Token") != "" || r.Header.Get("Cookie") != "" {
			t.Fatal("Apple credentials reached lyrics service")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"trackName":"Example","artistName":"Example Artist","albumName":"Example Album","duration":120,"syncedLyrics":"[00:01.00]Example line"}`))}, nil
	})
	doc, err := c.Lyrics(context.Background(), catalog.Subject{Source: "apple", ID: "i.saved", PlaybackID: "catalog-id"})
	if err != nil || len(doc.Lines) != 1 || calls != 1 {
		t.Fatal(doc, err)
	}
	if _, err = c.Lyrics(context.Background(), catalog.Subject{Source: "local", ID: "song"}); err == nil {
		t.Fatal("wrong source accepted")
	}
}
