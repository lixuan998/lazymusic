package browserless

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestVideoVariantRequiresExplicitSoftwarePolicyAndMatchingAudioGroup(t *testing.T) {
	data := `#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="stereo",URI="audio.m3u8"
#EXT-X-STREAM-INF:CODECS="avc1.1,mp4a.40.2",HDCP-LEVEL=TYPE-0,ALLOWED-CPC="WIDEVINE_HARDWARE",RESOLUTION=1920x1080,AUDIO="stereo"
hardware.m3u8
#EXT-X-STREAM-INF:CODECS="avc1.1,mp4a.40.2",HDCP-LEVEL=NONE,ALLOWED-CPC="WIDEVINE_SOFTWARE",RESOLUTION=640x360,AUDIO="stereo"
software.m3u8
`
	v, a, err := selectVideoVariant([]byte(data), "https://media.example/master.m3u8")
	if err != nil || v != "https://media.example/software.m3u8" || a != "https://media.example/audio.m3u8" {
		t.Fatal("wrong rendition", v, a, err)
	}
	if _, _, err = selectVideoVariant([]byte(strings.ReplaceAll(data, "WIDEVINE_SOFTWARE", "WIDEVINE_HARDWARE")), "https://media.example/master.m3u8"); err == nil {
		t.Fatal("ignored hardware policy")
	}
}

func TestVideoVariantKeepsClearestSoftwareSourceWithinDisplayBudget(t *testing.T) {
	data := `#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="stereo",URI="audio.m3u8"
#EXT-X-STREAM-INF:CODECS="avc1.1,mp4a.40.2",HDCP-LEVEL=NONE,ALLOWED-CPC="WIDEVINE_SOFTWARE",RESOLUTION=640x290,AUDIO="stereo"
small.m3u8
#EXT-X-STREAM-INF:CODECS="avc1.1,mp4a.40.2",HDCP-LEVEL=NONE,ALLOWED-CPC="WIDEVINE_SOFTWARE",RESOLUTION=864x392,AUDIO="stereo"
original.m3u8
#EXT-X-STREAM-INF:CODECS="avc1.1,mp4a.40.2",HDCP-LEVEL=TYPE-0,ALLOWED-CPC="WIDEVINE_HARDWARE",RESOLUTION=1920x872,AUDIO="stereo"
protected.m3u8
`
	v, _, err := selectVideoVariant([]byte(data), "https://media.example/master.m3u8")
	if err != nil || v != "https://media.example/original.m3u8" {
		t.Fatal("discarded available picture detail or selected protected HD", v, err)
	}
}

func TestVideoPrefetchIsCanceledAndDiscardedOnDecoderReset(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer server.Close()
	s := &StreamServer{}
	s.videoContext, s.videoCancel = context.WithCancel(context.Background())
	defer s.videoCancel()
	defer s.closeMedia()
	p := s.prefetchVideo(&trackStream{segments: []segmentRef{{}, {uri: server.URL + "/next-encrypted-segment"}}}, 1)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("next segment was not fetched ahead of consumption")
	}
	s.videoMu.Lock()
	s.videoState.prefetch = p
	s.resetVideoLocked()
	s.videoMu.Unlock()
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("reset leaked a video download")
	}
	if !errors.Is(p.err, context.Canceled) || s.videoState.prefetch != nil {
		t.Fatal("canceled prefetch survived a source/seek reset", p.err)
	}
}
