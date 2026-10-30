package apple

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"lazymusic/internal/catalog"
	"lazymusic/internal/music"
	"lazymusic/internal/vibez/provider"
)

// New/Radio use Apple's public, server-rendered editorial content. Only the
// serialized content payload is parsed: no web token, cookies or scripts are
// extracted or executed. Account authorization is never sent to the web origin.
var editorialScript = regexp.MustCompile(`(?is)<script\b[^>]*\bid=["']serialized-server-data["'][^>]*>(.*?)</script\s*>`)

type editorialCache struct {
	at     time.Time
	groups []editorialGroup
}
type editorialGroup struct {
	id, title string
	items     []Item
}
type webDescriptor struct {
	Kind        string `json:"kind"`
	URL         string `json:"url"`
	Identifiers struct {
		ID string `json:"storeAdamID"`
	} `json:"identifiers"`
}
type webSegue struct {
	Destination struct {
		Content webDescriptor `json:"contentDescriptor"`
		Intent  struct {
			ID   string `json:"id"`
			Kind string `json:"$kind"`
		} `json:"intent"`
	} `json:"destination"`
}
type webLink struct {
	Title string   `json:"title"`
	URL   string   `json:"url"`
	Segue webSegue `json:"segue"`
}
type webItem struct {
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	Subtitle      string        `json:"subtitle"`
	TitleLinks    []webLink     `json:"titleLinks"`
	SubtitleLinks []webLink     `json:"subtitleLinks"`
	Content       webDescriptor `json:"contentDescriptor"`
	Segue         webSegue      `json:"segue"`
}
type webSection struct {
	ID     string    `json:"id"`
	Kind   string    `json:"itemKind"`
	Items  []webItem `json:"items"`
	Header struct {
		Item struct {
			Title string  `json:"title"`
			Link  webLink `json:"titleLink"`
		} `json:"item"`
	} `json:"header"`
}

func playbackNote(kind string) string {
	switch kind {
	case "stations", "radioStation":
		return "Station playback is not connected to this native player yet"
	case "music-videos", "musicVideo", "artistUploadedVideo":
		return "Video playback is not supported by this audio player"
	default:
		return "Playback is not available for this resource type yet"
	}
}

func (c *Client) editorial(ctx context.Context, q Query) (Page, error) {
	sf, err := c.sf(ctx)
	if err != nil {
		return Page{}, err
	}
	pagePath := "/" + sf + "/" + q.Page
	if q.Page == "editorial" {
		pagePath = q.Term
	}
	if pagePath, err = editorialPath(pagePath, sf); err != nil {
		return Page{}, err
	}
	groups, err := c.editorialGroups(ctx, sf, pagePath, q.Refresh)
	if err != nil {
		return Page{}, err
	}
	out := Page{Items: []Item{}, Tabs: []catalog.Tab{}}
	if len(groups) == 0 {
		return out, nil
	}
	selected := 0
	for i, g := range groups {
		query := q
		query.ID = g.id
		query.Next = ""
		query.Refresh = false
		out.Tabs = append(out.Tabs, catalog.Tab{ID: g.id, Title: g.title, Query: query})
		if g.id == q.ID {
			selected = i
		}
	}
	g := groups[selected]
	out.ActiveTab = g.id
	start := 0
	if q.Next != "" {
		start, err = strconv.Atoi(q.Next)
		if err != nil || start < 0 || start > len(g.items) {
			return Page{}, fmt.Errorf("invalid editorial page cursor")
		}
	}
	end := min(len(g.items), start+50)
	out.Items = append(out.Items, g.items[start:end]...)
	if end < len(g.items) {
		out.Next = strconv.Itoa(end)
	}
	return out, nil
}

func editorialPath(raw, sf string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
		return "", fmt.Errorf("invalid Apple editorial link")
	}
	if (u.Scheme != "" || u.Host != "") && (u.Host != "music.apple.com" || u.Scheme != "https") {
		return "", fmt.Errorf("refusing external editorial link")
	}
	p := path.Clean(u.Path)
	if strings.ContainsAny(p, "?#\r\n") || !strings.HasPrefix(p, "/"+sf+"/") || p != u.Path {
		return "", fmt.Errorf("editorial link does not match the account storefront")
	}
	return p, nil
}

