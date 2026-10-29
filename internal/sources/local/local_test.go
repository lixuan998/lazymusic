package local

import (
	"context"
	"lazymusic/internal/catalog"
	"os"
	"path/filepath"
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

func TestConfigureCanonicalizesSelectedFolderAndStoresPrivateConfig(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "music-link")
	if err := os.Symlink(root, link); err != nil {
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
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Fatal("local config was not stored privately")
	}
}
