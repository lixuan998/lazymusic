//go:build linux || darwin

package browserless

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"lazymusic/internal/vibez/player/browserless/license"
	"lazymusic/internal/vibez/player/browserless/mp4"
)

var hlsFields = regexp.MustCompile(`(?:^|,)([A-Z0-9-]+)=(?:"([^"]*)"|([^,]*))`)

func hlsAttributes(line string) map[string]string {
	_, line, _ = strings.Cut(line, ":")
	out := map[string]string{}
	for _, m := range hlsFields.FindAllStringSubmatch(line, -1) {
		out[m[1]] = m[2]
		if m[2] == "" {
			out[m[1]] = m[3]
		}
	}
	return out
}
func mediaURL(base, reference string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid station media base")
	}
	r, err := url.Parse(reference)
	if err != nil {
		return "", fmt.Errorf("invalid station media reference")
	}
	u = u.ResolveReference(r)
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost")) {
		return "", fmt.Errorf("station media requires HTTPS")
	}
	if u.User != nil {
		return "", fmt.Errorf("invalid station media credentials")
	}
	return u.String(), nil
}
func mediaBytes(ctx context.Context, uri string, offset, length int64, limit int64) ([]byte, error) {
	if _, err := mediaURL(uri, ""); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	if length > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("station media network request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return nil, fmt.Errorf("station media HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("station media exceeds size limit")
	}
	return data, nil
}
func selectStationVariant(master []byte, base string, prefer256 bool) (string, error) {
	target := 256000
	if !prefer256 {
		target = 64000
	}
	best := ""
	distance := int(^uint(0) >> 1)
	bandwidth := 0
	for _, line := range strings.Split(string(master), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			a := hlsAttributes(line)
			bandwidth, _ = strconv.Atoi(a["BANDWIDTH"])
			continue
		}
		if line != "" && !strings.HasPrefix(line, "#") && bandwidth > 0 {
			d := bandwidth - target
			if d < 0 {
				d = -d
			}
			if d < distance {
				u, err := mediaURL(base, line)
				if err != nil {
					return "", err
				}
				best, distance = u, d
			}
			bandwidth = 0
		}
	}
	if best == "" {
		if strings.Contains(string(master), "#EXTINF:") {
			return base, nil
		}
		return "", fmt.Errorf("station has no audio variant")
	}
	return best, nil
}

type stationManifest struct {
	segments                         []segmentRef
	key, init                        string
	initOffset, initLength, sequence int64
	live                             bool
}

func parseStationManifest(data []byte, base string) (stationManifest, error) {
	m := stationManifest{live: !strings.Contains(string(data), "#EXT-X-ENDLIST")}
	duration := 0.
	var offset, length, nextOffset int64
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"):
			m.sequence, _ = strconv.ParseInt(strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"), 10, 64)
		case strings.HasPrefix(line, "#EXT-X-KEY:"):
			a := hlsAttributes(line)
			if a["KEYFORMAT"] == "urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed" {
				m.key = a["URI"]
			}
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			a := hlsAttributes(line)
			u, err := mediaURL(base, a["URI"])
			if err != nil {
				return m, err
			}
			m.init = u
			if r := a["BYTERANGE"]; r != "" {
				m.initLength, m.initOffset = parseStationRange(r, 0)
			}
		case strings.HasPrefix(line, "#EXTINF:"):
			v, _, _ := strings.Cut(strings.TrimPrefix(line, "#EXTINF:"), ",")
			duration, _ = strconv.ParseFloat(v, 64)
		case strings.HasPrefix(line, "#EXT-X-BYTERANGE:"):
			length, offset = parseStationRange(strings.TrimPrefix(line, "#EXT-X-BYTERANGE:"), nextOffset)
		case line != "" && !strings.HasPrefix(line, "#"):
			u, err := mediaURL(base, line)
			if err != nil {
				return m, err
			}
			m.segments = append(m.segments, segmentRef{keyURI: m.key, initURI: m.init, initOffset: m.initOffset, initLength: m.initLength, uri: u, offset: offset, length: length, duration: duration})
			nextOffset = offset + length
			offset, length = 0, 0
		}
	}
	if len(m.segments) == 0 || m.init == "" || m.key == "" {
		return m, fmt.Errorf("station stream is missing audio or Widevine metadata")
	}
	for _, seg := range m.segments {
		if seg.keyURI == "" || seg.initURI == "" {
			return m, fmt.Errorf("station segment is missing protection metadata")
		}
	}
	m.key, m.init, m.initOffset, m.initLength = m.segments[0].keyURI, m.segments[0].initURI, m.segments[0].initOffset, m.segments[0].initLength
	return m, nil
}
func parseStationRange(raw string, next int64) (int64, int64) {
	size, start, ok := strings.Cut(raw, "@")
	n, _ := strconv.ParseInt(size, 10, 64)
	if !ok {
		return n, next
	}
	offset, _ := strconv.ParseInt(start, 10, 64)
	return n, offset
}