func (c *Client) editorialGroups(ctx context.Context, sf, pagePath string, refresh bool) ([]editorialGroup, error) {
	c.editorialMu.Lock()
	defer c.editorialMu.Unlock()
	if cached, ok := c.editorialCache[pagePath]; ok && !refresh && time.Since(cached.at) < 5*time.Minute {
		return cached.groups, nil
	}
	base, err := url.Parse(c.publicBase)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.publicBase+pagePath, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", "geo="+strings.ToUpper(sf))
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || req.URL.Scheme != base.Scheme || req.URL.Host != base.Host {
			return fmt.Errorf("refusing external editorial redirect")
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not load Apple editorial content: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Apple editorial content HTTP %d", resp.StatusCode)
	}
	if resp.Request.URL.Path != pagePath {
		return nil, fmt.Errorf("Apple redirected this editorial page; it may be unavailable in your storefront")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 16<<20 {
		return nil, fmt.Errorf("Apple editorial page exceeds size limit")
	}
	groups, err := parseEditorial(data, sf, pagePath)
	if err != nil {
		return nil, err
	}
	if c.editorialCache == nil {
		c.editorialCache = map[string]editorialCache{}
	}
	if len(c.editorialCache) >= 32 {
		oldest := ""
		var at time.Time
		for key, entry := range c.editorialCache {
			if oldest == "" || entry.at.Before(at) {
				oldest, at = key, entry.at
			}
		}
		delete(c.editorialCache, oldest)
	}
	c.editorialCache[pagePath] = editorialCache{time.Now(), groups}
	return groups, nil
}

func parseEditorial(html []byte, sf, pagePath string) ([]editorialGroup, error) {
	match := editorialScript.FindSubmatch(html)
	if len(match) != 2 {
		return nil, fmt.Errorf("Apple editorial page format changed; no structured content found")
	}
	var payload struct {
		Data []struct {
			Data struct {
				Sections []webSection `json:"sections"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(match[1], &payload); err != nil {
		return nil, fmt.Errorf("could not read Apple editorial content")
	}
	var groups []editorialGroup
	for _, page := range payload.Data {
		for _, section := range page.Data.Sections {
			title := provider.Clean(section.Header.Item.Link.Title)
			if title == "" {
				title = provider.Clean(section.Header.Item.Title)
			}
			if title == "" {
				if len(groups) == 0 {
					title = "Featured"
				} else {
					title = fmt.Sprintf("Section %d", len(groups)+1)
				}
			}
			id := section.ID
			if id == "" {
				id = fmt.Sprintf("section-%d", len(groups))
			}
			g := editorialGroup{id: id, title: title, items: []Item{}}
			for i, entry := range section.Items {
				g.items = append(g.items, editorialItem(entry, section.Kind, sf, title, id, i))
			}
			if more := webLinkPath(section.Header.Item.Link, sf); more != "" && more != pagePath {
				q := Query{Source: music.Apple, Page: "editorial", Section: "songs", Term: more}
				g.items = append(g.items, Item{Source: music.Apple, ID: "more:" + more, Kind: "editorial-sections", Title: "View all · " + title, Group: title, Open: &q})
			}
			groups = append(groups, g)
		}
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("Apple editorial page contains no readable sections")
	}
	return groups, nil
}

func webLinkPath(link webLink, sf string) string {
	if link.URL != "" {
		if p, err := editorialPath(link.URL, sf); err == nil {
			return p
		}
	}
	intent := link.Segue.Destination.Intent
	if intent.ID != "" {
		var kind string
		switch intent.Kind {
		case "RoomPageIntent":
			kind = "room"
		case "MultiRoomPageIntent":
			kind = "multiroom"
		}
		if kind != "" {
			p, err := editorialPath("/"+sf+"/"+kind+"/"+url.PathEscape(intent.ID), sf)
			if err == nil {
				return p
			}
		}
	}
	return ""
}

func editorialItem(entry webItem, fallback, sf, group, sectionID string, index int) Item {
	d := entry.Content
	if d.Kind == "" {
		d = entry.Segue.Destination.Content
	}
	kind := d.Kind
	if kind == "" {
		kind = fallback
	}
	id := d.Identifiers.ID
	title := entry.Title
	if title == "" && len(entry.TitleLinks) > 0 {
		title = entry.TitleLinks[0].Title
	}
	subtitle := entry.Subtitle
	if subtitle == "" {
		var labels []string
		for _, link := range entry.SubtitleLinks {
			labels = append(labels, link.Title)
		}
		subtitle = strings.Join(labels, " · ")
	}
	resourceKind := map[string]string{"song": "songs", "album": "albums", "playlist": "playlists", "artist": "artists", "radioStation": "stations", "musicVideo": "music-videos"}[kind]
	var item Item
	if id != "" && resourceKind != "" {
		r := resource{ID: id, Type: resourceKind}
		r.Attributes.Name = title
		r.Attributes.Artist = subtitle
		r.Attributes.Curator = subtitle
		item = convert(collection{Data: []resource{r}}).Items[0]
	} else {
		id = entry.ID
		if id == "" {
			id = fmt.Sprintf("%s-item-%d", sectionID, index)
		}
		item = Item{Source: music.Apple, ID: id, Kind: kind, Title: title, Subtitle: subtitle}
	}
	item.Group = group
	if item.Title == "" {
		item.Title = "Untitled " + kind
	}
	if link := webLinkPath(webLink{Segue: entry.Segue}, sf); link != "" {
		q := Query{Source: music.Apple, Page: "editorial", Section: "songs", Term: link}
		item.Open = &q
	}
	if item.Track == nil && item.Play == nil && item.Open == nil {
		item.PlaybackNote = playbackNote(item.Kind)
		if item.Subtitle == "" {
			item.Subtitle = item.PlaybackNote
		}
	}
	return item
}
