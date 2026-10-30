package apple

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestHomeFlattensGroupsAndProvidesSourceScopedActions(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") == "10" {
			_, _ = w.Write([]byte(`{"data":[{"id":"next-category","type":"personal-recommendation","attributes":{"title":{"stringForDisplay":"Listen Again"}},"relationships":{"contents":{"data":[{"id":"another","type":"albums","attributes":{"name":"Another Album"}}]}}}]}`))
			return
		}
		if strings.Contains(r.URL.Path, "/contents") {
			_, _ = w.Write([]byte(`{"data":[{"id":"s","type":"songs","attributes":{"name":"Song","durationInMillis":1000}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"next":"/v1/me/recommendations?offset=10","data":[{"id":"group","type":"personal-recommendation","attributes":{"title":{"stringForDisplay":"Made for You"}},"relationships":{"contents":{"data":[{"id":"album","type":"albums","attributes":{"name":"Album","artistName":"Artist"}},{"id":"nested","type":"personal-recommendation","attributes":{"title":{"stringForDisplay":"Discover"}},"relationships":{"contents":{"data":[{"id":"list","type":"playlists","attributes":{"name":"Mix"}}]}}},{"id":"station","type":"stations","attributes":{"name":"Radio"}}]}}},{"id":"lazy","type":"personal-recommendation","attributes":{"title":{"stringForDisplay":"More"}},"relationships":{"contents":{"data":[]}}}]}`))
	})
	p, err := c.Browse(context.Background(), Query{Source: "apple", Page: "home"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 3 || len(p.Tabs) != 3 || p.ActiveTab != "group" {
		t.Fatalf("home=%+v", p)
	}
	for _, i := range p.Items[:2] {
		if i.Source != "apple" || i.Open == nil || i.Play == nil || i.Play.Source != "apple" || i.Play.Page != "detail" {
			t.Fatalf("missing actions: %+v", i)
		}
	}
	if p.Items[1].Group != "Made for You / Discover" {
		t.Fatal("lost nested group context")
	}
	if p.Items[2].ID != "station" || p.Items[2].Play == nil || !p.Items[2].Play.Continuous || p.Items[2].Play.ID != "station" {
		t.Fatal("station was hidden or lost its continuous play action")
	}
	more, err := c.Browse(context.Background(), p.Tabs[1].Query)
	if err != nil || len(more.Items) != 1 || more.Items[0].Track == nil {
		t.Fatalf("lazy section: %v %+v", err, more)
	}
}

func TestHomeKeepsStationOnlyAndUnknownResourceCategories(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"stations","attributes":{"title":{"stringForDisplay":"Stations for You"}},"relationships":{"contents":{"data":[{"id":"station","type":"stations","attributes":{"name":"Personal station"}}]}}},{"id":"mixed","attributes":{"title":{"stringForDisplay":"Find your mood"}},"relationships":{"contents":{"data":[{"id":"video","type":"music-videos","attributes":{"name":"Film"}},{"id":"unknown","type":"future-resource","attributes":{"title":"Something new"}}]}}}]}`))
	})
	p, err := c.Browse(context.Background(), Query{Page: "home"})
	if err != nil || len(p.Tabs) != 2 || p.Tabs[0].Title != "Stations for You" || len(p.Items) != 1 || p.Items[0].Kind != "stations" {
		t.Fatalf("station-only category: %+v %v", p, err)
	}
	p, err = c.Browse(context.Background(), p.Tabs[1].Query)
	if err != nil || len(p.Items) != 2 || p.Items[0].Kind != "music-videos" || p.Items[1].Title != "Something new" {
		t.Fatalf("resources were filtered: %+v %v", p, err)
	}
	for _, item := range p.Items {
		if item.PlaybackNote == "" || item.Track != nil || item.Play != nil {
			t.Fatal("missing honest playback limit")
		}
	}
}
func TestHomeDoesNotHideAuthenticationErrors(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) })
	if _, err := c.Browse(context.Background(), Query{Page: "home"}); err == nil {
		t.Fatal("presented authorization failure as empty recommendations")
	}
}
