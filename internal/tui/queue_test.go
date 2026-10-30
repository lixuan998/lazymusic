package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"lazymusic/internal/music"
)

func queueFixture(count int) []music.Track {
	tracks := make([]music.Track, count)
	for i := range tracks {
		// Repeated identities must still focus the actual queue occurrence.
		tracks[i] = music.Track{ID: "repeated", Title: fmt.Sprintf("Song %03d", i)}
	}
	return tracks
}

func TestQueueFocusCentersCurrentOccurrenceAndClampsAtEnds(t *testing.T) {
	for _, tc := range []struct {
		name                                      string
		count, index, height, player, offset, row int
		logs, paused                              bool
	}{
		{"middle", 100, 50, 26, 0, 45, 5, false, false},
		{"paused", 100, 50, 26, 0, 45, 5, false, true},
		{"even-height", 100, 50, 27, 0, 44, 6, false, false},
		{"near-start", 100, 2, 26, 0, 0, 2, false, false},
		{"near-end", 100, 98, 26, 0, 89, 9, false, false},
		{"last", 100, 99, 26, 0, 89, 10, false, false},
		{"short", 7, 4, 26, 0, 0, 4, false, false},
		{"single", 1, 0, 26, 0, 0, 0, false, false},
		{"logs", 100, 50, 26, 0, 49, 1, true, false},
		{"taller-player", 100, 50, 26, 8, 46, 4, false, false},
	} {
		for _, layout := range []string{"full", "queue"} {
			t.Run(tc.name+"/"+layout, func(t *testing.T) {
				m, api := testModel()
				m.height = tc.height
				m.UI.Focus, m.UI.Layout, m.UI.ShowLogs, m.UI.PlayerHeight = 2, layout, tc.logs, tc.player
				m.State.Queue, m.State.Index = queueFixture(tc.count), tc.index
				m.State.Playback.Track = &m.State.Queue[tc.index]
				m.State.Playback.Playing = !tc.paused
				m.UI.QueueCursor = tc.count - 1
				m.Update(key("f"))
				if m.UI.QueueCursor != tc.index || m.UI.QueueOffset != tc.offset || m.UI.Focus != 2 {
					t.Fatalf("focus cursor=%d offset=%d", m.UI.QueueCursor, m.UI.QueueOffset)
				}
				for i := 0; i < 2; i++ {
					rows := m.queueRows(m.bodyHeight() - 2)
					if !strings.Contains(ansi.Strip(rows[tc.row]), m.State.Queue[tc.index].Title) {
						t.Fatalf("current song is not on expected visible row %d", tc.row)
					}
				}
				if api.lastCommand.Op != "" || m.State.Index != tc.index || m.State.Playback.Playing == tc.paused {
					t.Fatal("focus changed playback")
				}
			})
		}
	}
}

func TestQueueFocusKeepsViewportWhileBrowsingAndPolling(t *testing.T) {
	m, _ := testModel()
	m.height, m.UI.Focus = 26, 2
	m.State.Queue, m.State.Index = queueFixture(100), 50
	m.normal("f")
	m.normal("j")
	if m.UI.QueueCursor != 51 || m.UI.QueueOffset != 45 {
		t.Fatal("ordinary movement discarded the centered viewport")
	}
	for i := 0; i < 7; i++ {
		m.normal("j")
	}
	if m.UI.QueueCursor != 58 || m.UI.QueueOffset != 48 {
		t.Fatal("cursor did not scroll into view at the lower edge")
	}
	next := m.State
	next.Index, next.ObservedAt = 51, time.Now()
	m.Update(stateMsg{s: next})
	if m.UI.QueueCursor != 58 || m.UI.QueueOffset != 48 {
		t.Fatal("playback polling stole the queue selection")
	}
	m.normal("f")
	if m.UI.QueueCursor != 51 || m.UI.QueueOffset != 46 {
		t.Fatal("focus did not use the latest playing index")
	}
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 36})
	m.normal("f")
	if m.UI.QueueCursor != 51 || m.UI.QueueOffset != 42 {
		t.Fatal("focus ignored the resized queue viewport")
	}
	next.Queue, next.ObservedAt = next.Queue[:3], time.Now()
	m.Update(commandMsg{s: next})
	if m.UI.QueueCursor != 2 || m.UI.QueueOffset != 0 {
		t.Fatal("queue shrink left the selection or viewport out of bounds")
	}
}

func TestQueueFocusIsScopedRemappableAndSafeWithoutCurrentSong(t *testing.T) {
	for _, focus := range []int{0, 1, 3, 4, 5} {
		m, _ := testModel()
		m.UI.Focus, m.UI.QueueCursor = focus, 2
		m.State.Queue, m.State.Index = queueFixture(8), 5
		if m.normal("f") != nil || m.UI.QueueCursor != 2 || strings.Contains(strings.Join(m.helpLines(), "\n"), "Focus current song") {
			t.Fatal("queue f leaked into another panel")
		}
	}
	for _, index := range []int{-1, 8} {
		m, _ := testModel()
		m.UI.Focus, m.UI.QueueCursor = 2, 2
		m.State.Queue, m.State.Index = queueFixture(8), index
		if m.normal("f") != nil || m.UI.QueueCursor != 2 {
			t.Fatal("invalid playing index changed selection")
		}
	}
	m, _ := testModel()
	m.UI.Focus = 2
	if m.normal("f") != nil {
		t.Fatal("empty queue focus was not a no-op")
	}
	m.State.Queue, m.State.Index = queueFixture(100), 50
	m.keys["queue_focus_playing"] = []string{"z"}
	if m.normal("f") != nil || m.UI.QueueCursor != 0 {
		t.Fatal("removed default binding still focused the queue")
	}
	m.normal("z")
	if m.UI.QueueCursor != 50 || !strings.Contains(strings.Join(m.helpLines(), "\n"), "z  Focus current song") {
		t.Fatal("custom binding or Queue help was not updated")
	}
}

func TestQueueCenteredViewportSurvivesSessionRestore(t *testing.T) {
	m, api := testModel()
	m.height, m.UI.Focus = 26, 2
	m.State.Queue, m.State.Index = queueFixture(100), 50
	m.normal("f")
	m.persist()()
	next := m.State
	next.UI = api.saved
	restored := New(context.Background(), api, next, nil)
	restored.SetSize(100, 26)
	rows := restored.queueRows(restored.bodyHeight() - 2)
	if restored.UI.QueueCursor != 50 || restored.UI.QueueOffset != 45 || !strings.Contains(ansi.Strip(rows[5]), "Song 050") {
		t.Fatal("session restore lost the centered queue view")
	}
}