func (s *StreamServer) prepareStation(ctx context.Context, id string, prefer256 bool) (string, time.Duration, error) {
	info, err := s.licClient.StationPlaybackInfo(ctx, id)
	if err != nil {
		return "", 0, err
	}
	master, err := mediaBytes(ctx, info.HLSPlaylistURL, 0, 0, 4<<20)
	if err != nil {
		return "", 0, err
	}
	uri, err := selectStationVariant(master, info.HLSPlaylistURL, prefer256)
	if err != nil {
		return "", 0, err
	}
	data := master
	if uri != info.HLSPlaylistURL {
		data, err = mediaBytes(ctx, uri, 0, 0, 4<<20)
		if err != nil {
			return "", 0, err
		}
	}
	m, err := parseStationManifest(data, uri)
	if err != nil {
		return "", 0, err
	}
	s.mu.Lock()
	if s.serverCert == nil {
		cert, err := s.licClient.FetchServerCertificate(ctx, info.WidevineCertURL)
		if err != nil {
			s.mu.Unlock()
			return "", 0, err
		}
		if err = s.cdmEngine.SetServerCertificate(cert); err != nil {
			s.mu.Unlock()
			return "", 0, err
		}
		s.serverCert = cert
	}
	s.mu.Unlock()
	enc, cfg, kid, err := s.stationProtection(ctx, m, info, id)
	if err != nil {
		return "", 0, err
	}
	st := &trackStream{trackID: id, kid: kid, audioCfg: cfg, segments: m.segments, encryption: enc, stationInfo: info, createdAt: time.Now(), stationURI: uri, live: m.live, mediaSequence: m.sequence, keyURI: m.key, initURI: m.init}
	st.stationProtections = &stationProtectionStore{entries: map[string]stationProtectionData{protectionKey(m.key, m.init, m.initOffset, m.initLength): {encryption: enc, cfg: cfg, kid: kid}}}
	var seconds float64
	for _, seg := range m.segments {
		seconds += seg.duration
	}
	if !m.live {
		st.totalDur = time.Duration(seconds * float64(time.Second))
	}
	if m.live && len(st.segments) > 3 {
		skip := len(st.segments) - 3
		st.segments = st.segments[skip:]
		st.mediaSequence += int64(skip)
	}
	return s.register(ctx, fmt.Sprintf("station-%d", time.Now().UnixNano()), st, ".aac"), st.totalDur, nil
}
func encryptionOptions(st *trackStream) []mp4.Encryption {
	if st.encryption == nil {
		return nil
	}
	return []mp4.Encryption{*st.encryption}
}

