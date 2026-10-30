//go:build darwin || linux

package browserless

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStationMasterVariantAndWidevinePlaylist(t *testing.T) {
	master := []byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=64000,CODECS=\"mp4a.40.5\"\nlow.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=256000,CODECS=\"mp4a.40.2\"\nhigh.m3u8\n")
	for _, tc := range []struct {
		high bool
		want string
	}{{false, "low.m3u8"}, {true, "high.m3u8"}} {
		uri, err := selectStationVariant(master, "https://audio.example/path/master.m3u8", tc.high)
		if err != nil || !strings.HasSuffix(uri, tc.want) {
			t.Fatalf("variant %s %v", uri, err)
		}
	}
	data := []byte("#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"skd://ignored\",KEYFORMAT=\"com.apple.streamingkeydelivery\"\n#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"data:;base64,synthetic\",KEYFORMAT=\"urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed\"\n#EXT-X-MEDIA-SEQUENCE:50\n#EXTINF:6,\nsegment.mp4\n#EXT-X-ENDLIST\n")
	m, err := parseStationManifest(data, "https://audio.example/path/media.m3u8")
	if err != nil || m.live || m.sequence != 50 || len(m.segments) != 1 || m.key != "data:;base64,synthetic" || m.init != "https://audio.example/path/init.mp4" {
		t.Fatalf("station manifest: %+v %v", m, err)
	}
}

func TestStationKeyRotationIsAttachedToEachSegment(t *testing.T) {
	data := []byte("#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"data:;base64,first\",KEYFORMAT=\"urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed\"\n#EXTINF:6,\na.mp4\n#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"data:;base64,second\",KEYFORMAT=\"urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed\"\n#EXTINF:6,\nb.mp4\n#EXT-X-ENDLIST\n")
	m, err := parseStationManifest(data, "https://audio.example/media.m3u8")
	if err != nil || len(m.segments) != 2 || m.key != "data:;base64,first" || m.segments[0].keyURI == m.segments[1].keyURI {
		t.Fatal("a new window key replaced the protection for older segments")
	}
}

func TestLiveWindowContinuesWithoutRepeatingOverlappingSegments(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/media.m3u8" {
			fmt.Fprint(w, "#EXTM3U\n#EXT-X-MAP:URI=\"init\"\n#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"data:;base64,key\",KEYFORMAT=\"urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed\"\n#EXT-X-MEDIA-SEQUENCE:51\n#EXTINF:1,\nB\n#EXTINF:1,\nC\n#EXTINF:1,\nD\n#EXT-X-ENDLIST\n")
			return
		}
		fmt.Fprint(w, strings.TrimPrefix(r.URL.Path, "/"))
	}))
	defer server.Close()
	initial := &trackStream{trackID: "station", raw: true, live: true, mediaSequence: 50, stationURI: server.URL + "/media.m3u8", keyURI: "data:;base64,key", initURI: server.URL + "/init", segments: []segmentRef{{uri: server.URL + "/A"}, {uri: server.URL + "/B"}, {uri: server.URL + "/C"}}}
	s := &StreamServer{}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, "GET", "http://127.0.0.1/stream", nil)
	w := httptest.NewRecorder()
	s.serveLive(w, req, "stream", initial)
	if got := w.Body.String(); got != "ABCD" {
		t.Fatalf("live window replayed or dropped content: %q", got)
	}
}
