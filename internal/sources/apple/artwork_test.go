package apple

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"

	"lazymusic/internal/catalog"
)

type artworkTransport func(*http.Request) (*http.Response, error)

func (f artworkTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestArtworkResolvesCatalogAndDownloadsWithoutAccountHeaders(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/catalog/cn/songs/catalog-id" {
			t.Error("wrong artwork resource", r.URL.Path)
		}
		w.Write([]byte(`{"data":[{"id":"catalog-id","attributes":{"artwork":{"url":"https://is1-ssl.mzstatic.com/image/{w}x{h}{c}.{f}"}}}]}`))
	})
	var cover bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 4, 4))
	im.Set(2, 2, color.RGBA{R: 255, A: 255})
	png.Encode(&cover, im)
	api := c.http.Transport
	if api == nil {
		api = http.DefaultTransport
	}
	calls := 0
	c.http.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() != "is1-ssl.mzstatic.com" {
			return api.RoundTrip(r)
		}
		calls++
		if r.Header.Get("Authorization") != "" || r.Header.Get("Music-User-Token") != "" || r.Header.Get("Cookie") != "" {
			t.Error("account material sent to artwork host")
		}
		if r.URL.Path != "/image/384x384bb.jpg" {
			t.Error("unresolved artwork template")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(cover.Bytes())), Header: http.Header{}}, nil
	})
	got, err := c.Artwork(context.Background(), catalog.Subject{Source: "apple", ID: "i.saved", PlaybackID: "catalog-id"})
	if err != nil || !got.Valid() || calls != 1 {
		t.Fatalf("artwork: %v calls=%d", err, calls)
	}
	for _, raw := range []string{"http://is1-ssl.mzstatic.com/private", "https://is1-ssl.mzstatic.com.evil.invalid/private", "https://127.0.0.1/private", "https://user:password@is1-ssl.mzstatic.com/private"} {
		if _, err = c.downloadArtwork(context.Background(), raw); err == nil || strings.Contains(err.Error(), "private") {
			t.Fatal("accepted or leaked an unsafe image URL")
		}
	}
}
