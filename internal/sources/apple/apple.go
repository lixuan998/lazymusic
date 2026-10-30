// Package apple adapts Apple Music API resources into neutral catalog items.
package apple

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"lazymusic/internal/catalog"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
	"lazymusic/internal/vibez/config"
	"lazymusic/internal/vibez/provider"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Query = catalog.Query
type Page = catalog.Page
type Item = catalog.Item
type Client struct {
	stationMu      sync.Mutex
	stationCache   map[string]stationInfo
	editorialMu    sync.Mutex
	editorialCache map[string]editorialCache
	publicBase     string
	homeMu         sync.Mutex
	homeGroups     []homeResource
	homeAt         time.Time
	cfg            *config.Config
	http           *http.Client
	base           string
	mu             sync.Mutex
	storefront     string
}

func New(cfg *config.Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}, base: "https://api.music.apple.com/v1", publicBase: "https://music.apple.com", editorialCache: map[string]editorialCache{}, storefront: cfg.StoreFront}
}

type resource struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Attributes struct {
		Artwork struct {
			URL string `json:"url"`
		} `json:"artwork"`
		CanEdit    bool     `json:"canEdit"`
		Name       string   `json:"name"`
		Artist     string   `json:"artistName"`
		Album      string   `json:"albumName"`
		Curator    string   `json:"curatorName"`
		Duration   int64    `json:"durationInMillis"`
		TrackCount int      `json:"trackCount"`
		Composer   string   `json:"composerName"`
		Label      string   `json:"recordLabel"`
		Copyright  string   `json:"copyright"`
		Title      string   `json:"title"`
		Roles      []string `json:"roleNames"`
		PlayParams struct {
			CatalogID string `json:"catalogId"`
		} `json:"playParams"`
	} `json:"attributes"`
	Relationships map[string]collection `json:"relationships"`
}

type collection struct {
	Data []resource `json:"data"`
	Next string     `json:"next"`
}

func (c *Client) get(ctx context.Context, endpoint string, dst any) error {
	_, err := c.request(ctx, http.MethodGet, endpoint, nil, dst)
	return err
}

func (c *Client) request(ctx context.Context, method, endpoint string, body any, dst any) (int, error) {
	if c.cfg.AppleDeveloperToken == "" || c.cfg.AppleUserToken == "" {
		return 0, fmt.Errorf("not signed in; open Login to authorize Apple Music")
	}
	base, _ := url.Parse(c.base)
	var u *url.URL
	var err error
	if strings.HasPrefix(endpoint, "https://") || strings.HasPrefix(endpoint, "http://") {
		u, err = url.Parse(endpoint)
		if err != nil || u.Scheme != base.Scheme || !strings.EqualFold(u.Host, base.Host) {
			return 0, fmt.Errorf("refusing cross-origin pagination link")
		}
	} else {
		if !strings.HasPrefix(endpoint, "/") {
			return 0, fmt.Errorf("invalid API path")
		}
		endpoint = strings.TrimPrefix(endpoint, "/v1")
		u, err = url.Parse(c.base + endpoint)
		if err != nil {
			return 0, err
		}
	}
	var data []byte
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return 0, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.AppleDeveloperToken)
	req.Header.Set("Music-User-Token", c.cfg.AppleUserToken)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("Apple Music request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return resp.StatusCode, fmt.Errorf("Apple Music authorization failed or operation is forbidden (HTTP %d); check Login and permissions", resp.StatusCode)
	}
	if resp.StatusCode == 429 {
		return resp.StatusCode, fmt.Errorf("Apple Music rate limit; retry shortly")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("Apple Music HTTP %d", resp.StatusCode)
	}
	if dst == nil {
		return resp.StatusCode, nil
	}
	return resp.StatusCode, json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(dst)
}

func (c *Client) sf(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.storefront != "" {
		return c.storefront, nil
	}
	var r collection
	if err := c.get(ctx, "/me/storefront", &r); err != nil {
		return "", err
	}
	if len(r.Data) == 0 {
		return "", fmt.Errorf("Apple returned no storefront")
	}
	c.storefront = r.Data[0].ID
	return c.storefront, nil
}

