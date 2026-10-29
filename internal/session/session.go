// Package session stores UI and playback metadata, never credentials or stream URLs.
package session

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"lazymusic/internal/music"
)

const Version = 2

type Location struct {
	Playback *music.Query `json:"playback,omitempty"`
	Source   string       `json:"source,omitempty"`
	Page     string       `json:"page"`
	Section  string       `json:"section"`
	ID       string       `json:"id,omitempty"`
	Kind     string       `json:"kind,omitempty"`
	Title    string       `json:"title,omitempty"`
	Scope    string       `json:"scope,omitempty"`
	Query    string       `json:"query,omitempty"`
}

type Cursor struct {
	Index  int    `json:"index"`
	Offset int    `json:"offset"`
	ID     string `json:"id,omitempty"`
}

type UI struct {
	NavWidth       int               `json:"nav_width,omitempty"`
	QueueWidth     int               `json:"queue_width,omitempty"`
	NavHeight      int               `json:"nav_height,omitempty"`
	PlayerHeight   int               `json:"player_height,omitempty"`
	LogHeight      int               `json:"log_height,omitempty"`
	ZenEQWidth     int               `json:"zen_eq_width,omitempty"`
	EQBand         int               `json:"eq_band,omitempty"`
	Zen            bool              `json:"zen,omitempty"`
	Layout         string            `json:"layout,omitempty"`
	ShowLogs       bool              `json:"show_logs,omitempty"`
	Visualizer     int               `json:"visualizer,omitempty"`
	HomeCategory   string            `json:"home_category,omitempty"`
	NavPage        string            `json:"nav_page,omitempty"`
	LastMainFocus  int               `json:"last_main_focus,omitempty"`
	LibrarySection string            `json:"library_section,omitempty"`
	SearchScope    string            `json:"search_scope,omitempty"`
	SearchSection  string            `json:"search_section,omitempty"`
	SearchQuery    string            `json:"search_query,omitempty"`
	Location       Location          `json:"location"`
	History        []Location        `json:"history,omitempty"`
	Focus          int               `json:"focus"`
	NavIndex       int               `json:"nav_index"`
	QueueCursor    int               `json:"queue_cursor"`
	Filter         string            `json:"filter,omitempty"`
	Cursors        map[string]Cursor `json:"cursors"`
}

func DefaultUI() UI {
	return UI{Location: Location{Page: "home", Section: "songs", Title: "Home"}, Focus: 1, Cursors: map[string]Cursor{}}
}

func (u UI) Clone() UI {
	v := u
	v.History = append([]Location(nil), u.History...)
	v.Cursors = make(map[string]Cursor, len(u.Cursors))
	for k, c := range u.Cursors {
		v.Cursors[k] = c
	}
	return v
}

type Playback struct {
	Equalizer  music.Equalizer `json:"equalizer"`
	Quality    string          `json:"quality,omitempty"`
	Queue      []music.Track   `json:"queue"`
	Index      int             `json:"index"`
	Position   time.Duration   `json:"position_ns"`
	Volume     float64         `json:"volume"`
	Repeat     int             `json:"repeat"`
	Shuffle    bool            `json:"shuffle"`
	WasPlaying bool            `json:"was_playing"`
}

type Snapshot struct {
	Version   int       `json:"version"`
	Backend   string    `json:"backend"`
	UpdatedAt time.Time `json:"updated_at"`
	Playback  Playback  `json:"playback"`
	UI        UI        `json:"ui"`
}

func CleanTrack(t music.Track) music.Track {
	// Artwork/preview URLs are not needed to restore a terminal session.
	t.ArtworkURL = ""
	t.PreviewURL = ""
	t.Genres = append([]string(nil), t.Genres...)
	t.AudioVariants = append([]string(nil), t.AudioVariants...)
	return t
}

func CleanTracks(tracks []music.Track) []music.Track {
	out := make([]music.Track, len(tracks))
	for i, t := range tracks {
		out[i] = CleanTrack(t)
	}
	return out
}

func Load(dir string) (Snapshot, error) {
	s := Snapshot{Version: Version, UI: DefaultUI(), Playback: Playback{Volume: 0.15}}
	f, err := os.Open(filepath.Join(dir, "session.json"))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	defer f.Close()
	if err = json.NewDecoder(io.LimitReader(f, 16<<20)).Decode(&s); err != nil {
		return s, fmt.Errorf("read session: %w", err)
	}
	if s.Version == 1 {
		s.Version = Version
		s.UI.NavIndex++
		if s.UI.NavIndex > 3 {
			s.UI.NavIndex = 3
		}
	}
	if s.Version != Version {
		return s, fmt.Errorf("unsupported session version %d", s.Version)
	}
	if len(s.Playback.Queue) > 10000 {
		return s, fmt.Errorf("saved queue exceeds 10000 tracks")
	}
	s.Normalize()
	return s, nil
}

func (s *Snapshot) Normalize() {
	if s.UI.SearchScope == "apple" {
		s.UI.SearchScope = "catalog"
	}
	if s.UI.Location.Scope == "apple" {
		s.UI.Location.Scope = "catalog"
	}
	for i := range s.UI.History {
		if s.UI.History[i].Scope == "apple" {
			s.UI.History[i].Scope = "catalog"
		}
	}
	if s.UI.Location.Page == "" {
		s.UI = DefaultUI()
	}
	if s.UI.Cursors == nil {
		s.UI.Cursors = map[string]Cursor{}
	}
	s.UI.Focus = max(0, min(5, s.UI.Focus))
	if s.UI.Layout != "content" && s.UI.Layout != "queue" {
		s.UI.Layout = "full"
	}
	if s.UI.Focus == 4 && !s.UI.ShowLogs {
		s.UI.Focus = 1
	}
	s.UI.Visualizer = max(0, min(2, s.UI.Visualizer))
	s.UI.NavIndex = max(0, min(32, s.UI.NavIndex))
	if len(s.UI.History) > 32 {
		s.UI.History = s.UI.History[len(s.UI.History)-32:]
	}
	if len(s.Playback.Queue) == 0 {
		s.Playback.Index = 0
		s.Playback.Position = 0
	} else {
		s.Playback.Index = max(0, min(len(s.Playback.Queue)-1, s.Playback.Index))
	}
	s.UI.EQBand = max(0, min(9, s.UI.EQBand))
	if err := s.Playback.Equalizer.Validate(); err != nil {
		s.Playback.Equalizer = music.Equalizer{}
	}
	s.Playback.Position = max(0, s.Playback.Position)
	s.Playback.Volume = max(0, min(1, s.Playback.Volume))
	s.Playback.Repeat = max(0, min(2, s.Playback.Repeat))
	s.Playback.Queue = CleanTracks(s.Playback.Queue)
	for i := range s.Playback.Queue {
		if s.Playback.Queue[i].Source == "" {
			s.Playback.Queue[i].Source = s.Backend
		}
	}
}

func Save(dir string, s Snapshot) error {
	s.Version = Version
	s.UpdatedAt = time.Now().UTC()
	s.Normalize()
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".session-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(append(b, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, filepath.Join(dir, "session.json")); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
