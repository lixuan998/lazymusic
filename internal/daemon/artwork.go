package daemon

import (
	"context"
	"fmt"

	"lazymusic/internal/artwork"
	"lazymusic/internal/catalog"
)

type cachedArtwork struct {
	subject catalog.Subject
	image   artwork.Image
}

func (s *Service) artwork(ctx context.Context, subject catalog.Subject) (artwork.Image, error) {
	if subject.ID == "" || (subject.Source != "" && subject.Source != s.source.ID) {
		return artwork.Image{}, fmt.Errorf("invalid artwork identity or source")
	}
	subject.Source, subject.Credits = s.source.ID, false
	s.mu.Lock()
	for _, entry := range s.artworks {
		if entry.subject == subject {
			s.mu.Unlock()
			return entry.image, nil
		}
	}
	provider, ok := s.browser.(catalog.ArtworkProvider)
	s.mu.Unlock()
	if !ok {
		return artwork.Image{}, nil
	}
	im, err := provider.Artwork(ctx, subject)
	if err != nil {
		return artwork.Image{}, err
	}
	if len(im.RGB) != 0 && !im.Valid() {
		return artwork.Image{}, fmt.Errorf("source returned invalid album artwork")
	}
	s.mu.Lock()
	s.artworks = append(s.artworks, cachedArtwork{subject, im})
	if len(s.artworks) > 16 {
		s.artworks = s.artworks[len(s.artworks)-16:]
	}
	s.mu.Unlock()
	return im, nil
}
