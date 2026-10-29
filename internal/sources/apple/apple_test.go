package apple

import (
	"context"
	"encoding/json"
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

func TestPaginationCannotSendCredentialsToAnotherHost(t *testing.T) {
	hit := false
	outside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer outside.Close()
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})
	if _, err := c.Browse(context.Background(), Query{Next: outside.URL + "/steal"}); err == nil || hit {
		t.Fatal("accepted cross-origin pagination")
	}
}
