package local

import (
	"context"
	"fmt"
	"lazymusic/internal/catalog"
	"lazymusic/internal/music"
	"os"
	"path/filepath"
)

func (b *Browser) Inspect(ctx context.Context, subject catalog.Subject) (catalog.Details, error) {
	if err := ctx.Err(); err != nil {
		return catalog.Details{}, err
	}
	if subject.Source != "" && subject.Source != music.Local {
		return catalog.Details{}, fmt.Errorf("wrong source for LOCAL")
	}
	d := catalog.Details{CreditsNote: "LOCAL reads file names and folders. Contributor credits are not available; folder names are not verified credits."}
	if subject.Kind == "artists" {
		d.Title = subject.ID
		return d, nil
	}
	file, err := Resolve(b.Root, subject.ID)
	if err != nil {
		return d, err
	}
	info, err := os.Stat(file)
	if err != nil {
		return d, err
	}
	id := filepath.ToSlash(filepath.Clean(filepath.FromSlash(subject.ID)))
	folder := id
	if !info.IsDir() {
		if !Supported(file) {
			return d, fmt.Errorf("unsupported local work")
		}
		t := localTrack(id)
		d.Title, d.Subtitle = t.Title, t.Artist
		folder = filepath.ToSlash(filepath.Dir(filepath.FromSlash(id)))
		if folder != "." {
			q := catalog.Query{Source: music.Local, Page: "detail", Section: "songs", Kind: "albums", ID: folder}
			d.Links = append(d.Links, catalog.Item{Source: music.Local, ID: folder, Kind: "albums", Title: t.Album, Open: &q, Play: &q})
		}
	} else {
		d.Title = info.Name()
	}
	if parent := filepath.Dir(filepath.FromSlash(folder)); parent != "." {
		artist := filepath.Base(parent)
		q := catalog.Query{Source: music.Local, Page: "detail", Section: "albums", Kind: "artists", ID: artist}
		d.Links = append(d.Links, catalog.Item{Source: music.Local, ID: artist, Kind: "artists", Title: artist, Open: &q})
	}
	return d, nil
}
