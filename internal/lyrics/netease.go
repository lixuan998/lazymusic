package lyrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"lazymusic/internal/music"
)

type lyricSong struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Duration int64  `json:"duration"`
	Artists  []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Album struct {
		Name string `json:"name"`
	} `json:"album"`
}

func publicLyricsJSON(ctx context.Context, client *http.Client, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://music.163.com"+path, nil)
	if err != nil {
		return fmt.Errorf("invalid public lyrics request")
	}
	req.Header.Set("User-Agent", "lazymusic dev (https://github.com/lixuan998/lazymusic)")
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("public lyrics lookup unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("public lyrics HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxBytes+1))
	if err != nil || len(data) > MaxBytes {
		return fmt.Errorf("public lyrics response exceeds the supported size")
	}
	if err = json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("invalid public lyrics response")
	}
	return nil
}

func lookupNetEase(ctx context.Context, track music.Track, transport http.RoundTripper) (Document, error) {
	if strings.TrimSpace(track.Title) == "" || strings.TrimSpace(track.Artist) == "" || track.Duration <= 0 || len(track.Title) > 1024 || len(track.Artist) > 1024 {
		return Document{}, nil
	}
	client := &http.Client{Transport: transport, Timeout: 7 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	query := url.Values{"s": {track.Title + " " + track.Artist}, "type": {"1"}, "limit": {"20"}, "offset": {"0"}}
	var result struct {
		Code   int `json:"code"`
		Result struct {
			Songs []lyricSong `json:"songs"`
		} `json:"result"`
	}
	if err := publicLyricsJSON(ctx, client, "/api/search/get?"+query.Encode(), &result); err != nil {
		return Document{}, err
	}
	if result.Code != 200 {
		return Document{}, fmt.Errorf("public lyrics search code %d", result.Code)
	}
	var matches []lyricSong
	for _, song := range result.Result.Songs {
		if song.ID <= 0 || !strings.EqualFold(strings.TrimSpace(song.Name), strings.TrimSpace(track.Title)) || math.Abs(float64(song.Duration)/1000-track.Duration.Seconds()) > 2 {
			continue
		}
		var names []string
		for _, artist := range song.Artists {
			names = append(names, artist.Name)
		}
		if artistKey(strings.Join(names, " & ")) != artistKey(track.Artist) {
			continue
		}
		matches = append(matches, song)
	}
	// Equal-title/artist/duration recordings can occur on singles and albums.
	// Prefer the same release, without rejecting a reissue of the same recording.
	sort.SliceStable(matches, func(i, j int) bool {
		return strings.EqualFold(matches[i].Album.Name, track.Album) && !strings.EqualFold(matches[j].Album.Name, track.Album)
	})
	for _, song := range matches[:min(3, len(matches))] {
		var response struct {
			Code    int  `json:"code"`
			NoLyric bool `json:"nolyric"`
			LRC     struct {
				Lyric string `json:"lyric"`
			} `json:"lrc"`
		}
		path := "/api/song/lyric?id=" + strconv.FormatInt(song.ID, 10) + "&lv=-1&kv=-1&tv=-1"
		if err := publicLyricsJSON(ctx, client, path, &response); err != nil {
			return Document{}, err
		}
		if response.Code != 200 {
			return Document{}, fmt.Errorf("public lyrics code %d", response.Code)
		}
		if response.NoLyric || response.LRC.Lyric == "" {
			continue
		}
		document, err := ParseLRC(response.LRC.Lyric)
		if err != nil {
			return Document{}, err
		}
		if len(document.Lines) > 0 {
			document.Provider = "NetEase Music"
			return document, nil
		}
	}
	return Document{}, nil
}

var artistSeparator = regexp.MustCompile(`(?i)\s*(?:[&,、;＆]|\s+feat\.?\s+|\s+featuring\s+|\s+with\s+|\s+and\s+)\s*`)

func artistKey(raw string) string {
	parts := artistSeparator.Split(raw, -1)
	var names []string
	for _, part := range parts {
		name := strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return unicode.ToLower(r)
			}
			return -1
		}, part)
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return strings.Join(names, "|")
}
