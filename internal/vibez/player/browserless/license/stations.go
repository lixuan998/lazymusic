package license

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// StationPlaybackInfo resolves Apple's authenticated station assets. The
// original play parameters are kept in memory and only sent back to Apple.
func (c *Client) StationPlaybackInfo(ctx context.Context, id string) (*PlaybackInfo, error) {
	sf := c.cfg.StoreFront
	if sf == "" {
		var response struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := c.stationJSON(ctx, "/me/storefront", &response); err != nil {
			return nil, err
		}
		if len(response.Data) == 0 {
			return nil, fmt.Errorf("Apple returned no storefront")
		}
		sf = response.Data[0].ID
	}
	var metadata struct {
		Data []struct {
			Attributes struct {
				Live bool                       `json:"isLive"`
				Play map[string]json.RawMessage `json:"playParams"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := c.stationJSON(ctx, "/catalog/"+url.PathEscape(sf)+"/stations/"+url.PathEscape(id), &metadata); err != nil {
		return nil, err
	}
	if len(metadata.Data) == 0 {
		return nil, fmt.Errorf("station unavailable in the current storefront")
	}
	params := url.Values{"keyFormat": {"web"}}
	for key, raw := range metadata.Data[0].Attributes.Play {
		var value any
		if json.Unmarshal(raw, &value) != nil {
			continue
		}
		switch v := value.(type) {
		case string:
			params.Set(key, v)
		case bool:
			params.Set(key, strconv.FormatBool(v))
		case float64:
			params.Set(key, strconv.FormatFloat(v, 'f', -1, 64))
		}
	}
	var response struct {
		Results struct {
			Assets []struct {
				URL         string `json:"url"`
				License     string `json:"keyServerUrl"`
				Certificate string `json:"widevineKeyCertificateUrl"`
			} `json:"assets"`
		} `json:"results"`
	}
	if err := c.stationJSON(ctx, "/play/assets?"+params.Encode(), &response); err != nil {
		return nil, err
	}
	if len(response.Results.Assets) == 0 || response.Results.Assets[0].URL == "" {
		return nil, fmt.Errorf("Apple returned no station stream")
	}
	a := response.Results.Assets[0]
	return &PlaybackInfo{SongID: id, HLSPlaylistURL: a.URL, HLSKeyServerURL: a.License, WidevineCertURL: a.Certificate}, nil
}

func (c *Client) stationJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.music.apple.com/v1"+path, nil)
	if err != nil {
		return err
	}
	c.setHeaders(req)
	req.Header.Set("Music-User-Token", c.cfg.AppleUserToken)
	client := *c.httpClient
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("station metadata request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Apple station media HTTP %d", resp.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out); err != nil {
		return fmt.Errorf("invalid station metadata")
	}
	return nil
}
