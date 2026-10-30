package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"lazymusic/internal/artwork"
	"lazymusic/internal/catalog"
	"lazymusic/internal/music"
)

type artworkAPI struct {
	fakeAPI
	contexts []context.Context
}

func (a *artworkAPI) Artwork(ctx context.Context, _ catalog.Subject) (artwork.Image, error) {
	a.contexts = append(a.contexts, ctx)
	return artwork.Image{Width: 1, Height: 1, RGB: []byte{240, 100, 180}}, nil
}

func TestTurntableArtworkFollowsPlayingTrackAndRejectsOldReplies(t *testing.T) {
	m, _ := testModel()
	a := &artworkAPI{}
	m.api = a
	m.UI.Visualizer = turntableView
	m.State.Playback.Track = &music.Track{Source: "demo", ID: "first"}
	first := m.syncArtwork()
	if first == nil || m.syncArtwork() != nil {
		t.Fatal("artwork missing or duplicated")
	}
	old := first().(artworkMsg)
	m.State.Playback.Track = &music.Track{Source: "demo", ID: "second"}
	second := m.syncArtwork()
	if second == nil || a.contexts[0].Err() == nil {
		t.Fatal("old artwork request not canceled")
	}
	m.acceptArtwork(old)
	if m.artwork.image.Valid() {
		t.Fatal("previous song artwork replaced the new cover")
	}
	m.acceptArtwork(second().(artworkMsg))
	if !m.artwork.image.Valid() || m.artwork.loading || m.syncArtwork() != nil {
		t.Fatal("artwork did not settle")
	}
	m.State.Playback.Track = &music.Track{Source: "demo", ID: "third"}
	pending := m.syncArtwork()
	m.UI.Layout = "queue"
	m.syncArtwork()
	m.acceptArtwork(pending().(artworkMsg))
	if m.artwork.image.Valid() || m.artwork.loading {
		t.Fatal("hidden cover accepted a late response")
	}
}

func TestHeaderAndComfortablePlayerFrame(t *testing.T) {
	m, _ := testModel()
	m.SetSize(120, 40)
	m.State.Source.Name = "APPLE MUSIC"
	m.State.Playback.Track = &music.Track{ID: "current", Title: "Track Title", Artist: "Artist Name", Album: "Album Name", Duration: 4 * time.Minute}
	m.State.Playback.Playing = true
	v := strings.Split(ansi.Strip(m.View().Content), "\n")
	if !strings.HasPrefix(v[0], "┌") || !strings.HasPrefix(v[1], "│") || !strings.HasSuffix(v[1], "│") || !strings.Contains(v[1], "LazyMusic") || !strings.Contains(v[1], "APPLE MUSIC") || !strings.HasPrefix(v[2], "└") {
		t.Fatal("top bar escaped its frame")
	}
	player := strings.Split(ansi.Strip(m.playerBox(120)), "\n")
	if len(player) != 8 || !strings.Contains(player[1], "Track Title") || !strings.Contains(player[2], "Artist Name") || !strings.Contains(player[2], "Album Name") || strings.Trim(player[3], "│ ") != "" || !strings.Contains(player[5], "▶ PLAYING") {
		t.Fatal("player metadata and controls are cramped or missing")
	}
}

func TestTurntableCoverMetadataAndResponsiveLayouts(t *testing.T) {
	for _, size := range [][2]int{{60, 16}, {80, 24}, {100, 30}, {160, 50}} {
		for _, logs := range []bool{false, true} {
			m, _ := testModel()
			m.SetSize(size[0], size[1])
			m.UI.Visualizer, m.UI.Focus, m.UI.ShowLogs = turntableView, 5, logs
			m.State.Playback.Track = &music.Track{Title: "中文歌曲", Album: "Album Name", Artist: "Artist Name"}
			m.artwork.image = artwork.Image{Width: 1, Height: 1, RGB: []byte{220, 60, 100}}
			lines := strings.Split(m.View().Content, "\n")
			if len(lines) != size[1] {
				t.Fatalf("size %v logs=%v: height %d", size, logs, len(lines))
			}
			for _, line := range lines {
				if ansi.StringWidth(line) != size[0] {
					t.Fatalf("size %v: width %d", size, ansi.StringWidth(line))
				}
			}
		}
	}
	m, _ := testModel()
	m.State.Playback.Track = &music.Track{Title: "Song", Album: "Album", Artist: "Artist"}
	rows := strings.Join(m.turntableRows(26, 20), "\n")
	for _, label := range []string{"ALBUM", "Album", "ARTIST", "Artist", "TRACK", "Song"} {
		if !strings.Contains(ansi.Strip(rows), label) {
			t.Fatal("missing turntable metadata", label)
		}
	}
	m.UI.Focus = 5
	m.normal(",")
	if m.UI.Visualizer != turntableView {
		t.Fatal("new view missing from cycle")
	}
	m.normal(".")
	if m.UI.Visualizer != 0 {
		t.Fatal("view cycle failed to wrap")
	}
}

func TestZenCyclesAllDistinctViewsAndKeepsEQControls(t *testing.T) {
	m, api := testModel()
	m.SetSize(140, 40)
	m.State.Source.Capabilities.Equalizer = true
	m.UI.Visualizer = 2
	m.normal("ctrl+-")
	if !m.UI.Zen || m.UI.Visualizer != 0 {
		t.Fatal("entering Zen did not select a visual view")
	}
	for _, want := range []int{1, turntableView, 0, 1, turntableView} {
		m.normal(".")
		if m.UI.Visualizer != want {
			t.Fatal("Zen repeats or omits a visualization")
		}
		v := ansi.Strip(m.View().Content)
		if !strings.Contains(v, "6 Equalizer") {
			t.Fatal("view change hid the equalizer")
		}
		if want == turntableView && !strings.Contains(v, "Turntable") {
			t.Fatal("Zen did not render the turntable")
		}
	}
	m.normal("l")
	cmd := m.normal("k")
	cmd()
	if api.lastCommand.Op != "equalizer" || api.lastCommand.EQBand != 1 {
		t.Fatal("turntable view consumed Zen EQ controls")
	}
	m.normal(",")
	if m.UI.Visualizer != 1 {
		t.Fatal("previous Zen view did not work")
	}
}

func TestZenTurntableDimensionsStayInsideViewport(t *testing.T) {
	for _, size := range [][2]int{{60, 16}, {100, 28}, {180, 50}} {
		for _, logs := range []bool{false, true} {
			m, _ := testModel()
			m.SetSize(size[0], size[1])
			m.UI.Zen, m.UI.Visualizer, m.UI.Focus, m.UI.ShowLogs = true, turntableView, 5, logs
			for _, eqW := range []int{20, size[0] / 2, size[0] - 20} {
				m.UI.ZenEQWidth = eqW
				lines := strings.Split(m.View().Content, "\n")
				if len(lines) != size[1] {
					t.Fatal("Zen photo exceeds terminal height")
				}
				for _, line := range lines {
					if ansi.StringWidth(line) > size[0] {
						t.Fatal("Zen photo exceeds terminal width")
					}
				}
				w, h := m.turntableSize()
				if h >= 7 {
					left, top, cols, rows := turntableImageRect(w, m.turntableGraphicHeight(h, w))
					if left < 0 || top < 0 || cols > w || rows > h {
						t.Fatal("photograph placement exceeds its panel")
					}
				}
			}
		}
	}
}
