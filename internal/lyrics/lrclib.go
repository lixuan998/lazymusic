package lyrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"lazymusic/internal/music"
)

var lookupGate struct {
	sync.Mutex
	next, retryAfter time.Time
}

// Lookup uses an exact public metadata lookup, never a fuzzy search. The client
// is fresh: Apple authorization headers, cookies and user tokens cannot follow.
func Lookup(ctx context.Context, track music.Track, transport http.RoundTripper) (Document, error) {
	if strings.TrimSpace(track.Title) == "" || strings.TrimSpace(track.Artist) == "" || track.Duration <= 0 {
		return Document{}, nil
	}
	lookupGate.Lock()
	defer lookupGate.Unlock()
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	now := time.Now()
	if now.Before(lookupGate.retryAfter) {
		return Document{}, fmt.Errorf("lyrics service is temporarily rate limited")
	}
	if delay := time.Until(lookupGate.next); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return Document{}, ctx.Err()
		case <-timer.C:
		}
	}
	defer func() { lookupGate.next = time.Now().Add(250 * time.Millisecond) }()
	values := url.Values{"track_name": {track.Title}, "artist_name": {track.Artist}, "album_name": {track.Album}, "duration": {strconv.FormatInt(int64(math.Round(track.Duration.Seconds())), 10)}}
	for _, value := range values {
		if len(value[0]) > 1024 {
			return Document{}, nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://lrclib.net/api/get?"+values.Encode(), nil)
	if err != nil {
		return Document{}, fmt.Errorf("invalid lyrics lookup")
	}
	req.Header.Set("User-Agent", "lazymusic dev (https://github.com/lixuan998/lazymusic)")
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Document{}, ctx.Err()
		}
		return Document{}, fmt.Errorf("lyrics lookup unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return Document{}, nil
	}
	if response.StatusCode == http.StatusTooManyRequests {
		until := time.Now().Add(time.Minute)
		value := response.Header.Get("Retry-After")
		if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
			until = time.Now().Add(time.Duration(seconds) * time.Second)
		} else if date, err := http.ParseTime(value); err == nil && date.After(time.Now()) {
			until = date
		}
		lookupGate.retryAfter = until
	}
	if response.StatusCode != http.StatusOK {
		return Document{}, fmt.Errorf("lyrics service HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxBytes+1))
	if err != nil || len(data) > MaxBytes {
		return Document{}, fmt.Errorf("lyrics response exceeds the supported size")
	}
	var result struct {
		TrackName, ArtistName, AlbumName, SyncedLyrics string
		Duration                                       float64
		Instrumental                                   bool
	}
	if err = json.Unmarshal(data, &result); err != nil {
		return Document{}, fmt.Errorf("invalid lyrics response")
	}
	// Do not use a different performance or remix with a similar title.
	if result.Instrumental || result.SyncedLyrics == "" || !strings.EqualFold(strings.TrimSpace(result.TrackName), strings.TrimSpace(track.Title)) || !strings.EqualFold(strings.TrimSpace(result.ArtistName), strings.TrimSpace(track.Artist)) || track.Album != "" && !strings.EqualFold(strings.TrimSpace(result.AlbumName), strings.TrimSpace(track.Album)) || math.Abs(result.Duration-track.Duration.Seconds()) > 2 {
		return Document{}, nil
	}
	doc, err := ParseLRC(result.SyncedLyrics)
	doc.Provider = "LRCLIB"
	return doc, err
}
