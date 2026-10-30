package license

import (
	"context"
	"net/http"
	"testing"

	"lazymusic/internal/vibez/config"
)

func TestStationAssetsUseAccountStorefrontAndOriginalPlayParameters(t *testing.T) {
	c := NewClient(&config.Config{AppleDeveloperToken: "dev", AppleUserToken: "user"})
	calls := 0
	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Music-User-Token") != "user" || r.Header.Get("X-Apple-Music-User-Token") != "user" || r.Header.Get("Authorization") != "Bearer dev" {
			t.Fatal("missing station authorization")
		}
		switch r.URL.Path {
		case "/v1/me/storefront":
			return jsonResponse(`{"data":[{"id":"cn"}]}`), nil
		case "/v1/catalog/cn/stations/ra.episode":
			return jsonResponse(`{"data":[{"attributes":{"isLive":false,"playParams":{"id":"ra.episode","kind":"radioStation","format":"stream","hasDrm":true,"stationHash":"opaque"}}}]}`), nil
		case "/v1/play/assets":
			q := r.URL.Query()
			if q.Get("id") != "ra.episode" || q.Get("hasDrm") != "true" || q.Get("stationHash") != "opaque" || q.Get("keyFormat") != "web" {
				t.Fatal("lost station play parameters")
			}
			return jsonResponse(`{"results":{"assets":[{"url":"https://audio.example/station.m3u8","keyServerUrl":"https://play.example/license","widevineKeyCertificateUrl":"https://play.example/cert"}]}}`), nil
		default:
			t.Fatalf("unexpected station route %s", r.URL.Path)
			return nil, nil
		}
	})
	info, err := c.StationPlaybackInfo(context.Background(), "ra.episode")
	if err != nil || info.SongID != "ra.episode" || info.HLSPlaylistURL != "https://audio.example/station.m3u8" || calls != 3 {
		t.Fatalf("station assets: %+v %v", info, err)
	}
}
