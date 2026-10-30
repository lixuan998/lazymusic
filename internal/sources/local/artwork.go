package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"lazymusic/internal/artwork"
	"lazymusic/internal/catalog"
	"lazymusic/internal/music"
)

func (b *Browser) Artwork(ctx context.Context, subject catalog.Subject) (artwork.Image, error) {
	if err := ctx.Err(); err != nil {
		return artwork.Image{}, err
	}
	if subject.Source != "" && subject.Source != music.Local {
		return artwork.Image{}, fmt.Errorf("wrong artwork source")
	}
	file, err := Resolve(b.Root, subject.ID)
	if err != nil {
		return artwork.Image{}, fmt.Errorf("local artwork: music file is unavailable")
	}
	if !Supported(file) {
		return artwork.Image{}, nil
	}
	dir := filepath.Dir(file)
	root, err := filepath.EvalSymlinks(b.Root)
	if err != nil {
		return artwork.Image{}, fmt.Errorf("local music folder is unavailable")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return artwork.Image{}, fmt.Errorf("local artwork folder is unavailable")
	}
	for _, name := range []string{"cover.jpg", "cover.jpeg", "cover.png", "folder.jpg", "folder.png", "front.jpg", "front.png"} {
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(entry.Name(), name) {
				continue
			}
			rel, err := filepath.Rel(root, filepath.Join(dir, entry.Name()))
			if err != nil {
				return artwork.Image{}, fmt.Errorf("invalid local artwork")
			}
			path, err := Resolve(b.Root, filepath.ToSlash(rel))
			if err != nil {
				return artwork.Image{}, fmt.Errorf("local artwork escapes the music directory or is unavailable")
			}
			info, err := os.Stat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > artwork.MaxBytes {
				return artwork.Image{}, fmt.Errorf("local artwork is not a supported image file")
			}
			f, err := os.Open(path)
			if err != nil {
				return artwork.Image{}, fmt.Errorf("local artwork could not be opened")
			}
			defer f.Close()
			if err = ctx.Err(); err != nil {
				return artwork.Image{}, err
			}
			return artwork.Decode(f)
		}
	}
	return artwork.Image{}, nil
}
