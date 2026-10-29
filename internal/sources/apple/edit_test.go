package apple

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"lazymusic/internal/catalog"
	"lazymusic/internal/music"
)

func TestLibraryAddsUseTypedCatalogIDsAndReportAccepted(t *testing.T) {
	for _, kind := range []string{"songs", "albums"} {
		t.Run(kind, func(t *testing.T) {
			count := 0
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				count++
				if r.Method != "POST" || r.URL.Path != "/v1/me/library" || r.URL.Query().Get("ids["+kind+"]") != "123" {
					t.Errorf("wrong request %s %s", r.Method, r.URL)
				}
				if r.Header.Get("Authorization") != "Bearer dev" || r.Header.Get("Music-User-Token") != "user" {
					t.Error("missing authorization")
				}
				w.WriteHeader(http.StatusAccepted)
			})
			result, err := c.Mutate(context.Background(), catalog.Mutation{Source: music.Apple, Op: catalog.AddLibrary, Subject: catalog.Subject{ID: "123", Kind: kind}})
			if err != nil || !result.Pending || count != 1 || !strings.Contains(result.Message, "accepted") {
				t.Fatalf("result %+v %v, requests %d", result, err, count)
			}
		})
	}
}

func TestCreatePlaylistEscapesUnicodeNameAndDoesNotRetry(t *testing.T) {
	count := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		count++
		if r.Method != "POST" || r.URL.Path != "/v1/me/library/playlists" {
			t.Error("wrong create endpoint")
		}
		var body struct{ Attributes struct{ Name string } }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Attributes.Name != "夜曲 \"live\"" {
			t.Errorf("wrong body %+v %v", body, err)
		}
		w.WriteHeader(http.StatusInternalServerError)
	})
	_, err := c.Mutate(context.Background(), catalog.Mutation{Source: music.Apple, Op: catalog.CreatePlaylist, Name: "夜曲 \"live\""})
	if err == nil || count != 1 || !strings.Contains(err.Error(), "before retrying") {
		t.Fatalf("write retried or error hidden: %v %d", err, count)
	}
}

func TestPlaylistAdditionChecksCanEditAndPreservesSongIdentity(t *testing.T) {
	for _, editable := range []bool{true, false} {
		for _, kind := range []string{"songs", "library-songs"} {
			writes := 0
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					if r.URL.Path != "/v1/me/library/playlists/p.1" {
						t.Error("wrong ownership check")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "p.1", "attributes": map[string]bool{"canEdit": editable}}}})
					return
				}
				writes++
				if r.Method != "POST" || r.URL.Path != "/v1/me/library/playlists/p.1/tracks" {
					t.Error("wrong playlist endpoint")
				}
				var body struct{ Data []struct{ ID, Type string } }
				_ = json.NewDecoder(r.Body).Decode(&body)
				if len(body.Data) != 1 || body.Data[0].ID != "song" || body.Data[0].Type != kind {
					t.Errorf("wrong identity: %+v", body)
				}
				w.WriteHeader(http.StatusNoContent)
			})
			_, err := c.Mutate(context.Background(), catalog.Mutation{Source: music.Apple, Op: catalog.AddPlaylist, Subject: catalog.Subject{ID: "song", Kind: kind}, PlaylistID: "p.1"})
			if editable && (err != nil || writes != 1) {
				t.Fatalf("editable write failed: %v %d", err, writes)
			}
			if !editable && (err == nil || writes != 0) {
				t.Fatal("readonly playlist modified")
			}
		}
	}
}

func TestUnsupportedAndCrossSourceWritesNeverReachHTTP(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unsupported write sent to Apple") })
	for _, m := range []catalog.Mutation{
		{Op: catalog.DeletePlaylist, Subject: catalog.Subject{ID: "p.1"}},
		{Op: catalog.RemoveLibrary, Subject: catalog.Subject{ID: "i.1"}},
		{Source: music.Local, Op: catalog.CreatePlaylist, Name: "Test"},
		{Op: catalog.AddLibrary, Subject: catalog.Subject{ID: "i.1", Kind: "library-songs"}},
		{Op: catalog.AddLibrary, Subject: catalog.Subject{ID: "1", Kind: "artists"}},
		{Op: catalog.CreatePlaylist, Name: "\n"},
		{Op: catalog.AddLibrary, Subject: catalog.Subject{ID: "one,two", Kind: "songs"}},
		{Op: catalog.AddPlaylist, Subject: catalog.Subject{ID: "song", Kind: "songs"}, PlaylistID: ".."},
	} {
		if _, err := c.Mutate(context.Background(), m); err == nil {
			t.Fatalf("invalid write accepted: %+v", m)
		}
	}
	a, err := c.Actions(context.Background(), catalog.Subject{ID: "i.1"})
	if err != nil || a.AddLibrary || a.Playlists == nil || a.RemovalNote == "" {
		t.Fatalf("wrong library actions: %+v %v", a, err)
	}
}
