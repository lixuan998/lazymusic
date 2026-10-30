package apple

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"lazymusic/internal/catalog"
	"lazymusic/internal/lyrics"
	"lazymusic/internal/music"
)

func (c *Client) Lyrics(ctx context.Context, subject catalog.Subject) (lyrics.Document, error) {
	if subject.ID == "" || subject.Source != "" && subject.Source != music.Apple {
		return lyrics.Document{}, fmt.Errorf("invalid lyrics identity")
	}
	kind, id := subject.Kind, subject.ID
	if kind == "" {
		kind = "songs"
		if strings.HasPrefix(id, "i.") {
			kind = "library-songs"
		}
	}
	if (kind == "songs" || kind == "library-songs") && subject.PlaybackID != "" {
		kind, id = "songs", subject.PlaybackID
	}
	if kind != "songs" && kind != "library-songs" {
		return lyrics.Document{}, nil
	}
	sf, err := c.sf(ctx)
	if err != nil {
		return lyrics.Document{}, err
	}
	path := "/catalog/" + url.PathEscape(sf) + "/songs/" + url.PathEscape(id)
	if kind == "library-songs" {
		path = "/me/library/songs/" + url.PathEscape(id)
	}
	var response collection
	if err = c.get(ctx, path, &response); err != nil {
		return lyrics.Document{}, err
	}
	if len(response.Data) == 0 {
		return lyrics.Document{}, nil
	}
	a := response.Data[0].Attributes
	return lyrics.Resolve(ctx, music.Track{Title: a.Name, Artist: a.Artist, Album: a.Album, Duration: time.Duration(a.Duration) * time.Millisecond}, c.http.Transport)
}
