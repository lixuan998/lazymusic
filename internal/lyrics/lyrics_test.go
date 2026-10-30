package lyrics

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"lazymusic/internal/music"
)

func TestLRCRepeatsTranslationsGapsAndBackwardSeek(t *testing.T) {
	d, err := ParseLRC("\ufeff[ar:Example]\n[00:01.00][00:03.000]First example\n[00:02.50]Second example\n[00:02.50]第二句示例\n[00:04.00]\n")
	if err != nil || len(d.Lines) != 4 || !d.Valid() {
		t.Fatal(d, err)
	}
	if d.Lines[1].Text != "Second example\n第二句示例" || d.Lines[3].Text != "" {
		t.Fatal("translations or instrumental gap lost")
	}
	for _, tc := range []struct {
		at   time.Duration
		want int
	}{{0, -1}, {time.Second, 0}, {2600 * time.Millisecond, 1}, {3500 * time.Millisecond, 2}, {time.Second, 0}} {
		if d.Active(tc.at) != tc.want {
			t.Fatal("lyric boundary or backwards seek failed", tc)
		}
	}
	for _, raw := range []string{strings.Repeat("x", MaxBytes+1), "[00:99.00]bad", strings.Repeat("[00:01.00]x\n", MaxLines+1)} {
		if _, err = ParseLRC(raw); err == nil {
			t.Fatal("accepted invalid or unbounded LRC")
		}
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func resetGate() {
	lookupGate.Lock()
	lookupGate.next, lookupGate.retryAfter = time.Time{}, time.Time{}
	lookupGate.Unlock()
}

func TestPublicLyricsExactMatchAndNoAccountMaterial(t *testing.T) {
	resetGate()
	defer resetGate()
	track := music.Track{Title: "Example", Artist: "Example Artist", Album: "Example Album", Duration: 120 * time.Second}
	calls := 0
	transport := transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Scheme != "https" || r.URL.Host != "lrclib.net" || r.URL.Path != "/api/get" || r.URL.Query().Get("track_name") != track.Title || r.URL.Query().Get("duration") != "120" {
			t.Fatal("invalid exact lookup")
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Music-User-Token") != "" || r.Header.Get("Cookie") != "" || !strings.Contains(r.UserAgent(), "lazymusic") {
			t.Fatal("account credentials or missing client identity")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"trackName":"Example","artistName":"Example Artist","albumName":"Example Album","duration":120,"syncedLyrics":"[00:01.00]Example line"}`))}, nil
	})
	d, err := Lookup(context.Background(), track, transport)
	if err != nil || len(d.Lines) != 1 || d.Provider != "LRCLIB" || calls != 1 {
		t.Fatal(d, err)
	}
	track.Duration = 130 * time.Second
	resetGate()
	d, err = Lookup(context.Background(), track, transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"trackName":"Example","artistName":"Example Artist","albumName":"Example Album","duration":120,"syncedLyrics":"[00:01.00]Another version"}`))}, nil
	}))
	if err != nil || len(d.Lines) != 0 {
		t.Fatal("used lyrics from another recording")
	}
}

func TestPublicLyricsHonorsRateLimitWithoutRetrying(t *testing.T) {
	resetGate()
	defer resetGate()
	calls := 0
	transport := transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"60"}}, Body: io.NopCloser(strings.NewReader("limited"))}, nil
	})
	track := music.Track{Title: "Example", Artist: "Example Artist", Duration: time.Minute}
	for range 2 {
		if _, err := Lookup(context.Background(), track, transport); err == nil {
			t.Fatal("throttle was ignored")
		}
	}
	if calls != 1 {
		t.Fatal("retried within Retry-After")
	}
}
