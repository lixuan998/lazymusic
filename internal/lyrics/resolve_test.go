package lyrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"lazymusic/internal/music"
)

func TestUnavailablePrimaryUsesExactPublicFallback(t *testing.T) {
	resetGate()
	defer resetGate()
	var paths []string
	transport := transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Music-User-Token") != "" {
			t.Fatal("account materials leaked to public lyrics provider")
		}
		if r.URL.Host == "lrclib.net" {
			return nil, fmt.Errorf("TLS unavailable")
		}
		if r.URL.Scheme != "https" || r.URL.Host != "music.163.com" {
			t.Fatal("unexpected provider")
		}
		paths = append(paths, r.URL.Path)
		body := `{"code":200,"result":{"songs":[{"id":9,"name":"Example (Remix)","duration":186500,"artists":[{"name":"First Artist"},{"name":"Second Artist"}]},{"id":10,"name":"Example","duration":250000,"artists":[{"name":"First Artist"},{"name":"Second Artist"}]},{"id":11,"name":"Example","duration":186500,"artists":[{"name":"Other Artist"}]},{"id":12,"name":"Example","duration":186500,"artists":[{"name":"Second Artist"},{"name":"First Artist"}],"album":{"name":"Example"}}]}}`
		if r.URL.Path == "/api/song/lyric" {
			if r.URL.Query().Get("id") != "12" {
				t.Fatal("matched a remix, wrong artist or different length")
			}
			body = `{"code":200,"lrc":{"lyric":"[00:01.00]First example\n[00:02.00]Second example"}}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	doc, err := Resolve(context.Background(), music.Track{Title: "Example", Artist: "First Artist & Second Artist", Album: "Example - Single", Duration: 186 * time.Second}, transport)
	if err != nil || len(doc.Lines) != 2 || doc.Provider != "NetEase Music" || len(paths) != 2 {
		t.Fatal(doc, err, paths)
	}
}

func TestPublicFallbackKeepsDistinctRecordingsSeparate(t *testing.T) {
	for _, pair := range [][2]string{{"Calvin Harris & Ellie Goulding", "Ellie Goulding, Calvin Harris"}, {"First Artist feat. Second Artist", "Second Artist & First Artist"}} {
		if artistKey(pair[0]) != artistKey(pair[1]) {
			t.Fatal("collaboration punctuation changed identity")
		}
	}
	if artistKey("First Artist & Second Artist") == artistKey("First Artist") {
		t.Fatal("lost a named artist")
	}
}
