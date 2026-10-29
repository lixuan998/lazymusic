//go:build linux

package browserless

import "lazymusic/internal/vibez/player/gst"

func newAudioSink() (audioSink, error) {
	if err := gst.EnsureAACDecoder(); err != nil {
		return nil, err
	}
	return gst.New()
}
