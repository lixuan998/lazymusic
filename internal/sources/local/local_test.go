package local

import (
	"context"
	"lazymusic/internal/catalog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFoldersSearchAndSourceIdentities(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "Artist", "Album")
	_ = os.MkdirAll(album, 0700)
	_ = os.WriteFile(filepath.Join(album, "中文 song.mp3"), []byte("fixture"), 0600)
	_ = os.WriteFile(filepath.Join(album, "not-a-song.txt"), nil, 0600)
	b := &Browser{Root: root}
	p, err := b.Browse(context.Background(), catalog.Query{Source: "local", Page: "library", Section: "songs"})
	if err != nil || len(p.Items) != 1 {
		t.Fatalf("library: %v %+v", err, p)
	}
	if p.Items[0].Track.Source != "local" || p.Items[0].Track.ID != "Artist/Album/中文 song.mp3" {
		t.Fatal("track identity is not rooted and source scoped")
	}
	p, err = b.Browse(context.Background(), catalog.Query{Source: "local", Page: "folders"})
	if err != nil || len(p.Items) != 1 || p.Items[0].Open == nil || p.Items[0].Play == nil {
		t.Fatal("folder actions missing")
	}
	p, err = b.Browse(context.Background(), catalog.Query{Source: "local", Page: "search", Section: "songs", Term: "中文"})
	if err != nil || len(p.Items) != 1 {
		t.Fatal("local search did not match Unicode")
	}
	if _, err = Resolve(root, "../outside.mp3"); err == nil {
		t.Fatal("accepted parent traversal")
	}
	outside := filepath.Join(t.TempDir(), "outside.mp3")
	_ = os.WriteFile(outside, nil, 0600)
	_ = os.Symlink(outside, filepath.Join(root, "link.mp3"))
	if _, err = Resolve(root, "link.mp3"); err == nil {
		t.Fatal("accepted escaping symlink")
	}
}

func TestArtistCollectionIncludesEveryLocalAlbum(t *testing.T) {
	root := t.TempDir()
	for _, album := range []string{"One", "Two"} {
		dir := filepath.Join(root, "Artist", album)
		os.MkdirAll(dir, 0700)
		if err := os.WriteFile(filepath.Join(dir, "Song.mp3"), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	b := &Browser{Root: root}
	p, err := b.Browse(context.Background(), catalog.Query{Page: "library", Section: "artists"})
	if err != nil || len(p.Items) != 1 || p.Items[0].Play == nil {
		t.Fatal("local artist has no collection query")
	}
	tracks, err := catalog.All(context.Background(), b, *p.Items[0].Play)
	if err != nil || len(tracks) != 2 || tracks[0].Artist != "Artist" || tracks[1].Artist != "Artist" {
		t.Fatalf("artist collection: %v %v", tracks, err)
	}
}

func TestConfigureCanonicalizesSelectedFolderAndStoresPrivateConfig(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "music-link")
	if err := os.Symlink(root, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("Windows symlink privilege unavailable: %v", err)
		}
		t.Fatal(err)
	}
	profile := t.TempDir()
	if err := Configure(profile, link); err != nil {
		t.Fatal(err)
	}
	cfg, err := ReadConfig(profile)
	canonical, _ := filepath.EvalSymlinks(root)
	if err != nil || cfg.Root != canonical {
		t.Fatal("selected folder symlink was not normalized")
	}
	fi, err := os.Stat(filepath.Join(profile, "local.json"))
	if err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0600) {
		t.Fatal("local config was not stored privately")
	}
}

func TestInspectLocalLinksAreRootedAndCreditsAreNotInvented(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "Artist", "Album")
	if err := os.MkdirAll(album, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(album, "Song.flac"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	b := &Browser{Root: root}
	d, err := b.Inspect(context.Background(), catalog.Subject{Source: "local", ID: "Artist/Album/Song.flac", Credits: true})
	if err != nil || len(d.Links) != 2 || d.Links[0].Open.ID != "Artist/Album" || d.Links[1].Open.ID != "Artist" {
		t.Fatalf("local related links: %+v %v", d, err)
	}
	if len(d.Credits) != 0 || d.CreditsNote == "" {
		t.Fatal("folder names were presented as verified credits")
	}
	if _, err := b.Inspect(context.Background(), catalog.Subject{ID: "../outside.flac"}); err == nil {
		t.Fatal("metadata inspection accepted traversal")
	}
}
