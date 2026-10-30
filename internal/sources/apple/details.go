package apple

import (
	"context"
	"fmt"
	"lazymusic/internal/catalog"
	"lazymusic/internal/music"
	"lazymusic/internal/vibez/provider"
	"net/url"
	"strings"
)

func (c *Client) Inspect(ctx context.Context, subject catalog.Subject) (catalog.Details, error) {
	if subject.Source != "" && subject.Source != music.Apple {
		return catalog.Details{}, fmt.Errorf("wrong source for Apple Music")
	}
	if subject.ID == "" {
		return catalog.Details{}, fmt.Errorf("work requires an ID")
	}
	kind, id := subject.Kind, subject.ID
	if kind == "" {
		kind = "songs"
		if strings.HasPrefix(id, "i.") {
			kind = "library-songs"
		}
	}
	if strings.HasSuffix(kind, "songs") && subject.PlaybackID != "" {
		kind, id = "songs", subject.PlaybackID
	}
	if kind != "music-videos" && kind != "songs" && kind != "library-songs" && kind != "albums" && kind != "library-albums" && kind != "artists" && kind != "library-artists" && kind != "playlists" && kind != "library-playlists" {
		return catalog.Details{CreditsNote: "Apple Music does not provide credits for this type of work."}, nil
	}
	sf, err := c.sf(ctx)
	if err != nil {
		return catalog.Details{}, err
	}
	r, err := c.inspectResource(ctx, sf, kind, id)
	if err != nil {
		return catalog.Details{}, err
	}
	// Library imports may have no catalog counterpart. Keep their actual library
	// relationships instead of searching by name and guessing the artist.
	if strings.HasPrefix(kind, "library-") {
		if refs := r.Relationships["catalog"].Data; len(refs) > 0 {
			kind, id = refs[0].Type, refs[0].ID
			r, err = c.inspectResource(ctx, sf, kind, id)
			if err != nil {
				return catalog.Details{}, err
			}
		}
	}
	d := catalog.Details{Title: provider.Clean(r.Attributes.Name), Subtitle: provider.Clean(r.Attributes.Artist)}
	for _, relationship := range []string{"albums", "artists"} {
		refs := r.Relationships[relationship]
		seen := map[string]bool{}
		identities := map[string]bool{}
		for page := 0; ; page++ {
			for _, ref := range refs.Data {
				if ref.ID == "" || identities[ref.Type+":"+ref.ID] {
					continue
				}
				identities[ref.Type+":"+ref.ID] = true
				if ref.Attributes.Name == "" {
					ref, err = c.inspectResource(ctx, sf, ref.Type, ref.ID)
					if err != nil {
						return catalog.Details{}, err
					}
				}
				items := convert(collection{Data: []resource{ref}}).Items
				if len(items) > 0 && items[0].Open != nil {
					d.Links = append(d.Links, items[0])
					if len(d.Links) > 1000 {
						return catalog.Details{}, fmt.Errorf("too many work relationships")
					}
				}
			}
			if refs.Next == "" {
				break
			}
			if page >= 19 || seen[refs.Next] {
				return catalog.Details{}, fmt.Errorf("too many or repeated work relationships")
			}
			seen[refs.Next] = true
			var next collection
			if err := c.get(ctx, refs.Next, &next); err != nil {
				return catalog.Details{}, err
			}
			refs = next
		}
	}
	if !subject.Credits {
		return d, nil
	}
	for _, group := range r.Relationships["credits"].Data {
		for _, person := range group.Relationships["credit-artists"].Data {
			if person.Attributes.Name != "" {
				d.Credits = append(d.Credits, catalog.Credit{Group: provider.Clean(group.Attributes.Title), Name: provider.Clean(person.Attributes.Name), Roles: person.Attributes.Roles})
			}
		}
	}
	if len(d.Credits) == 0 {
		if r.Attributes.Artist != "" {
			d.Credits = append(d.Credits, catalog.Credit{Group: "Artists", Name: provider.Clean(r.Attributes.Artist)})
		}
		if r.Attributes.Composer != "" {
			d.Credits = append(d.Credits, catalog.Credit{Group: "Composition", Name: provider.Clean(r.Attributes.Composer), Roles: []string{"Composer"}})
		}
		d.CreditsNote = "Only the metadata Apple Music supplied is shown. A complete contributor list is unavailable for this work."
	}
	if r.Attributes.Label != "" {
		d.Credits = append(d.Credits, catalog.Credit{Group: "Release", Name: provider.Clean(r.Attributes.Label), Roles: []string{"Record label"}})
	}
	if r.Attributes.Copyright != "" {
		d.Credits = append(d.Credits, catalog.Credit{Group: "Copyright", Name: provider.Clean(r.Attributes.Copyright)})
	}
	return d, nil
}

func (c *Client) inspectResource(ctx context.Context, sf, kind, id string) (resource, error) {
	if id == "" {
		return resource{}, fmt.Errorf("missing work identity")
	}
	var endpoint, include string
	if strings.HasPrefix(kind, "library-") {
		endpoint = "/me/library/" + url.PathEscape(strings.TrimPrefix(kind, "library-"))
		include = "catalog,albums,artists"
	} else {
		endpoint = "/catalog/" + url.PathEscape(sf) + "/" + url.PathEscape(kind)
		include = "albums,artists"
		if kind == "songs" {
			include += ",credits"
		}
	}
	var response collection
	if err := c.get(ctx, endpoint+"/"+url.PathEscape(id)+"?include="+url.QueryEscape(include), &response); err != nil {
		return resource{}, err
	}
	if len(response.Data) == 0 {
		return resource{}, fmt.Errorf("Apple Music returned no work metadata")
	}
	return response.Data[0], nil
}
