package apple

import (
	"context"
	"encoding/json"
	"fmt"
	"lazymusic/internal/catalog"
	"lazymusic/internal/music"
	"lazymusic/internal/vibez/provider"
	"net/url"
	"strings"
	"time"
)

func (c *Client) recommendationGroups(ctx context.Context, refresh bool) ([]homeResource, error) {
	c.homeMu.Lock()
	defer c.homeMu.Unlock()
	if !refresh && len(c.homeGroups) > 0 && time.Since(c.homeAt) < 5*time.Minute {
		return c.homeGroups, nil
	}
	endpoint := "/me/recommendations?limit=10"
	seen := map[string]bool{}
	var all []homeResource
	for page := 0; page < 20; page++ {
		if seen[endpoint] {
			return nil, fmt.Errorf("repeated recommendation category page")
		}
		seen[endpoint] = true
		var r struct {
			Data []homeResource `json:"data"`
			Next string         `json:"next"`
		}
		if err := c.get(ctx, endpoint, &r); err != nil {
			return nil, err
		}
		all = append(all, r.Data...)
		if r.Next == "" {
			c.homeGroups = all
			c.homeAt = time.Now()
			return all, nil
		}
		endpoint = r.Next
	}
	return nil, fmt.Errorf("too many recommendation category pages")
}
func (c *Client) home(ctx context.Context, q Query) (Page, error) {
	if q.Page == "home-section" {
		if q.ID == "" {
			return Page{}, fmt.Errorf("recommendation section requires an ID")
		}
		endpoint := "/me/recommendations/" + url.PathEscape(q.ID) + "/contents?limit=25"
		if q.Next != "" {
			endpoint = q.Next
		}
		var r struct {
			Data []homeResource `json:"data"`
			Next string         `json:"next"`
		}
		if err := c.get(ctx, endpoint, &r); err != nil {
			return Page{}, err
		}
		out := Page{Items: []Item{}, Next: r.Next}
		for _, child := range r.Data {
			out.Items = appendHome(out.Items, child, "")
		}
		return out, nil
	}
	groups, err := c.recommendationGroups(ctx, q.Refresh)
	if err != nil {
		return Page{}, err
	}
	out := Page{Items: []Item{}}
	var selected *homeResource
	for i := range groups {
		r := &groups[i]
		title := provider.Clean(r.Attributes.Title.Display)
		if title == "" {
			title = "For You"
		}
		out.Tabs = append(out.Tabs, catalog.Tab{ID: r.ID, Title: title, Query: Query{Source: music.Apple, Page: "home", Section: "songs", ID: r.ID}})
		if selected == nil || r.ID == q.ID {
			selected = r
		}
		if r.ID == q.ID {
			out.ActiveTab = r.ID
		}
	}
	if selected == nil {
		return out, nil
	}
	if out.ActiveTab == "" {
		selected = nil
		id := out.Tabs[0].ID
		for i := range groups {
			if groups[i].ID == id {
				selected = &groups[i]
				break
			}
		}
	}
	if selected == nil {
		return out, nil
	}
	out.ActiveTab = selected.ID
	title := provider.Clean(selected.Attributes.Title.Display)
	if q.Next != "" || len(selected.Relationships.Contents.Data) == 0 {
		endpoint := q.Next
		if endpoint == "" {
			endpoint = "/me/recommendations/" + url.PathEscape(selected.ID) + "/contents?limit=25"
		}
		var r struct {
			Data []homeResource `json:"data"`
			Next string         `json:"next"`
		}
		if err = c.get(ctx, endpoint, &r); err != nil {
			return Page{}, err
		}
		out.Next = r.Next
		for _, child := range r.Data {
			out.Items = appendHome(out.Items, child, title)
		}
	} else {
		out.Next = selected.Relationships.Contents.Next
		for _, child := range selected.Relationships.Contents.Data {
			out.Items = appendHome(out.Items, child, title)
		}
	}
	return out, nil
}

type homeResource struct {
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	Attributes    homeAttributes `json:"attributes"`
	Relationships struct {
		Contents struct {
			Data []homeResource `json:"data"`
			Next string         `json:"next"`
		} `json:"contents"`
	} `json:"relationships"`
}
type homeAttributes struct {
	Name     string    `json:"name"`
	Artist   string    `json:"artistName"`
	Curator  string    `json:"curatorName"`
	Duration int64     `json:"durationInMillis"`
	Title    homeTitle `json:"title"`
}

type homeTitle struct {
	Display string `json:"stringForDisplay"`
}

func (t *homeTitle) UnmarshalJSON(data []byte) error {
	var text string
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		t.Display = text
		return nil
	}
	type object homeTitle
	return json.Unmarshal(data, (*object)(t))
}

func appendHome(out []Item, r homeResource, group string) []Item {
	if r.Type == "personal-recommendation" || r.Type == "personal-recommendations" {
		title := provider.Clean(r.Attributes.Title.Display)
		if title == "" {
			title = "For You"
		}
		if group != "" {
			title = group + " / " + title
		}
		for _, child := range r.Relationships.Contents.Data {
			out = appendHome(out, child, title)
		}
		if len(r.Relationships.Contents.Data) == 0 || r.Relationships.Contents.Next != "" {
			q := Query{Source: music.Apple, Page: "home-section", Section: "songs", ID: r.ID}
			out = append(out, Item{Source: music.Apple, ID: r.ID, Kind: "recommendations", Title: title, Group: group, Subtitle: "Open recommendations", Open: &q})
		}
		return out
	}
	raw := resource{ID: r.ID, Type: r.Type}
	raw.Attributes.Name = r.Attributes.Name
	if raw.Attributes.Name == "" {
		raw.Attributes.Name = r.Attributes.Title.Display
	}
	raw.Attributes.Artist = r.Attributes.Artist
	raw.Attributes.Curator = r.Attributes.Curator
	raw.Attributes.Duration = r.Attributes.Duration
	page := convert(collection{Data: []resource{raw}})
	if len(page.Items) > 0 {
		i := page.Items[0]
		i.Group = group
		if strings.TrimSpace(i.Title) == "" {
			i.Title = "Untitled " + r.Type
		}
		if i.Track == nil && i.Play == nil && i.Open == nil {
			i.PlaybackNote = playbackNote(r.Type)
			if i.Subtitle == "" {
				i.Subtitle = i.PlaybackNote
			}
		}
		out = append(out, i)
	}
	return out
}
