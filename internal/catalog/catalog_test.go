package catalog

import (
	"context"
	"lazymusic/internal/music"
	"testing"
)

type paginatedBrowser struct{}

func (paginatedBrowser) Browse(_ context.Context, q Query) (Page, error) {
	if q.Next == "" {
		return Page{Items: []Item{{Track: &music.Track{ID: "one"}}}, Next: "second"}, nil
	}
	return Page{Items: []Item{{Track: &music.Track{ID: "two"}}}}, nil
}
func TestPlayCollectionLoadsAllPages(t *testing.T) {
	tracks, err := All(context.Background(), paginatedBrowser{}, Query{Page: "detail", Section: "songs", Kind: "playlists", ID: "fixture"})
	if err != nil || len(tracks) != 2 || tracks[1].ID != "two" {
		t.Fatalf("tracks=%v err=%v", tracks, err)
	}
}
