//go:build linux || darwin

package browserless

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"lazymusic/internal/vibez/player/browserless/mp4"
	videoframe "lazymusic/internal/video"
)

// selectVideoVariant respects the manifest's software playback policy. Hardware
// protected/HDCP variants are never rewritten or passed to this decoder.
func selectVideoVariant(master []byte, base string) (video, audio string, err error) {
	lines := strings.Split(string(master), "\n")
	group, best := "", -int(^uint(0)>>1)
	for i, line := range lines {
		if !strings.HasPrefix(line, "#EXT-X-STREAM-INF:") || i+1 >= len(lines) {
			continue
		}
		a := hlsAttributes(line)
		if !strings.Contains(a["CODECS"], "avc1.") || a["HDCP-LEVEL"] != "NONE" || !strings.Contains(a["ALLOWED-CPC"], "WIDEVINE_SOFTWARE") {
			continue
		}
		dimensions := strings.Split(a["RESOLUTION"], "x")
		if len(dimensions) != 2 {
			continue
		}
		width, _ := strconv.Atoi(dimensions[0])
		height, _ := strconv.Atoi(dimensions[1])
		if width < 1 || height < 1 || width > 1920 || height > 1080 {
			continue
		}
		// Keep the clearest software-authorized rendition up to the display
		// budget. Above that budget prefer the smallest available source.
		score := width * height
		if width > videoframe.MaxWidth || height > videoframe.MaxHeight {
			score = -score
		}
		for _, codec := range strings.Split(a["CODECS"], ",") {
			if strings.TrimSpace(codec) == "mp4a.40.2" {
				score++
			}
		}
		if score <= best {
			continue
		}
		video, err = mediaURL(base, strings.TrimSpace(lines[i+1]))
		if err != nil {
			return "", "", err
		}
		group, best = a["AUDIO"], score
	}
	if video == "" {
		return "", "", fmt.Errorf("Apple returned no software-authorized H.264 video rendition")
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "#EXT-X-MEDIA:") {
			continue
		}
		a := hlsAttributes(line)
		if a["TYPE"] == "AUDIO" && a["GROUP-ID"] == group && a["URI"] != "" {
			audio, err = mediaURL(base, a["URI"])
			return video, audio, err
		}
	}
	return "", "", fmt.Errorf("video audio rendition is unavailable")
}

func (s *StreamServer) prepareVideo(ctx context.Context, id string) (string, time.Duration, error) {
	info, err := s.licClient.VideoPlaybackInfo(ctx, id)
	if err != nil {
		return "", 0, err
	}
	master, masterBase, err := s.videoManifest(ctx, info.HLSPlaylistURL)
	if err != nil {
		return "", 0, err
	}
	videoURI, audioURI, err := selectVideoVariant(master, masterBase)
	if err != nil {
		return "", 0, err
	}
	cert, err := s.licClient.FetchServerCertificate(ctx, info.WidevineCertURL)
	if err != nil {
		return "", 0, err
	}
	if err = s.cdmEngine.SetServerCertificate(cert); err != nil {
		return "", 0, err
	}
	streams := make([]*trackStream, 0, 2)
	for _, uri := range []string{videoURI, audioURI} {
		data, finalBase, err := s.videoManifest(ctx, uri)
		if err != nil {
			return "", 0, err
		}
		manifest, err := parseStationManifest(data, finalBase)
		if err != nil {
			return "", 0, err
		}
		if manifest.live || len(manifest.segments) > 10000 {
			return "", 0, fmt.Errorf("unsupported video timeline")
		}
		st := &trackStream{trackID: id, segments: manifest.segments, stationInfo: info, createdAt: time.Now()}
		if len(streams) == 1 {
			init, err := s.mediaBytes(ctx, manifest.init, manifest.initOffset, manifest.initLength, 4<<20)
			if err != nil {
				return "", 0, err
			}
			cfg, err := mp4.ParseAudioTrackConfig(init)
			if err != nil {
				return "", 0, err
			}
			st.fragmentConfig = &cfg
			st.audioCfg, err = mp4.ParseAudioConfig(init)
			if err != nil {
				return "", 0, err
			}
		}

		st.stationProtections = &stationProtectionStore{entries: map[string]stationProtectionData{}}

		for _, segment := range manifest.segments {
			st.totalDur += time.Duration(segment.duration * float64(time.Second))
		}
		streams = append(streams, st)
	}
	streams[0].video = true
	uri := s.register(ctx, fmt.Sprintf("video-%d", time.Now().UnixNano()), streams[0], ".m3u8")
	streams[1].videoStream = uri
	audio := s.register(ctx, fmt.Sprintf("video-audio-%d", time.Now().UnixNano()), streams[1], ".aac")
	return audio, streams[0].totalDur, nil
}

func (s *StreamServer) PrepareMedia(ctx context.Context, id, kind string, prefer256 bool) (string, time.Duration, error) {
	if kind == "video" {
		return s.prepareVideo(ctx, id)
	}
	return s.PrepareTrack(ctx, id, prefer256)
}
func (p *Player) VideoFrame(ctx context.Context, expected string) (videoframe.Frame, error) {
	p.mu.RLock()
	t := p.state.Track
	uri := p.curStreamURL
	position := p.state.Position
	if p.state.Playing && !p.state.Loading {
		position = p.seekOffset + p.sink.Position()
	}
	valid := t != nil && t.ID == expected && t.MediaType == "video" && uri != "" && !p.state.Loading
	p.mu.RUnlock()
	if !valid {
		return videoframe.Frame{}, nil
	}
	frame, err := p.streamer.VideoFrame(ctx, uri, position)
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.curStreamURL != uri || p.state.Track == nil || p.state.Track.ID != expected {
		return videoframe.Frame{}, nil
	}
	frame.ID = expected
	return frame, err
}

// Resolve playlist-relative references against the final response URL; Apple
// playlist redirects can change both the host and the media directory.
func (s *StreamServer) videoManifest(ctx context.Context, uri string) ([]byte, string, error) {
	return s.readMedia(ctx, uri, 0, 0, 4<<20)
}
