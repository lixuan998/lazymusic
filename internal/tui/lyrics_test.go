package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"lazymusic/internal/catalog"
	"lazymusic/internal/lyrics"
	"lazymusic/internal/music"
)

type lyricsAPI struct {
	fakeAPI
	contexts []context.Context
}

func (a *lyricsAPI) Lyrics(ctx context.Context, _ catalog.Subject) (lyrics.Document, error) {
	a.contexts = append(a.contexts, ctx)
	return lyrics.Document{Lines: []lyrics.Line{{Start: time.Second, Text: "First example"}, {Start: 2 * time.Second, Text: "第二句示例"}, {Start: 3 * time.Second, Text: "Third example"}}}, nil
}

func TestLyricsFollowTrackAndRejectCanceledReplies(t *testing.T) {
	m, _ := testModel()
	a := &lyricsAPI{}
	m.api = a
	m.UI.Visualizer = turntableView
	m.State.Playback.Track = &music.Track{ID: "first"}
	first := m.syncLyrics()
	if first == nil || m.syncLyrics() != nil {
		t.Fatal("lyrics request missing or duplicated")
	}
	old := first().(lyricsMsg)
	m.State.Playback.Track = &music.Track{ID: "second"}
	second := m.syncLyrics()
	if second == nil || a.contexts[0].Err() == nil {
		t.Fatal("previous lyrics request was not canceled")
	}
	m.acceptLyrics(old)
	if len(m.lyrics.document.Lines) != 0 {
		t.Fatal("old song lyrics replaced the new song")
	}
	m.acceptLyrics(second().(lyricsMsg))
	if len(m.lyrics.document.Lines) != 3 || m.lyrics.loading {
		t.Fatal("new lyrics did not settle")
	}
	m.State.Playback.Track = &music.Track{ID: "third"}
	pending := m.syncLyrics()
	m.UI.Layout = "queue"
	m.syncLyrics()
	m.acceptLyrics(pending().(lyricsMsg))
	if len(m.lyrics.document.Lines) != 0 {
		t.Fatal("hidden panel accepted a late lyric response")
	}
}

func TestLyricsCenterCurrentLineScrollSeekPauseAndFallback(t *testing.T) {
	m, _ := testModel()
	m.UI.Visualizer = turntableView
	m.State.Playback.Track = &music.Track{ID: "song", Title: "Do not repeat this title"}
	m.lyrics.document = lyrics.Document{Lines: []lyrics.Line{{Start: time.Second, Text: "First example"}, {Start: 2 * time.Second, Text: "第二句示例"}, {Start: 3 * time.Second, Text: "Third example"}}}
	for _, tc := range []struct {
		position time.Duration
		text     string
	}{{time.Second, "First example"}, {2500 * time.Millisecond, "第二句示例"}, {3500 * time.Millisecond, "Third example"}, {time.Second, "First example"}} {
		m.State.Playback.Position = tc.position
		rows := m.lyricRows(42, 5)
		if !strings.Contains(ansi.Strip(rows[2]), tc.text) || !strings.Contains(rows[2], "\x1b[") {
			t.Fatal("current lyric is not centered and highlighted", tc)
		}
		for _, row := range rows {
			if ansi.StringWidth(row) != 42 {
				t.Fatal("lyrics escaped panel")
			}
		}
	}
	m.State.Playback.Playing = true
	if m.syncLyricsClock() == nil {
		t.Fatal("lyric clock did not start")
	}
	m.State.Playback.Playing = false
	if m.syncLyricsClock() != nil || m.lyrics.ticking {
		t.Fatal("pause did not stop lyric updates")
	}
	m.State.Playback.Status = "stopped"
	rows := strings.Join(m.turntableRows(42, 20), "\n")
	if !strings.Contains(ansi.Strip(rows), "First example") || strings.Contains(rows, "Do not repeat") {
		t.Fatal("stop discarded available lyrics or repeated song metadata")
	}
	m.State.Playback.Status = "paused"
	m.lyrics.document = lyrics.Document{}
	if !strings.Contains(ansi.Strip(strings.Join(m.lyricRows(42, 5), "\n")), "No Lyrics Available") {
		t.Fatal("missing lyrics have no fallback")
	}
}

func TestPlayerProgressUsesASolidHalfHeightBar(t *testing.T) {
	m, _ := testModel()
	m.SetSize(100, 30)
	m.State.Playback.Track = &music.Track{ID: "song", Duration: time.Minute}
	m.State.Playback.Position = 30 * time.Second
	view := ansi.Strip(m.playerBox(100))
	if strings.Count(view, "▄") < 50 {
		t.Fatal("player retained its thin progress line")
	}
}

func TestLyricsTemporaryFailuresRetryWithoutHammeringProvider(t *testing.T) {
	m, _ := testModel()
	m.api = &lyricsAPI{}
	m.UI.Visualizer = turntableView
	m.State.Playback.Track = &music.Track{ID: "song"}
	first := m.syncLyrics()().(lyricsMsg)
	first.err = errors.New("temporary network failure")
	m.acceptLyrics(first)
	if m.lyrics.loaded || !m.lyrics.retryAt.After(time.Now()) || m.syncLyrics() != nil {
		t.Fatal("temporary failure became permanent or retried immediately")
	}
	m.lyrics.retryAt = time.Now().Add(-time.Second)
	retry := m.syncLyrics()
	if retry == nil {
		t.Fatal("lyrics never retried after backoff")
	}
	m.acceptLyrics(retry().(lyricsMsg))
	if !m.lyrics.loaded || len(m.lyrics.document.Lines) == 0 {
		t.Fatal("successful retry did not restore lyrics")
	}
}
