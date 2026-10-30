package engine

import (
	"testing"
	"time"

	"lazymusic/internal/music"
	"lazymusic/internal/session"
	"lazymusic/internal/vibez/player/browserless"
	"lazymusic/internal/vibez/provider"
)

func TestRemovingCurrentSongCannotResumeRemovedStream(t *testing.T) {
	n := &Native{p: &browserless.Player{}}
	n.p.RestoreTracks([]provider.Track{{ID: "one"}, {ID: "two"}}, 0, 17*time.Second)
	if err := n.Remove(0); err != nil {
		t.Fatal(err)
	}
	s, q, i := n.State()
	if s.Playing || s.Loading || s.Position != 0 || s.Track == nil || s.Track.ID != "two" || len(q) != 1 || i != 0 {
		t.Fatalf("removed song remains active: %+v queue=%v index=%d", s, q, i)
	}
	_ = n.Remove(0)
	if err := n.Play(); err == nil {
		t.Fatal("empty queue entered playing state")
	}
}

func TestQualityReloadPreservesPausedQueueAndPosition(t *testing.T) {
	n := &Native{p: &browserless.Player{}}
	n.p.RestoreTracks([]provider.Track{{ID: "one", Duration: time.Minute}}, 0, 17*time.Second)
	if err := n.SetQuality("standard"); err != nil {
		t.Fatal(err)
	}
	s, q, i := n.State()
	if s.Bitrate != 64 || s.Position != 17*time.Second || s.Playing || len(q) != 1 || i != 0 {
		t.Fatal("quality switch lost playback state", s)
	}
	if err := n.SetQuality("lossless"); err == nil {
		t.Fatal("unsupported bitrate accepted")
	}
}

func TestRadioEpisodeMetadataSurvivesNativeConversionAndRestore(t *testing.T) {
	n := &Native{p: &browserless.Player{}}
	track := music.Track{Source: music.Apple, ID: "ra.episode", PlaybackID: "ra.episode", Kind: "stations", MediaType: "episode", Title: "Radio episode", Duration: time.Hour}
	saved := session.Playback{Queue: []music.Track{track}, Position: 19 * time.Second}
	n.p.RestoreTracks(toNativeTracks(saved.Queue), saved.Index, saved.Position)
	s, q, _ := n.State()
	if s.Playing || s.Track == nil || s.Track.Kind != "stations" || s.Track.MediaType != "episode" || q[0].MediaType != "episode" || s.Position != 19*time.Second {
		t.Fatal("episode lost its media identity")
	}
}