func (c *Client) Browse(ctx context.Context, q Query) (Page, error) {
	if q.Source != "" && q.Source != music.Apple {
		return Page{}, fmt.Errorf("wrong source for Apple Music")
	}
	if q.Page == "station" {
		return c.stationSelection(ctx, q)
	}
	if q.Page == "home" || q.Page == "home-section" {
		return c.home(ctx, q)
	}
	if q.Page == "new" || q.Page == "radio" || q.Page == "editorial" {
		return c.editorial(ctx, q)
	}
	if q.Next != "" {
		if q.Page == "search" {
			var r struct {
				Results map[string]collection `json:"results"`
				Data    []resource            `json:"data"`
				Next    string                `json:"next"`
			}
			if err := c.get(ctx, q.Next, &r); err != nil {
				return Page{}, err
			}
			typeName := q.Section
			if q.Scope == "library" {
				typeName = "library-" + typeName
			}
			if found, ok := r.Results[typeName]; ok {
				return convert(found), nil
			}
			return convert(collection{Data: r.Data, Next: r.Next}), nil
		}
		var r collection
		if err := c.get(ctx, q.Next, &r); err != nil {
			return Page{}, err
		}
		return convert(r), nil
	}
	section := q.Section
	if section == "" {
		section = "songs"
	}
	if section != "songs" && section != "albums" && section != "artists" && section != "playlists" {
		return Page{}, fmt.Errorf("unknown section %q", section)
	}
	var endpoint string
	switch q.Page {
	case "library":
		endpoint = "/me/library/" + section + "?limit=50"
	case "playlists":
		endpoint = "/me/library/playlists?limit=50"
	case "search":
		if strings.TrimSpace(q.Term) == "" {
			return Page{Items: []Item{}}, nil
		}
		typeName := section
		if q.Scope == "library" {
			typeName = "library-" + section
			endpoint = "/me/library/search"
		} else {
			sf, err := c.sf(ctx)
			if err != nil {
				return Page{}, err
			}
			endpoint = "/catalog/" + url.PathEscape(sf) + "/search"
		}
		endpoint += "?term=" + url.QueryEscape(q.Term) + "&types=" + typeName + "&limit=25"
		var r struct {
			Results map[string]collection `json:"results"`
		}
		if err := c.get(ctx, endpoint, &r); err != nil {
			return Page{}, err
		}
		out := convert(r.Results[typeName])
		return out, nil
	case "detail":
		if q.ID == "" {
			return Page{}, fmt.Errorf("detail requires an ID")
		}
		kind := q.Kind
		if kind != "albums" && kind != "library-albums" && kind != "playlists" && kind != "library-playlists" && kind != "artists" && kind != "library-artists" {
			return Page{}, fmt.Errorf("unsupported collection kind %q", kind)
		}
		if strings.HasPrefix(kind, "library-") {
			endpoint = "/me/library/" + strings.TrimPrefix(kind, "library-") + "/" + url.PathEscape(q.ID)
		} else {
			sf, err := c.sf(ctx)
			if err != nil {
				return Page{}, err
			}
			endpoint = "/catalog/" + url.PathEscape(sf) + "/" + kind + "/" + url.PathEscape(q.ID)
		}
		if strings.HasSuffix(kind, "artists") {
			endpoint += "/albums?limit=50"
		} else {
			endpoint += "/tracks?limit=50"
		}
	default:
		return Page{}, fmt.Errorf("unknown page %q", q.Page)
	}
	var r collection
	if err := c.get(ctx, endpoint, &r); err != nil {
		return Page{}, err
	}
	return convert(r), nil
}

func convert(r collection) Page {
	out := Page{Next: r.Next, Items: make([]Item, 0, len(r.Data))}
	for _, v := range r.Data {
		a := v.Attributes
		i := Item{Source: music.Apple, ID: v.ID, Kind: v.Type, Title: provider.Clean(a.Name), Subtitle: provider.Clean(a.Artist)}
		i.Writable = v.Type == "library-playlists" && a.CanEdit
		if strings.HasSuffix(v.Type, "songs") {
			t := session.CleanTrack(music.Track{Source: music.Apple, ID: v.ID, PlaybackID: a.PlayParams.CatalogID, Title: i.Title, Artist: i.Subtitle, Album: provider.Clean(a.Album), Duration: time.Duration(a.Duration) * time.Millisecond})
			if !strings.HasPrefix(v.Type, "library-") {
				t.PlaybackID = v.ID
			}
			i.Track = &t
		} else if strings.HasSuffix(v.Type, "playlists") {
			i.Subtitle = provider.Clean(a.Curator)
			if a.TrackCount > 0 {
				i.Subtitle = strconv.Itoa(a.TrackCount) + " tracks"
			}
		}
		if strings.HasSuffix(v.Type, "albums") || strings.HasSuffix(v.Type, "playlists") || strings.HasSuffix(v.Type, "artists") {
			q := Query{Source: music.Apple, Page: "detail", Section: "songs", Kind: v.Type, ID: v.ID}
			i.Open = &q
			p := q
			i.Play = &p
		}
		if v.Type == "stations" {
			q := Query{Source: music.Apple, Page: "station", Kind: "stations", ID: v.ID, Term: i.Title, Continuous: true}
			i.Play = &q
		}
		out.Items = append(out.Items, i)
	}
	return out
}
