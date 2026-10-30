package apple

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"lazymusic/internal/catalog"
	"lazymusic/internal/music"
)

func TestStationUsesExactIDAndBoundedRealSongBatches(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/v1/catalog/cn/stations/ra.personal" {
			fmt.Fprint(w, `{"data":[{"attributes":{"name":"Personal station","playParams":{"format":"tracks"}}}]}`)
			return
		}
		if r.Method != "POST" || r.URL.Path != "/v1/me/stations/next-tracks/ra.personal" {
			t.Fatalf("wrong station endpoint %s %s", r.Method, r.URL.Path)
		}
		calls++
		fmt.Fprintf(w, `{"data":[{"id":"%d-a","type":"songs","attributes":{"name":"A","durationInMillis":90000}},{"id":"%d-b","type":"songs","attributes":{"name":"B"}}]}`, calls, calls)
	})
	q := music.Query{Source: music.Apple, Page: "station", ID: "ra.personal", Continuous: true}
	tracks, err := c.StationTracks(context.Background(), q)
	if err != nil || len(tracks) != 6 || calls != 3 || tracks[0].ID != "1-a" || tracks[5].PlaybackID != "3-b" {
		t.Fatalf("batch %v %v calls=%d", tracks, err, calls)
	}
	q.Source = music.Local
	if _, err = c.StationTracks(context.Background(), q); err == nil {
		t.Fatal("accepted foreign station")
	}
}

func TestStationStreamsKeepEpisodeAndLiveMediaKinds(t *testing.T) {
	for _, live := range []bool{false, true} {
		t.Run(fmt.Sprint(live), func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Fatal("stream station used next-tracks")
				}
				fmt.Fprintf(w, `{"data":[{"attributes":{"name":"Program","isLive":%t,"durationInMillis":900000,"playParams":{"format":"stream"}}}]}`, live)
			})
			tracks, err := c.StationTracks(context.Background(), music.Query{Source: music.Apple, Page: "station", ID: "ra.program", Continuous: true})
			want := "episode"
			if live {
				want = "live"
			}
			if err != nil || len(tracks) != 1 || tracks[0].Kind != "stations" || tracks[0].MediaType != want || tracks[0].PlaybackID != "ra.program" {
				t.Fatalf("station stream identity: %v %v", tracks, err)
			}
			queued, err := catalog.All(context.Background(), c, music.Query{Source: music.Apple, Page: "station", ID: "ra.program", Continuous: true})
			if err != nil || len(queued) != 1 || queued[0].MediaType != want {
				t.Fatal("stream could not resolve as a single queue entry")
			}
		})
	}
}
