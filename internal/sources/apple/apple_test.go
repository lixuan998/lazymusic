package apple

import (
	"context"
	"encoding/json"
	"lazymusic/internal/catalog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lazymusic/internal/vibez/config"
)

func testClient(t *testing.T, fn http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(fn)
	t.Cleanup(server.Close)
	c := New(&config.Config{AppleDeveloperToken: "dev", AppleUserToken: "user", StoreFront: "cn"})
	c.base = server.URL + "/v1"
	return c
}

func TestArtistPickExpandsAllAlbumAndTrackPages(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/catalog/cn/search":
			w.Write([]byte(`{"results":{"artists":{"data":[{"id":"artist","type":"artists","attributes":{"name":"Artist"}}]}}}`))
		case "/v1/catalog/cn/artists/artist/albums":
			if r.URL.Query().Get("offset") == "50" {
				w.Write([]byte(`{"data":[{"id":"two","type":"albums"}]}`))
			} else {
				w.Write([]byte(`{"data":[{"id":"one","type":"albums"}],"next":"/v1/catalog/cn/artists/artist/albums?offset=50"}`))
			}
		case "/v1/catalog/cn/albums/one/tracks":
			if r.URL.Query().Get("offset") == "50" {
				w.Write([]byte(`{"data":[{"id":"b","type":"songs"}]}`))
			} else {
				w.Write([]byte(`{"data":[{"id":"a","type":"songs"}],"next":"/v1/catalog/cn/albums/one/tracks?offset=50"}`))
			}
		case "/v1/catalog/cn/albums/two/tracks":
			w.Write([]byte(`{"data":[{"id":"c","type":"songs"}]}`))
		default:
			t.Errorf("unexpected collection route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	p, err := c.Browse(context.Background(), Query{Page: "search", Section: "artists", Term: "Artist"})
	if err != nil || len(p.Items) != 1 || p.Items[0].Play == nil {
		t.Fatal("artist does not expose a collection query")
	}
	tracks, err := catalog.All(context.Background(), c, *p.Items[0].Play)
	if err != nil || len(tracks) != 3 || tracks[0].ID != "a" || tracks[1].ID != "b" || tracks[2].ID != "c" {
		t.Fatalf("artist collection: %v %v", tracks, err)
	}
}

func TestSearchScopesAndWrappedPagination(t *testing.T) {
	var paths []string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.String())
		typ := r.URL.Query().Get("types")
		if r.URL.Query().Get("offset") == "25" {
			_, _ = w.Write([]byte(`{"results":{"library-songs":{"data":[{"id":"i.2","type":"library-songs","attributes":{"name":"Two"}}]}}}`))
			return
		}
		next := ""
		if typ == "library-songs" {
			next = "/v1/me/library/search?offset=25&types=library-songs"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": map[string]any{typ: map[string]any{"next": next, "data": []any{map[string]any{"id": "i.1", "type": typ, "attributes": map[string]any{"name": "One"}}}}}})
	})
	p, err := c.Browse(context.Background(), Query{Page: "search", Scope: "library", Section: "songs", Term: "中文 q"})
	if err != nil {
		t.Fatal(err)
	}
	p2, err := c.Browse(context.Background(), Query{Page: "search", Scope: "library", Section: "songs", Term: "中文 q", Next: p.Next})
	if err != nil || len(p2.Items) != 1 || p2.Items[0].ID != "i.2" {
		t.Fatalf("pagination=%+v %v", p2, err)
	}
	_, err = c.Browse(context.Background(), Query{Page: "search", Scope: "apple", Section: "albums", Term: "music"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(paths[0], "/v1/me/library/search?") || !strings.Contains(paths[2], "/catalog/cn/search?") {
		t.Fatalf("wrong scopes: %v", paths)
	}
}

func TestCatalogVideoSearchReturnsPlayableMediaAndKeepsPagination(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/catalog/cn/search" || r.URL.Query().Get("types") != "music-videos" {
			t.Fatal("wrong video catalog route")
		}
		w.Write([]byte(`{"results":{"music-videos":{"data":[{"id":"movie","type":"music-videos","attributes":{"name":"Video","durationInMillis":120000}}],"next":"/v1/catalog/cn/search?types=music-videos&offset=25"}}}`))
	})
	q := Query{Page: "search", Section: "music-videos", Term: "video"}
	for i := 0; i < 2; i++ {
		p, err := c.Browse(context.Background(), q)
		if err != nil || len(p.Items) != 1 || p.Items[0].Track == nil || p.Items[0].Track.MediaType != "video" || p.Items[0].Track.PlaybackID != "movie" {
			t.Fatal("video became an unsupported catalog item", p, err)
		}
		q.Next = p.Next
	}
	q.Next, q.Scope = "", "library"
	if _, err := c.Browse(context.Background(), q); err == nil {
		t.Fatal("unsupported library video search was sent")
	}
}

func TestPaginationCannotSendCredentialsToAnotherHost(t *testing.T) {
	hit := false
	outside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer outside.Close()
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})
	if _, err := c.Browse(context.Background(), Query{Next: outside.URL + "/steal"}); err == nil || hit {
		t.Fatal("accepted cross-origin pagination")
	}
}
