//go:build darwin || linux || windows

package browserless

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"lazymusic/internal/vibez/player/browserless/mp4"
	"lazymusic/internal/vibez/provider"
)

func TestPreciseSeekUsesAACFramesWithinSegment(t *testing.T) {
	s := &StreamServer{streams: map[string]*trackStream{"x": {audioCfg: mp4.DefaultAudioConfig(), segments: []segmentRef{{duration: 6}, {duration: 6}, {duration: 6}}, totalDur: 18 * time.Second}}}
	uri, offset, ok := s.SeekStream("x", "http://127.0.0.1/stream/x/token.aac", 13500*time.Millisecond)
	if !ok || !strings.Contains(uri, "start_seg=2&skip_frames=64") {
		t.Fatalf("uri=%s offset=%s", uri, offset)
	}
	if offset > 13500*time.Millisecond || 13500*time.Millisecond-offset > 24*time.Millisecond {
		t.Fatalf("seek lost precision: %s", offset)
	}
}

func TestTrimADTSFramesRetainsFrameBoundaries(t *testing.T) {
	frame := []byte{0xff, 0xf1, 0x50, 0x80, 0x01, 0xbf, 0xfc, 0x21, 0x10, 0x04, 0x60, 0x8c, 0x1c}
	original := bytes.Repeat(frame, 4)
	out, left, err := trimADTSFrames(original, 3)
	if err != nil || left != 0 || !bytes.Equal(out, frame) {
		t.Fatalf("trim: %d %v", left, err)
	}
	if len(original) != 52 {
		t.Fatal("modified cached input")
	}
	if _, _, err = trimADTSFrames([]byte{0xff}, 1); err == nil {
		t.Fatal("accepted truncated frame")
	}
}

func TestRestoreQueueDoesNotStartPlaybackAndRemembersPosition(t *testing.T) {
	p := &Player{}
	p.RestoreTracks([]provider.Track{{ID: "one", Duration: time.Minute}}, 0, 17*time.Second)
	state, _ := p.GetState()
	if state.Playing || state.Loading || state.Position != 17*time.Second || p.resumePosition != 17*time.Second {
		t.Fatalf("restore=%+v", state)
	}
}

func TestQueueMoveDownChangesOrderAndPreservesCurrentSong(t *testing.T) {
	p := &Player{queue: []provider.Track{{ID: "a"}, {ID: "b"}, {ID: "c"}}, idx: 1}
	_ = p.MoveInQueue(0, 2)
	if p.queue[0].ID != "b" || p.queue[1].ID != "c" || p.queue[2].ID != "a" || p.idx != 0 {
		t.Fatalf("move across playing song: queue=%v current=%d", p.queue, p.idx)
	}
	_ = p.MoveInQueue(0, 1)
	if p.queue[1].ID != "b" || p.idx != 1 {
		t.Fatalf("move playing song: queue=%v current=%d", p.queue, p.idx)
	}
}

func TestInsertTracksDoesNotResetActivePlayback(t *testing.T) {
	p := &Player{queue: []provider.Track{{ID: "a"}, {ID: "b"}}, idx: 1, curStream: "active-stream", resumePosition: 17 * time.Second, resumeTrackID: "b"}
	p.state.Track = &p.queue[1]
	p.state.Playing = true
	p.state.Position = 17 * time.Second
	if err := p.InsertTracks([]provider.Track{{ID: "x"}, {ID: "y"}}, 0); err != nil {
		t.Fatal(err)
	}
	if p.idx != 3 || p.state.Track.ID != "b" || !p.state.Playing || p.state.Position != 17*time.Second || p.curStream != "active-stream" || p.resumePosition != 17*time.Second {
		t.Fatal("insertion restarted or interrupted the active stream")
	}
}

func TestAtomicQueueEditRetainsActiveNativeStream(t *testing.T) {
	p := &Player{queue: []provider.Track{{ID: "a"}, {ID: "b"}}, idx: 1, curStream: "active", curStreamURL: "loopback-active", seekOffset: 12 * time.Second}
	p.state.Track = &p.queue[1]
	p.state.Playing = true
	p.state.Position = 17 * time.Second
	err := p.EditTracks(func(q []provider.Track, index int) ([]provider.Track, int, bool, error) {
		return []provider.Track{q[1], q[0]}, 0, false, nil
	})
	if err != nil || !p.state.Playing || p.state.Position != 17*time.Second || p.curStream != "active" || p.curStreamURL != "loopback-active" || p.seekOffset != 12*time.Second || p.idx != 0 || p.state.Track.ID != "b" {
		t.Fatal("queue edit restarted the decoder or changed position")
	}
}
