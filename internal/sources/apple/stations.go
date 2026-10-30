package apple

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"lazymusic/internal/music"
)

// StationTracks advances the actual station ID. It never turns a personal or
// editorial station into the unrelated ra.<songID> song-seeded station.
func (c *Client) StationTracks(ctx context.Context, q music.Query) ([]music.Track, error) {
	if q.Source != music.Apple || q.Page != "station" || q.ID == "" || !q.Continuous {
		return nil, fmt.Errorf("invalid Apple Music station")
	}
	info, err := c.stationInfo(ctx, q.ID)
	if err != nil {
		return nil, err
	}
	if info.Format == "stream" {
		return []music.Track{stationStreamTrack(q.ID, info)}, nil
	}
	var tracks []music.Track
	seen := map[string]bool{}
	for call := 0; call < 3 && len(tracks) < 6; call++ {
		var response collection
		_, err := c.request(ctx, http.MethodPost, "/me/stations/next-tracks/"+url.PathEscape(q.ID), map[string]any{}, &response)
		if err != nil {
			return nil, err
		}
		if len(response.Data) == 0 {
			break
		}
		for _, item := range convert(response).Items {
			if item.Track != nil && !seen[item.ID] {
				seen[item.ID] = true
				tracks = append(tracks, *item.Track)
			}
		}
	}
	if len(tracks) == 0 {
		return nil, fmt.Errorf("Apple returned no playable tracks for this station")
	}
	return tracks, nil
}

func stationStreamTrack(id string, info stationInfo) music.Track {
	kind := "episode"
	if info.Live {
		kind = "live"
	}
	return music.Track{Source: music.Apple, Kind: "stations", MediaType: kind, ID: id, PlaybackID: id, Title: info.Title, Duration: time.Duration(info.Duration) * time.Millisecond}
}

// Browsing a station selection only resolves metadata. It never advances the
// personalized stream; finite episodes can be queued/picked like single items.
func (c *Client) stationSelection(ctx context.Context, q Query) (Page, error) {
	if q.ID == "" {
		return Page{}, fmt.Errorf("station requires an ID")
	}
	info, err := c.stationInfo(ctx, q.ID)
	if err != nil {
		return Page{}, err
	}
	if info.Format != "stream" {
		return Page{Continuous: true, Items: []Item{}}, nil
	}
	track := stationStreamTrack(q.ID, info)
	return Page{Items: []Item{{Source: music.Apple, Kind: "stations", ID: q.ID, Title: info.Title, Track: &track}}}, nil
}

type stationInfo struct {
	Title, Format string
	Live          bool
	Duration      int64
}

func (c *Client) stationInfo(ctx context.Context, id string) (stationInfo, error) {
	c.stationMu.Lock()
	defer c.stationMu.Unlock()
	if info, ok := c.stationCache[id]; ok {
		return info, nil
	}
	sf, err := c.sf(ctx)
	if err != nil {
		return stationInfo{}, err
	}
	var response struct {
		Data []struct {
			Attributes struct {
				Name     string `json:"name"`
				Live     bool   `json:"isLive"`
				Duration int64  `json:"durationInMillis"`
				Play     struct {
					Format string `json:"format"`
				} `json:"playParams"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err = c.get(ctx, "/catalog/"+url.PathEscape(sf)+"/stations/"+url.PathEscape(id), &response); err != nil {
		return stationInfo{}, err
	}
	if len(response.Data) == 0 {
		return stationInfo{}, fmt.Errorf("station is unavailable in your storefront")
	}
	a := response.Data[0].Attributes
	info := stationInfo{Title: a.Name, Format: a.Play.Format, Live: a.Live, Duration: a.Duration}
	if c.stationCache == nil || len(c.stationCache) >= 128 {
		c.stationCache = map[string]stationInfo{}
	}
	c.stationCache[id] = info
	return info, nil
}
