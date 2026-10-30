// Package music contains backend-independent identities and playback metadata.
package music

import "time"

const (
	Apple         = "apple"
	Local         = "local"
	RepeatModeOff = 0
	RepeatModeOne = 1
	RepeatModeAll = 2
)

// Source and ID form an identity. PlaybackID is opaque to the UI; the JSON name
// retains compatibility with pre-P4 Apple session files.
type Track struct {
	Kind, MediaType        string
	Source                 string `json:"Source,omitempty"`
	ID                     string
	PlaybackID             string `json:"CatalogID,omitempty"`
	Title, Artist, Album   string
	Duration               time.Duration
	ArtworkURL, PreviewURL string
	Genres, AudioVariants  []string
}

type State struct {
	Continuous          *Query `json:"continuous,omitempty"`
	Status              string `json:"Status,omitempty"`
	Track               *Track
	Playing, Loading    bool
	Position            time.Duration
	Volume              float64
	Bitrate, RepeatMode int
	ShuffleMode         bool
	Error               string
	Logs                []string
	SkippedID           string
}

type Capabilities struct {
	PlaylistCreate bool `json:"playlist_create"`
	Equalizer      bool `json:"equalizer"`
	Home           bool `json:"home"`
	CatalogSearch  bool `json:"catalog_search"`
	Library        bool `json:"library"`
	Playlists      bool `json:"playlists"`
	Folders        bool `json:"folders"`
}

type Quality struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Bitrate int    `json:"bitrate,omitempty"`
}
type Source struct {
	Pages          []SourcePage `json:"pages,omitempty"`
	QualityNote    string       `json:"quality_note,omitempty"`
	Qualities      []Quality    `json:"qualities,omitempty"`
	DefaultQuality string       `json:"default_quality,omitempty"`
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	RequiresLogin  bool         `json:"requires_login"`
	Capabilities   Capabilities `json:"capabilities"`
}

// SourcePage adds a source-defined root without platform checks in the TUI.
type SourcePage struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Query Query  `json:"query"`
}
