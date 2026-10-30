package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"lazymusic/internal/catalog"
	"lazymusic/internal/lyrics"
	"lazymusic/internal/music"
)

func (b *Browser) Lyrics(ctx context.Context, subject catalog.Subject) (lyrics.Document, error) {
	if err := ctx.Err(); err != nil {
		return lyrics.Document{}, err
	}
	if subject.Source != "" && subject.Source != music.Local {
		return lyrics.Document{}, fmt.Errorf("wrong lyrics source")
	}
	file, err := Resolve(b.Root, subject.ID)
	if err != nil {
		return lyrics.Document{}, fmt.Errorf("local lyrics: music file is unavailable")
	}
	if !Supported(file) {
		return lyrics.Document{}, nil
	}
	dir := filepath.Dir(file)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return lyrics.Document{}, fmt.Errorf("local lyrics folder is unavailable")
	}
	name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)) + ".lrc"
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(entry.Name(), name) {
			continue
		}
		root, err := filepath.EvalSymlinks(b.Root)
		if err != nil {
			return lyrics.Document{}, fmt.Errorf("local music folder is unavailable")
		}
		rel, err := filepath.Rel(root, filepath.Join(dir, entry.Name()))
		if err != nil {
			return lyrics.Document{}, fmt.Errorf("invalid local lyrics")
		}
		path, err := Resolve(b.Root, filepath.ToSlash(rel))
		if err != nil {
			return lyrics.Document{}, fmt.Errorf("local lyrics escape the music directory or are unavailable")
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > lyrics.MaxBytes {
			return lyrics.Document{}, fmt.Errorf("local lyrics exceed the supported size or are not a regular file")
		}
		f, err := os.Open(path)
		if err != nil {
			return lyrics.Document{}, fmt.Errorf("local lyrics could not be opened")
		}
		data, err := io.ReadAll(io.LimitReader(f, lyrics.MaxBytes+1))
		f.Close()
		if err != nil {
			return lyrics.Document{}, fmt.Errorf("local lyrics could not be read")
		}
		if err = ctx.Err(); err != nil {
			return lyrics.Document{}, err
		}
		doc, err := lyrics.ParseLRC(string(data))
		doc.Provider = "LOCAL"
		return doc, err
	}
	return lyrics.Document{}, nil
}
