package daemon

import (
	"context"
	"fmt"

	"lazymusic/internal/catalog"
	"lazymusic/internal/lyrics"
)

type cachedLyrics struct {
	subject  catalog.Subject
	document lyrics.Document
}

func (s *Service) lyrics(ctx context.Context, subject catalog.Subject) (lyrics.Document, error) {
	if subject.ID == "" || subject.Source != "" && subject.Source != s.source.ID {
		return lyrics.Document{}, fmt.Errorf("invalid lyrics identity or source")
	}
	subject.Source, subject.Credits = s.source.ID, false
	s.mu.Lock()
	for _, entry := range s.lyricsCache {
		if entry.subject == subject {
			s.mu.Unlock()
			return entry.document, nil
		}
	}
	provider, ok := s.browser.(catalog.LyricsProvider)
	s.mu.Unlock()
	if !ok {
		return lyrics.Document{}, nil
	}
	document, err := provider.Lyrics(ctx, subject)
	if err != nil {
		return lyrics.Document{}, err
	}
	if !document.Valid() {
		return lyrics.Document{}, fmt.Errorf("source returned invalid lyrics")
	}
	s.mu.Lock()
	s.lyricsCache = append(s.lyricsCache, cachedLyrics{subject, document})
	if len(s.lyricsCache) > 16 {
		s.lyricsCache = s.lyricsCache[len(s.lyricsCache)-16:]
	}
	s.mu.Unlock()
	return document, nil
}
