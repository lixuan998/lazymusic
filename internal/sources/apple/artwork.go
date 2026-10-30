package apple

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"lazymusic/internal/artwork"
	"lazymusic/internal/catalog"
	"lazymusic/internal/music"
)

func (c *Client) Artwork(ctx context.Context, subject catalog.Subject) (artwork.Image, error) {
	if subject.ID == "" || (subject.Source != "" && subject.Source != music.Apple) {
		return artwork.Image{}, fmt.Errorf("invalid artwork identity")
	}
	kind, id := subject.Kind, subject.ID
	if kind == "" {
		kind = "songs"
		if strings.HasPrefix(id, "i.") {
			kind = "library-songs"
		}
	}
	if strings.HasSuffix(kind, "songs") && subject.PlaybackID != "" {
		kind, id = "songs", subject.PlaybackID
	}
	switch kind {
	case "songs", "library-songs", "albums", "library-albums", "stations", "music-videos":
	default:
		return artwork.Image{}, nil
	}
	sf, err := c.sf(ctx)
	if err != nil {
		return artwork.Image{}, err
	}
	var response collection
	path := "/catalog/" + url.PathEscape(sf) + "/" + kind + "/" + url.PathEscape(id)
	if strings.HasPrefix(kind, "library-") {
		path = "/me/library/" + strings.TrimPrefix(kind, "library-") + "/" + url.PathEscape(id)
	}
	if err = c.get(ctx, path, &response); err != nil {
		return artwork.Image{}, err
	}
	if len(response.Data) == 0 || response.Data[0].Attributes.Artwork.URL == "" {
		return artwork.Image{}, nil
	}
	raw := response.Data[0].Attributes.Artwork.URL
	for key, value := range map[string]string{"{w}": "1200", "{h}": "1200", "{f}": "jpg", "{c}": "bb"} {
		raw = strings.ReplaceAll(raw, key, value)
	}
	return c.downloadArtwork(ctx, raw)
}

func appleArtworkURL(u *url.URL) bool {
	host := strings.ToLower(u.Hostname())
	return u.Scheme == "https" && u.User == nil && (u.Port() == "" || u.Port() == "443") &&
		(host == "mzstatic.com" || strings.HasSuffix(host, ".mzstatic.com"))
}

func (c *Client) downloadArtwork(ctx context.Context, raw string) (artwork.Image, error) {
	u, err := url.Parse(raw)
	if err != nil || !appleArtworkURL(u) {
		return artwork.Image{}, fmt.Errorf("unsupported album artwork host")
	}
	// Only the transport is shared. No API headers, cookies or redirect policy
	// cross into this unauthenticated CDN request.
	client := &http.Client{Transport: c.http.Transport, Timeout: c.http.Timeout, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !appleArtworkURL(r.URL) {
			return fmt.Errorf("unsupported artwork redirect")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return artwork.Image{}, fmt.Errorf("invalid album artwork request")
	}
	req.Header.Set("Accept", "image/jpeg, image/png")
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return artwork.Image{}, ctx.Err()
		}
		return artwork.Image{}, fmt.Errorf("album artwork download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return artwork.Image{}, fmt.Errorf("album artwork HTTP %d", resp.StatusCode)
	}
	return artwork.Decode(resp.Body)
}
