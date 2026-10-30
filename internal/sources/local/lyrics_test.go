package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"lazymusic/internal/catalog"
	"lazymusic/internal/lyrics"
)

func TestLocalLyricsUseSameNameAndStayInsideMusicRoot(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "Example.mp3"), nil, 0600)
	os.WriteFile(filepath.Join(root, "Example.LRC"), []byte("[00:01.00]Example line"), 0600)
	b := &Browser{Root: root}
	subject := catalog.Subject{Source: "local", ID: "Example.mp3"}
	doc, err := b.Lyrics(context.Background(), subject)
	if err != nil || len(doc.Lines) != 1 || doc.Provider != "LOCAL" {
		t.Fatal(doc, err)
	}
	os.Remove(filepath.Join(root, "Example.LRC"))
	outside := filepath.Join(t.TempDir(), "outside.lrc")
	os.WriteFile(outside, []byte("[00:01.00]Private outside file"), 0600)
	os.Symlink(outside, filepath.Join(root, "Example.lrc"))
	if _, err = b.Lyrics(context.Background(), subject); err == nil {
		t.Fatal("lyrics symlink escaped root")
	}
	os.Remove(filepath.Join(root, "Example.lrc"))
	os.WriteFile(filepath.Join(root, "Example.lrc"), make([]byte, lyrics.MaxBytes+1), 0600)
	if _, err = b.Lyrics(context.Background(), subject); err == nil {
		t.Fatal("oversized lyrics accepted")
	}
	os.Remove(filepath.Join(root, "Example.lrc"))
	if doc, err = b.Lyrics(context.Background(), subject); err != nil || len(doc.Lines) != 0 {
		t.Fatal("missing lyrics were not optional")
	}
}
