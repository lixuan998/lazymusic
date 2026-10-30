package catalog

import (
	"context"
	"errors"
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

type collectionBrowser func(context.Context, Query) (Page, error)

func (b collectionBrowser) Browse(ctx context.Context, q Query) (Page, error) { return b(ctx, q) }

func TestNestedArtistCollectionsExpandAllPagesInOrder(t *testing.T) {
	b := collectionBrowser(func(_ context.Context, q Query) (Page, error) {
		song := func(id string) Item { return Item{Track: &music.Track{ID: id}} }
		album := func(id string) Item { return Item{Play: &Query{Page: "album", ID: id}} }
		switch {
		case q.Page == "artist" && q.Next == "":
			return Page{Items: []Item{album("one")}, Next: "more-albums"}, nil
		case q.Page == "artist":
			return Page{Items: []Item{album("two")}}, nil
		case q.ID == "one" && q.Next == "":
			return Page{Items: []Item{song("a")}, Next: "more-tracks"}, nil
		case q.ID == "one":
			return Page{Items: []Item{song("b")}}, nil
		default:
			return Page{Items: []Item{song("c")}}, nil
		}
	})
	tracks, err := All(context.Background(), b, Query{Page: "artist"})
	if err != nil || len(tracks) != 3 || tracks[0].ID != "a" || tracks[1].ID != "b" || tracks[2].ID != "c" {
		t.Fatalf("nested collection: %v %v", tracks, err)
	}
}

func TestCollectionCyclesAndLateFailuresReturnNoPartialTracks(t *testing.T) {
	for _, cycle := range []bool{false, true} {
		b := collectionBrowser(func(_ context.Context, q Query) (Page, error) {
			if q.Page == "root" {
				return Page{Items: []Item{{Track: &music.Track{ID: "first"}}, {Play: &Query{Page: "child"}}}}, nil
			}
			if cycle {
				return Page{Items: []Item{{Play: &Query{Page: "root"}}}}, nil
			}
			return Page{}, errors.New("later album failed")
		})
		tracks, err := All(context.Background(), b, Query{Page: "root"})
		if err == nil || tracks != nil {
			t.Fatal("partial or cyclic collection was accepted")
		}
	}
}
func TestPlayCollectionLoadsAllPages(t *testing.T) {
	tracks, err := All(context.Background(), paginatedBrowser{}, Query{Page: "detail", Section: "songs", Kind: "playlists", ID: "fixture"})
	if err != nil || len(tracks) != 2 || tracks[1].ID != "two" {
		t.Fatalf("tracks=%v err=%v", tracks, err)
	}
}

func TestContinuousSelectionsRequireFiniteResolutionBeforeQueueExpansion(t *testing.T) {
	for _, infinite := range []bool{false, true} {
		calls := 0
		b := collectionBrowser(func(context.Context, Query) (Page, error) {
			calls++
			return Page{Continuous: infinite, Items: []Item{{Track: &music.Track{ID: "episode", MediaType: "episode"}}}}, nil
		})
		tracks, err := All(context.Background(), b, Query{Continuous: true, Page: "opaque-station"})
		if calls != 1 {
			t.Fatal("read an endless stream while resolving a queue selection")
		}
		if infinite {
			if err == nil || tracks != nil {
				t.Fatal("expanded an endless source")
			}
		} else if err != nil || len(tracks) != 1 || tracks[0].MediaType != "episode" {
			t.Fatal("failed to queue a finite episode")
		}
	}
}
