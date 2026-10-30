package license

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// VideoPlaybackInfo uses the account's existing playback authorization. Video
// manifests are returned separately from song audio assets by this endpoint.
func (c *Client) VideoPlaybackInfo(ctx context.Context, id string) (*PlaybackInfo, error) {
	body, _ := json.Marshal(map[string]string{"salableAdamId": id})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://play.itunes.apple.com/WebObjects/MZPlay.woa/wa/webPlayback", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("video playback metadata request failed")
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return nil, ErrUnauthorized
	}
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("video playback metadata HTTP %d", response.StatusCode)
	}
	var result struct {
		Items []struct {
			Playlist    string `json:"hls-playlist-url"`
			License     string `json:"hls-key-server-url"`
			Certificate string `json:"widevine-cert-url"`
		} `json:"songList"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("invalid video playback metadata")
	}
	if len(result.Items) == 0 || result.Items[0].Playlist == "" {
		return nil, ErrTrackUnavailable
	}
	item := result.Items[0]
	var assets struct {
		Results struct {
			Assets []struct {
				URL string `json:"url"`
			} `json:"assets"`
		} `json:"results"`
	}
	params := url.Values{"id": {id}, "kind": {"musicVideo"}, "keyFormat": {"web"}}
	if err = c.stationJSON(ctx, "/play/assets?"+params.Encode(), &assets); err != nil {
		return nil, err
	}
	if len(assets.Results.Assets) == 0 || assets.Results.Assets[0].URL == "" {
		return nil, ErrTrackUnavailable
	}
	item.Playlist = assets.Results.Assets[0].URL
	return &PlaybackInfo{SongID: id, HLSPlaylistURL: item.Playlist, HLSKeyServerURL: item.License, WidevineCertURL: item.Certificate}, nil
}
