package apple

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"lazymusic/internal/music"
	"lazymusic/internal/vibez/config"
)

// Only synthetic content metadata belongs in fixtures, never Apple's web
// authorization/configuration payload or an account's recommendations.
func editorialFixture(sections ...any) string {
	data, _ := json.Marshal(map[string]any{"data": []any{map[string]any{"data": map[string]any{"sections": sections}}}})
	return `<html><script type="application/json" id="serialized-server-data">` + string(data) + `</script></html>`
}
func editorialHeader(title, link string) any {
	return map[string]any{"item": map[string]any{"titleLink": map[string]any{"title": title, "url": link}}}
}
func editorialEntry(kind, id, title string) any {
	return map[string]any{"id": "internal-row-" + id, "title": title, "contentDescriptor": map[string]any{"kind": kind, "identifiers": map[string]string{"storeAdamID": id}}}
}

func TestEditorialPreservesCategoriesAllResourceKindsAndRealCatalogIDs(t *testing.T) {
	html := editorialFixture(
		map[string]any{"id": "featured", "items": []any{editorialEntry("album", "album-id", "Album"), editorialEntry("playlist", "pl.mix", "Mix"), editorialEntry("song", "song-id", "Song"), editorialEntry("artist", "artist-id", "Artist")}},
		map[string]any{"id": "stations", "header": editorialHeader("Stations", "https://music.apple.com/cn/room/more-stations"), "items": []any{editorialEntry("radioStation", "station-id", "Station"), editorialEntry("musicVideo", "video-id", "Video"), editorialEntry("future-kind", "future-id", "Future")}},
		map[string]any{"id": "empty", "header": editorialHeader("Empty category", ""), "items": []any{}},
	)
	groups, err := parseEditorial([]byte(html), "cn", "/cn/new")
	if err != nil || len(groups) != 3 || groups[0].title != "Featured" || groups[1].title != "Stations" || groups[2].title != "Empty category" {
		t.Fatalf("categories: %+v %v", groups, err)
	}
	for _, item := range groups[0].items {
		if item.Source != music.Apple || strings.HasPrefix(item.ID, "internal-row-") || (item.Track == nil && item.Play == nil) {
			t.Fatalf("wrong resource action: %+v", item)
		}
	}
	if groups[0].items[2].Track.PlaybackID != "song-id" {
		t.Fatal("used the internal web row ID as a playback ID")
	}
	if len(groups[1].items) != 4 {
		t.Fatal("unsupported resources were filtered")
	}
	station := groups[1].items[0]
	if station.Play == nil || !station.Play.Continuous || station.Play.ID != "station-id" {
		t.Fatal("station has no continuous play action")
	}
	if v := groups[1].items[1]; v.Track == nil || v.Track.MediaType != "video" || v.Track.Kind != "music-videos" {
		t.Fatal("video lost its native playback identity")
	}
	for _, item := range groups[1].items[2:3] {
		if item.PlaybackNote == "" || item.Play != nil || item.Track != nil {
			t.Fatalf("unsupported playback: %+v", item)
		}
	}
	more := groups[1].items[3]
	if more.Open == nil || more.Open.Page != "editorial" || more.Open.Term != "/cn/room/more-stations" {
		t.Fatalf("lost view-all link: %+v", more)
	}
}

