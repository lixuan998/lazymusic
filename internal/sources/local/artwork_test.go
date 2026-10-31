package local

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"lazymusic/internal/catalog"
)

func TestLocalAlbumArtworkAndSymlinkBoundary(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "song.mp3"), nil, 0600)
	path := filepath.Join(root, "Cover.PNG")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	f.Close()
	b := &Browser{Root: root}
	subject := catalog.Subject{Source: "local", ID: "song.mp3"}
	im, err := b.Artwork(context.Background(), subject)
	if err != nil || !im.Valid() {
		t.Fatal("local album cover missing", err)
	}
	outside := filepath.Join(t.TempDir(), "cover.png")
	os.Rename(path, outside)
	if err := os.Symlink(outside, path); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("Windows symlink privilege unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if _, err = b.Artwork(context.Background(), subject); err == nil {
		t.Fatal("artwork escaped the music root")
	}
}
