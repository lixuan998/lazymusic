package daemon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"lazymusic/internal/catalog"
	"lazymusic/internal/video"
	"net/http"
	"strconv"
)

const videoMetadataHeader = "X-Lazymusic-Video"

// Keep the already compressed picture binary over the local socket. Encoding
// megabytes of PNG as JSON base64 on every frame wastes CPU and allocations.
func writeVideoFrame(w http.ResponseWriter, frame video.Frame) {
	w.Header().Set("Cache-Control", "no-store")
	if len(frame.PNG) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	metadata := frame
	metadata.PNG = nil
	data, _ := json.Marshal(metadata)
	w.Header().Set(videoMetadataHeader, base64.RawStdEncoding.EncodeToString(data))
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(frame.PNG)))
	w.Write(frame.PNG)
}

func (s *Service) videoFrame(ctx context.Context, subject catalog.Subject) (video.Frame, error) {
	if subject.ID == "" || subject.Source != s.source.ID {
		return video.Frame{}, fmt.Errorf("invalid video source or identity")
	}
	s.mu.Lock()
	e := s.engine
	stopping := s.stopping
	s.mu.Unlock()
	if e == nil || stopping {
		return video.Frame{}, nil
	}
	state, _, _ := e.State()
	if state.Track == nil || state.Track.ID != subject.ID || state.Track.MediaType != "video" {
		return video.Frame{}, nil
	}
	provider, ok := e.(video.Provider)
	if !ok {
		return video.Frame{}, nil
	}
	frame, err := provider.VideoFrame(ctx, subject.ID)
	if err != nil {
		return video.Frame{}, err
	}
	state, _, _ = e.State()
	if state.Track == nil || state.Track.ID != subject.ID || state.Track.MediaType != "video" {
		return video.Frame{}, nil
	}
	if len(frame.PNG) > 0 && (frame.Source != s.source.ID || frame.ID != subject.ID || !frame.Valid()) {
		return video.Frame{}, fmt.Errorf("backend returned an invalid video frame")
	}
	return frame, nil
}