func TestEditorialBrowsePaginationCacheRefreshAndNoAccountCredentials(t *testing.T) {
	var calls atomic.Int32
	var entries []any
	for i := 0; i < 67; i++ {
		entries = append(entries, editorialEntry("song", fmt.Sprint(i), fmt.Sprintf("Song %d", i)))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "" || r.Header.Get("Music-User-Token") != "" || r.Header.Get("Cookie") != "geo=CN" {
			t.Error("account credentials leaked into public content request")
		}
		if r.URL.Path != "/cn/new" && r.URL.Path != "/cn/radio" && r.URL.Path != "/cn/room/more" {
			t.Errorf("wrong editorial route: %s", r.URL.Path)
		}
		fmt.Fprint(w, editorialFixture(map[string]any{"id": "all", "header": editorialHeader("All", ""), "items": entries}))
	}))
	defer server.Close()
	c := New(&config.Config{AppleDeveloperToken: "private-dev", AppleUserToken: "private-user", StoreFront: "cn"})
	c.publicBase = server.URL
	q := Query{Source: music.Apple, Page: "new"}
	p, err := c.Browse(context.Background(), q)
	if err != nil || len(p.Items) != 50 || p.Next == "" || len(p.Tabs) != 1 {
		t.Fatalf("first page: %+v %v", p, err)
	}
	q = p.Tabs[0].Query
	q.Next = p.Next
	p, err = c.Browse(context.Background(), q)
	if err != nil || len(p.Items) != 17 || p.Items[0].ID != "50" || p.Next != "" || calls.Load() != 1 {
		t.Fatalf("continuation: %+v %v calls=%d", p, err, calls.Load())
	}
	q.Next = ""
	q.Refresh = true
	if _, err = c.Browse(context.Background(), q); err != nil || calls.Load() != 2 {
		t.Fatalf("refresh: %v calls=%d", err, calls.Load())
	}
	for _, q := range []Query{{Page: "radio"}, {Page: "editorial", Term: "/cn/room/more"}} {
		if _, err = c.Browse(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	for _, cursor := range []string{"-1", "9999", "/v1/me/library", "bad"} {
		q.Next = cursor
		if _, err = c.Browse(context.Background(), q); err == nil {
			t.Fatalf("accepted invalid cursor %q", cursor)
		}
	}
}

func TestEditorialRejectsExternalPathsAndWrongPageRedirects(t *testing.T) {
	for _, link := range []string{"https://evil.example/cn/new", "//evil.example/cn/new", "http://music.apple.com/cn/new", "javascript:/cn/new", "/us/radio", "/cn/../us/new", "/cn/room/%2e%2e/../new", "/cn/new?token=x", "https://user@music.apple.com/cn/new"} {
		if _, err := editorialPath(link, "cn"); err == nil {
			t.Errorf("accepted unsafe editorial path %q", link)
		}
	}
	for _, mode := range []string{"wrong-page", "external", "invalid-body", "http-error"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case mode == "external":
					http.Redirect(w, r, "https://example.invalid/cn/radio", 302)
				case mode == "http-error":
					w.WriteHeader(503)
				case mode == "wrong-page" && r.URL.Path == "/cn/radio":
					http.Redirect(w, r, "/cn/new", 302)
				case mode == "invalid-body":
					fmt.Fprint(w, "<html>Unsupported page</html>")
				default:
					fmt.Fprint(w, editorialFixture(map[string]any{"id": "new"}))
				}
			}))
			defer server.Close()
			c := New(&config.Config{StoreFront: "cn"})
			c.publicBase = server.URL
			if _, err := c.Browse(context.Background(), Query{Page: "radio"}); err == nil {
				t.Fatal("silently presented a wrong or unavailable page")
			}
		})
	}
}

func TestRadioLinkedTitlesAndEditorialRoomIntents(t *testing.T) {
	var section webSection
	err := json.Unmarshal([]byte(`{"id":"radio","items":[{"id":"internal","titleLinks":[{"title":"My station"}],"subtitleLinks":[{"title":"Music"},{"title":"Live"}],"contentDescriptor":{"kind":"radioStation","identifiers":{"storeAdamID":"ra.station"}}},{"id":"room","title":"Explore","segue":{"destination":{"intent":{"id":"mood-room","$kind":"MultiRoomPageIntent"}}}}]}`), &section)
	if err != nil {
		t.Fatal(err)
	}
	station := editorialItem(section.Items[0], "", "cn", "Radio", "radio", 0)
	if station.Title != "My station" || station.Subtitle != "Music · Live" || station.ID != "ra.station" || station.Kind != "stations" {
		t.Fatalf("lost radio metadata: %+v", station)
	}
	room := editorialItem(section.Items[1], "", "cn", "Radio", "radio", 1)
	if room.Open == nil || room.Open.Term != "/cn/multiroom/mood-room" {
		t.Fatalf("lost editorial navigation: %+v", room)
	}
}