// Live readers follow the moving media window, keeping only one playlist in
// memory. Each reader owns its cursor/protection state; seek/skip cancellation
// cannot mutate another reader's segment list or license selection.
func (s *StreamServer) serveLive(w http.ResponseWriter, r *http.Request, id string, initial *trackStream) {
	ctx := r.Context()
	current := initial
	next := initial.mediaSequence
	w.Header().Set("Content-Type", "audio/aac")
	w.Header().Set("Cache-Control", "no-store")
	flusher, _ := w.(http.Flusher)
	fail := func(err error) {
		if ctx.Err() == nil && s.onFail != nil {
			s.onFail(id, fmt.Errorf("live station stream: %w", err))
		}
		if ctx.Err() == nil {
			panic(http.ErrAbortHandler)
		}
	}
	for {
		for i, seg := range current.segments {
			sequence := current.mediaSequence + int64(i)
			if sequence < next {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			data, _, err := s.fetchAndDecryptSegment(ctx, &http.Client{Timeout: 15 * time.Second}, current, seg)
			if err != nil {
				fail(err)
				return
			}
			if _, err = w.Write(data); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
			next = sequence + 1
		}
		if !current.live {
			return
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		data, err := mediaBytes(ctx, initial.stationURI, 0, 0, 4<<20)
		if err != nil {
			fail(err)
			return
		}
		m, err := parseStationManifest(data, initial.stationURI)
		if err != nil {
			fail(err)
			return
		}
		current = &trackStream{raw: initial.raw, stationInfo: initial.stationInfo, stationProtections: initial.stationProtections, trackID: initial.trackID, kid: current.kid, audioCfg: current.audioCfg, encryption: current.encryption, segments: m.segments, live: m.live, mediaSequence: m.sequence, keyURI: m.key, initURI: m.init}
		if next < m.sequence {
			next = m.sequence
		}
	}
}

func (s *StreamServer) stationProtection(ctx context.Context, m stationManifest, info *license.PlaybackInfo, id string) (*mp4.Encryption, mp4.AudioConfig, []byte, error) {
	init, err := mediaBytes(ctx, m.init, m.initOffset, m.initLength, 4<<20)
	if err != nil {
		return nil, mp4.AudioConfig{}, nil, err
	}
	enc, err := mp4.ParseEncryption(init)
	if err != nil {
		return nil, mp4.AudioConfig{}, nil, err
	}
	_, encoded, ok := strings.Cut(m.key, ",")
	if !ok {
		return nil, mp4.AudioConfig{}, nil, fmt.Errorf("invalid station protection header")
	}
	pssh, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(pssh) > 65536 {
		return nil, mp4.AudioConfig{}, nil, fmt.Errorf("invalid station protection header")
	}
	kid, err := mp4.WidevineKID(pssh)
	if err != nil {
		return nil, mp4.AudioConfig{}, nil, err
	}
	challenge, sid, err := s.cdmEngine.GenerateChallenge(pssh)
	if err != nil {
		return nil, mp4.AudioConfig{}, nil, err
	}
	license, err := s.licClient.AcquireLicense(ctx, info.HLSKeyServerURL, challenge, m.key, id)
	if err != nil {
		return nil, mp4.AudioConfig{}, nil, err
	}
	if err = s.cdmEngine.UpdateSession(sid, license); err != nil {
		return nil, mp4.AudioConfig{}, nil, err
	}
	cfg, err := mp4.ParseAudioConfig(init)
	if err != nil {
		return nil, mp4.AudioConfig{}, nil, err
	}
	return &enc, cfg, kid, nil
}

type stationProtectionData struct {
	encryption *mp4.Encryption
	cfg        mp4.AudioConfig
	kid        []byte
}
type stationProtectionStore struct {
	mu      sync.Mutex
	entries map[string]stationProtectionData
}

func protectionKey(key, init string, offset, length int64) string {
	return fmt.Sprintf("%s\x00%s\x00%d/%d", key, init, offset, length)
}
func (s *StreamServer) stationSegmentProtection(ctx context.Context, st *trackStream, seg segmentRef) (*trackStream, error) {
	store := st.stationProtections
	if store == nil {
		return nil, fmt.Errorf("station protection cache is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	key := protectionKey(seg.keyURI, seg.initURI, seg.initOffset, seg.initLength)
	data, ok := store.entries[key]
	if !ok {
		m := stationManifest{key: seg.keyURI, init: seg.initURI, initOffset: seg.initOffset, initLength: seg.initLength}
		enc, cfg, kid, err := s.stationProtection(ctx, m, st.stationInfo, st.trackID)
		if err != nil {
			return nil, err
		}
		data = stationProtectionData{encryption: enc, cfg: cfg, kid: kid}
		if len(store.entries) >= 16 {
			store.entries = map[string]stationProtectionData{}
		}
		store.entries[key] = data
	}
	return &trackStream{raw: st.raw, baseURI: st.baseURI, audioCfg: data.cfg, encryption: data.encryption, kid: data.kid}, nil
}
