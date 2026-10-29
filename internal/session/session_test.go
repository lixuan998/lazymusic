package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lazymusic/internal/music"
)

func TestSnapshotRoundTripAndNoStreamURLs(t *testing.T) {
	dir := t.TempDir()
	s := Snapshot{Backend: "apple", UI: DefaultUI(), Playback: Playback{Queue: []music.Track{{ID: "1", Title: "中文", PreviewURL: "https://stream.invalid/?secret=abc", ArtworkURL: "https://art.invalid/"}}, Position: 18 * time.Second, Volume: .4, WasPlaying: true}}
	s.UI.Location = Location{Page: "search", Scope: "library", Query: "中文"}
	s.UI.Cursors["saved"] = Cursor{Index: 40, Offset: 29, ID: "resource"}
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	r, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Playback.Position != 18*time.Second || r.UI.Cursors["saved"].Offset != 29 || r.UI.Location.Query != "中文" {
		t.Fatalf("lost session: %+v", r)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "session.json"))
	if strings.Contains(string(b), "secret=abc") || strings.Contains(string(b), "art.invalid") {
		t.Fatal("persisted media URLs")
	}
	fi, _ := os.Stat(filepath.Join(dir, "session.json"))
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("mode=%o", fi.Mode().Perm())
	}
}

func TestUnknownVersionIsNotSilentlyReplaced(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "session.json"), []byte(`{"version":99}`), 0600)
	if _, err := Load(dir); err == nil {
		t.Fatal("accepted unknown version")
	}
}

func TestUICloneDoesNotShareCursorsOrHistory(t *testing.T) {
	u := DefaultUI()
	u.History = []Location{{Title: "before"}}
	v := u.Clone()
	v.History[0].Title = "after"
	v.Cursors["x"] = Cursor{Index: 9}
	if u.History[0].Title != "before" || len(u.Cursors) != 0 {
		t.Fatal("UI clone aliases mutable state")
	}
}

func TestV1MigrationKeepsAppleTrackIdentityAndView(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "session.json"), []byte(`{"version":1,"backend":"apple","ui":{"location":{"page":"library","section":"songs"},"nav_index":0},"playback":{"queue":[{"ID":"i.old","CatalogID":"42","Title":"Saved"}],"position_ns":17000000000,"volume":0.4}}`), 0600)
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Version != Version || s.UI.Location.Page != "library" || s.UI.NavIndex != 1 || s.Playback.Queue[0].Source != "apple" || s.Playback.Queue[0].PlaybackID != "42" || s.Playback.Position != 17*time.Second {
		t.Fatalf("migration lost data: %+v", s)
	}
}
