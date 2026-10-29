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
	Source                 string `json:"Source,omitempty"`
	ID                     string
	PlaybackID             string `json:"CatalogID,omitempty"`
	Title, Artist, Album   string
	Duration               time.Duration
	ArtworkURL, PreviewURL string
	Genres, AudioVariants  []string
}

type State struct {
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
	Equalizer     bool `json:"equalizer"`
	Home          bool `json:"home"`
	CatalogSearch bool `json:"catalog_search"`
	Library       bool `json:"library"`
	Playlists     bool `json:"playlists"`
	Folders       bool `json:"folders"`
}

type Quality struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Bitrate int    `json:"bitrate,omitempty"`
}
type Source struct {
	QualityNote    string       `json:"quality_note,omitempty"`
	Qualities      []Quality    `json:"qualities,omitempty"`
	DefaultQuality string       `json:"default_quality,omitempty"`
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	RequiresLogin  bool         `json:"requires_login"`
	Capabilities   Capabilities `json:"capabilities"`
}
