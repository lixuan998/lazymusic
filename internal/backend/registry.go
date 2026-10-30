// Package backend is the composition point for supported sources and players.
// Adding a backend does not require changes to the TUI or catalog contracts.
package backend

import (
	"context"
	"fmt"
	"lazymusic/internal/account"
	"lazymusic/internal/catalog"
	"lazymusic/internal/engine"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
	appleapi "lazymusic/internal/sources/apple"
	localfiles "lazymusic/internal/sources/local"
)

type Entry struct {
	Source music.Source
	Open   func(string) (catalog.Browser, []string, error)
	Engine func(context.Context, string, session.Playback) (engine.Engine, error)
}

var entries = []Entry{
	{Source: music.Source{
		ID: music.Apple, Name: "APPLE MUSIC", RequiresLogin: true,
		QualityNote:    "This native backend receives AAC streams. Lossless / Hi-Res requires a different playback and authorization path.",
		DefaultQuality: "high", Qualities: []music.Quality{{ID: "standard", Name: "AAC 64 kbps", Bitrate: 64}, {ID: "high", Name: "AAC 256 kbps", Bitrate: 256}},
		Capabilities: music.Capabilities{Equalizer: true, Home: true, CatalogSearch: true, Library: true, Playlists: true, PlaylistCreate: true},
		Pages: []music.SourcePage{
			{ID: "new", Title: "New", Query: music.Query{Source: music.Apple, Page: "new", Section: "songs"}},
			{ID: "radio", Title: "Radio", Query: music.Query{Source: music.Apple, Page: "radio", Section: "songs"}},
		},
	},
		Open: func(dir string) (catalog.Browser, []string, error) {
			cfg, err := account.ReadConfig(dir)
			if err != nil {
				return nil, nil, err
			}
			return appleapi.New(cfg), []string{cfg.AppleDeveloperToken, cfg.AppleUserToken}, nil
		}, Engine: engine.NewNative},
	{Source: music.Source{ID: music.Local, Name: "LOCAL", Capabilities: music.Capabilities{Equalizer: true, Library: true, Folders: true}},
		Open: func(dir string) (catalog.Browser, []string, error) { b, err := localfiles.New(dir); return b, nil, err }, Engine: engine.NewLocal},
}

func Sources() []music.Source {
	out := make([]music.Source, len(entries))
	for i, e := range entries {
		out[i] = e.Source
	}
	return out
}
func Lookup(id string) (Entry, error) {
	for _, e := range entries {
		if e.Source.ID == id {
			return e, nil
		}
	}
	return Entry{}, fmt.Errorf("unsupported source %q; supported: apple, local", id)
}
