package license

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"lazymusic/internal/vibez/config"
)

func TestVideoAssetsUseExistingAccountAndKeepLicenseMetadata(t *testing.T) {
	c := NewClient(&config.Config{AppleDeveloperToken: "dev", AppleUserToken: "user"})
	calls := 0
	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "Bearer dev" || r.Header.Get("X-Apple-Music-User-Token") != "user" {
			t.Fatal("lost existing playback authorization")
		}
		if r.URL.Path == "/WebObjects/MZPlay.woa/wa/webPlayback" {
			var body map[string]string
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["salableAdamId"] != "movie" {
				t.Fatal("wrong playback identity")
			}
			return jsonResponse(`{"songList":[{"hls-playlist-url":"https://media.example/old.m3u8","hls-key-server-url":"https://play.example/license","widevine-cert-url":"https://play.example/cert"}]}`), nil
		}
		if r.URL.Path != "/v1/play/assets" || r.URL.Query().Get("kind") != "musicVideo" || r.URL.Query().Get("keyFormat") != "web" || r.URL.Query().Get("id") != "movie" {
			t.Fatal("wrong native video asset parameters")
		}
		return jsonResponse(`{"results":{"assets":[{"url":"https://media.example/video.m3u8"}]}}`), nil
	})
	info, err := c.VideoPlaybackInfo(context.Background(), "movie")
	if err != nil || calls != 2 || info.HLSPlaylistURL != "https://media.example/video.m3u8" || info.HLSKeyServerURL != "https://play.example/license" || info.WidevineCertURL != "https://play.example/cert" {
		t.Fatal("video assets lost license metadata", info, err)
	}
}
