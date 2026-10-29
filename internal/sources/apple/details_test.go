package apple

import (
	"context"
	"lazymusic/internal/catalog"
	"net/http"
	"strings"
	"testing"
)

func TestInspectResolvesLibrarySongAndEveryArtist(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me/library/songs/i.saved":
			w.Write([]byte(`{"data":[{"id":"i.saved","type":"library-songs","relationships":{"catalog":{"data":[{"id":"123","type":"songs"}]}}}]}`))
		case "/v1/catalog/cn/songs/123":
			if !strings.Contains(r.URL.Query().Get("include"), "artists") {
				t.Error("artist relationships not requested")
			}
			w.Write([]byte(`{"data":[{"id":"123","type":"songs","attributes":{"name":"Song","composerName":"Writer One & Writer Two","artistName":"Artist One & Artist Two"},"relationships":{"albums":{"data":[{"id":"a","type":"albums","attributes":{"name":"Album"}}]},"artists":{"data":[{"id":"one","type":"artists","attributes":{"name":"Artist One"}},{"id":"two","type":"artists","attributes":{"name":"Artist Two"}}]}}}]}`))
		default:
			t.Errorf("unexpected metadata request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	d, err := c.Inspect(context.Background(), catalog.Subject{Source: "apple", ID: "i.saved", Credits: true})
	if err != nil || len(d.Links) != 3 || d.Links[1].ID != "one" || d.Links[2].ID != "two" || d.Links[2].Open.Kind != "artists" {
		t.Fatalf("related works: %+v %v", d, err)
	}
	if len(d.Credits) != 2 || d.Credits[1].Name != "Writer One & Writer Two" || d.CreditsNote == "" {
		t.Fatal("partial composer credits are missing or presented as complete")
	}
}

func TestInspectPreservesContributorGroupsAndRoles(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"123","type":"songs","attributes":{"name":"Song"},"relationships":{"credits":{"data":[{"id":"production","type":"credits","attributes":{"title":"Production & Engineering"},"relationships":{"credit-artists":{"data":[{"id":"engineer","type":"credit-artists","attributes":{"name":"Engineer","roleNames":["Mixing Engineer","Producer"]}}]}}}]}}}]}`))
	})
	d, err := c.Inspect(context.Background(), catalog.Subject{ID: "123", Credits: true})
	if err != nil || len(d.Credits) != 1 || d.Credits[0].Group != "Production & Engineering" || len(d.Credits[0].Roles) != 2 || d.CreditsNote != "" {
		t.Fatalf("credit roles lost: %+v %v", d, err)
	}
}

func TestInspectCannotFollowForeignRelationshipPagination(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"123","type":"songs","relationships":{"artists":{"next":"https://example.invalid/steal","data":[]}}}]}`))
	})
	if _, err := c.Inspect(context.Background(), catalog.Subject{ID: "123"}); err == nil || !strings.Contains(err.Error(), "cross-origin") {
		t.Fatal("accepted a foreign metadata pagination URL")
	}
}
