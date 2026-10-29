package catalog

import "context"

// Subject identifies a work without exposing a provider URL to the UI. Kind is
// optional for tracks restored from a queue; the source resolves its identity.
type Subject struct {
	Source     string `json:"source"`
	ID         string `json:"id"`
	Kind       string `json:"kind,omitempty"`
	PlaybackID string `json:"playback_id,omitempty"`
	Credits    bool   `json:"credits,omitempty"`
}

type Credit struct {
	Group string   `json:"group,omitempty"`
	Name  string   `json:"name"`
	Roles []string `json:"roles,omitempty"`
}

type Details struct {
	Title       string   `json:"title"`
	Subtitle    string   `json:"subtitle,omitempty"`
	Links       []Item   `json:"links,omitempty"`
	Credits     []Credit `json:"credits,omitempty"`
	CreditsNote string   `json:"credits_note,omitempty"`
}

// Inspector is optional: sources without contributor metadata report that
// limitation rather than inventing artist identities or credits.
type Inspector interface {
	Inspect(context.Context, Subject) (Details, error)
}
