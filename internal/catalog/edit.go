package catalog

import "context"

const (
	AddLibrary     = "add-library"
	CreatePlaylist = "create-playlist"
	AddPlaylist    = "add-playlist"
	RemoveLibrary  = "remove-library"
	DeletePlaylist = "delete-playlist"
)

// Actions describes operations on an actual backend identity. Queries remain
// opaque to the frontend, including the list of writable destination playlists.
type Actions struct {
	AddLibrary  bool   `json:"add_library"`
	Playlists   *Query `json:"playlists,omitempty"`
	Remove      string `json:"remove,omitempty"`
	RemovalNote string `json:"removal_note,omitempty"`
}

type Mutation struct {
	Source     string  `json:"source"`
	Op         string  `json:"op"`
	Subject    Subject `json:"subject"`
	Name       string  `json:"name,omitempty"`
	PlaylistID string  `json:"playlist_id,omitempty"`
}

type MutationResult struct {
	Message string `json:"message"`
	Pending bool   `json:"pending,omitempty"`
}

// Editor is optional. Writes must not be retried automatically: a failed reply
// can still mean the server accepted a non-idempotent operation.
type Editor interface {
	Actions(context.Context, Subject) (Actions, error)
	Mutate(context.Context, Mutation) (MutationResult, error)
}
