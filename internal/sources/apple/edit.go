package apple

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"lazymusic/internal/catalog"
	"lazymusic/internal/music"
)

const removalNote = "Apple Music's public API does not support deleting library songs, albums or playlists. Remove this item in the Apple Music app."

func subjectKind(s catalog.Subject) string {
	if s.Kind != "" {
		return s.Kind
	}
	if strings.HasPrefix(s.ID, "i.") {
		return "library-songs"
	}
	return "songs"
}

func validEditID(id string) bool {
	return id != "" && len(id) <= 512 && id != "." && id != ".." &&
		!strings.ContainsAny(id, "/\\?#,&=") && strings.IndexFunc(id, unicode.IsSpace) < 0 && strings.IndexFunc(id, unicode.IsControl) < 0
}

func (c *Client) Actions(_ context.Context, s catalog.Subject) (catalog.Actions, error) {
	if !validEditID(s.ID) || (s.Source != "" && s.Source != music.Apple) {
		return catalog.Actions{}, fmt.Errorf("invalid Apple Music work")
	}
	kind := subjectKind(s)
	a := catalog.Actions{}
	if kind == "songs" || kind == "albums" {
		a.AddLibrary = true
	}
	if kind == "songs" || kind == "library-songs" {
		a.Playlists = &catalog.Query{Source: music.Apple, Page: "playlists", Section: "songs"}
	}
	if kind == "library-songs" || kind == "library-albums" || kind == "library-playlists" {
		a.RemovalNote = removalNote
	}
	return a, nil
}

func (c *Client) Mutate(ctx context.Context, m catalog.Mutation) (catalog.MutationResult, error) {
	if (m.Source != "" && m.Source != music.Apple) || (m.Subject.Source != "" && m.Subject.Source != music.Apple) {
		return catalog.MutationResult{}, fmt.Errorf("mutation belongs to another music source")
	}
	var endpoint string
	var body any
	message := ""
	switch m.Op {
	case catalog.CreatePlaylist:
		name := strings.TrimSpace(m.Name)
		if name == "" || utf8.RuneCountInString(name) > 200 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			return catalog.MutationResult{}, fmt.Errorf("playlist name must contain 1–200 characters without control characters")
		}
		endpoint = "/me/library/playlists"
		body = map[string]any{"attributes": map[string]string{"name": name}}
		message = "Playlist created; refresh Playlists if it has not appeared yet"
	case catalog.AddLibrary:
		a, err := c.Actions(ctx, m.Subject)
		if err != nil {
			return catalog.MutationResult{}, err
		}
		if !a.AddLibrary {
			return catalog.MutationResult{}, fmt.Errorf("this work cannot be added to the library, or is already a library item")
		}
		query := url.Values{"ids[" + subjectKind(m.Subject) + "]": {m.Subject.ID}}
		endpoint = "/me/library?" + query.Encode()
		message = "Library addition accepted; Apple may take a moment to show it"
	case catalog.AddPlaylist:
		a, err := c.Actions(ctx, m.Subject)
		if err != nil {
			return catalog.MutationResult{}, err
		}
		if a.Playlists == nil || !validEditID(m.PlaylistID) {
			return catalog.MutationResult{}, fmt.Errorf("choose a song and an editable playlist")
		}
		// Recheck ownership at the write boundary, not just in the picker.
		var target collection
		if err := c.get(ctx, "/me/library/playlists/"+url.PathEscape(m.PlaylistID), &target); err != nil {
			return catalog.MutationResult{}, err
		}
		if len(target.Data) != 1 || target.Data[0].ID != m.PlaylistID || !target.Data[0].Attributes.CanEdit {
			return catalog.MutationResult{}, fmt.Errorf("this playlist is not editable")
		}
		endpoint = "/me/library/playlists/" + url.PathEscape(m.PlaylistID) + "/tracks"
		body = map[string]any{"data": []map[string]string{{"id": m.Subject.ID, "type": subjectKind(m.Subject)}}}
		message = "Song added to playlist"
	case catalog.RemoveLibrary, catalog.DeletePlaylist:
		return catalog.MutationResult{}, fmt.Errorf("%s", removalNote)
	default:
		return catalog.MutationResult{}, fmt.Errorf("unsupported catalog operation")
	}
	status, err := c.request(ctx, http.MethodPost, endpoint, body, nil)
	if err != nil {
		// A connection failure can happen after Apple committed the change.
		return catalog.MutationResult{}, fmt.Errorf("%w; check the library before retrying", err)
	}
	if status == http.StatusAccepted && m.Op != catalog.AddLibrary {
		message = "Change accepted; Apple may take a moment to show it"
	}
	return catalog.MutationResult{Message: message, Pending: status == http.StatusAccepted}, nil
}
