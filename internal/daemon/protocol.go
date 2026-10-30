package daemon

import (
	"time"

	"lazymusic/internal/catalog"
	"lazymusic/internal/eventlog"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
	"lazymusic/internal/volume"
)

const ProtocolVersion = 12

type State struct {
	Placement        *music.PlacementStatus `json:"placement,omitempty"`
	Equalizer        music.Equalizer        `json:"equalizer"`
	PreferredQuality string                 `json:"preferred_quality,omitempty"`
	Logs             []eventlog.Entry       `json:"logs,omitempty"`
	SystemVolume     volume.State           `json:"system_volume"`
	Source           music.Source           `json:"source"`
	ObservedAt       time.Time              `json:"observed_at"`
	Protocol         int                    `json:"protocol"`
	PID              int                    `json:"pid"`
	Backend          string                 `json:"backend"`
	StartedAt        time.Time              `json:"started_at"`
	Playback         music.State            `json:"playback"`
	Queue            []music.Track          `json:"queue"`
	Index            int                    `json:"index"`
	UI               session.UI             `json:"ui"`
	EngineReady      bool                   `json:"engine_ready"`
	Error            string                 `json:"error,omitempty"`
}

type Command struct {
	Placement         bool           `json:"placement,omitempty"`
	PlacementID       string         `json:"placement_id,omitempty"`
	PlacementRevision uint64         `json:"placement_revision,omitempty"`
	ExpectedQueue     string         `json:"expected_queue,omitempty"`
	EQBand            int            `json:"eq_band,omitempty"`
	EQDelta           float64        `json:"eq_delta,omitempty"`
	EQAction          string         `json:"eq_action,omitempty"`
	EQPreset          string         `json:"eq_preset,omitempty"`
	Quality           string         `json:"quality,omitempty"`
	ModeStep          int            `json:"mode_step,omitempty"`
	Op                string         `json:"op"`
	Tracks            []music.Track  `json:"tracks,omitempty"`
	Query             *catalog.Query `json:"query,omitempty"`
	IDs               []string       `json:"ids,omitempty"`
	StartID           string         `json:"start_id,omitempty"`
	Index             int            `json:"index,omitempty"`
	To                int            `json:"to,omitempty"`
	Position          time.Duration  `json:"position_ns,omitempty"`
	Delta             time.Duration  `json:"delta_ns,omitempty"`
	Volume            float64        `json:"volume,omitempty"`
}

type UIUpdate struct {
	UI       session.UI `json:"ui"`
	Revision uint64     `json:"revision"`
}
type AttachRequest struct {
	PID int `json:"pid"`
}
type Attachment struct {
	Token string `json:"token"`
	State State  `json:"state"`
}
type APIError struct {
	Error string `json:"error"`
}
