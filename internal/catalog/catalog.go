// Package catalog defines source-independent navigation and collection contracts.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
)

type Item struct {
	Writable bool         `json:"writable,omitempty"`
	Source   string       `json:"source,omitempty"`
	Group    string       `json:"group,omitempty"`
	Open     *Query       `json:"open,omitempty"`
	Play     *Query       `json:"play,omitempty"`
	ID       string       `json:"id"`
	Kind     string       `json:"kind"`
	Title    string       `json:"title"`
	Subtitle string       `json:"subtitle,omitempty"`
	Track    *music.Track `json:"track,omitempty"`
}

// CursorID distinguishes the same recommendation in different Home groups.
func (i Item) CursorID() string {
	b, _ := json.Marshal([4]string{i.Source, i.Kind, i.ID, i.Group})
	return string(b)
}

type Query = music.Query

type Tab struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Query Query  `json:"query"`
}
type Page struct {
	Tabs      []Tab  `json:"tabs,omitempty"`
	ActiveTab string `json:"active_tab,omitempty"`
	Items     []Item `json:"items"`
	Next      string `json:"next,omitempty"`
}

type Browser interface {
	Browse(context.Context, Query) (Page, error)
}

// All expands paginated collections, including nested artist/album collections,
// in source order. A failure returns no partial selection.
func All(ctx context.Context, b Browser, q Query) ([]music.Track, error) {
	var tracks []music.Track
	pages := 0
	ancestors := map[string]bool{}
	var visit func(Query, int) error
	visit = func(q Query, depth int) error {
		key := q.Key()
		if depth > 16 || ancestors[key] {
			return fmt.Errorf("collection contains a cyclic or too deeply nested selection")
		}
		ancestors[key] = true
		defer delete(ancestors, key)
		seen := map[string]bool{}
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			pages++
			if pages > 1000 {
				return fmt.Errorf("collection exceeds 1000 pages")
			}
			p, err := b.Browse(ctx, q)
			if err != nil {
				return err
			}
			for _, i := range p.Items {
				if i.Track != nil {
					tracks = append(tracks, session.CleanTrack(*i.Track))
				} else if i.Play != nil {
					if err := visit(*i.Play, depth+1); err != nil {
						return err
					}
				}
			}
			if len(tracks) > 10000 {
				return fmt.Errorf("collection exceeds 10000 tracks")
			}
			if p.Next == "" {
				return nil
			}
			if seen[p.Next] {
				return fmt.Errorf("Source returned a repeated pagination link")
			}
			seen[p.Next] = true
			q.Next = p.Next
		}
	}
	if err := visit(q, 0); err != nil {
		return nil, err
	}
	return tracks, nil
}
