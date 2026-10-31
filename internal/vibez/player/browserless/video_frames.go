//go:build linux || darwin || windows

package browserless

import (
	"context"
	"fmt"
	"time"

	"lazymusic/internal/vibez/player/browserless/cdm"
	"lazymusic/internal/vibez/player/browserless/mp4"
	"lazymusic/internal/video"
)

type nativeVideoState struct {
	key, init      string
	protection     *trackStream
	config         mp4.TrackConfig
	segment, index int
	samples        []mp4.EncryptedSample
	last           time.Duration
	frame          video.Frame
	future         cdm.VideoFrame
	idle           *time.Timer
	generation     uint64
	prefetch       *videoPrefetch
}

type videoPrefetch struct {
	index  int
	done   chan struct{}
	cancel context.CancelFunc
	data   []byte
	err    error
}

func (s *StreamServer) prefetchVideo(st *trackStream, index int) *videoPrefetch {
	if index >= len(st.segments) {
		return nil
	}
	parent := s.videoContext
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	p := &videoPrefetch{index: index, done: make(chan struct{}), cancel: cancel}
	seg := st.segments[index]
	go func() {
		defer cancel()
		p.data, p.err = s.mediaBytes(ctx, seg.uri, seg.offset, seg.length, 32<<20)
		close(p.done)
	}()
	return p
}

func (s *StreamServer) resetVideoLocked() {
	state := &s.videoState
	if state.idle != nil {
		state.idle.Stop()
	}
	if state.prefetch != nil {
		state.prefetch.cancel()
	}
	generation := state.generation + 1
	*state = nativeVideoState{generation: generation}
	if s.cdmEngine != nil {
		s.cdmEngine.ResetVideo()
	}
}

// VideoFrame advances the CDM's own video decoder against the audio clock.
// Video keys never go through raw Decrypt or an external decoding process.
// Only the current and next encrypted segments, a current picture and one future frame live
// in memory. Hidden frontends stop requesting frames and expire this state.
func (s *StreamServer) VideoFrame(ctx context.Context, audioURI string, position time.Duration) (result video.Frame, failure error) {
	s.videoMu.Lock()
	defer s.videoMu.Unlock()
	if s.videoClosed {
		return video.Frame{}, nil
	}
	request, cancel := context.WithCancel(ctx)
	defer cancel()
	if s.videoContext != nil {
		stop := context.AfterFunc(s.videoContext, cancel)
		defer stop()
	}
	ctx = request
	s.mu.RLock()
	audio := s.streams[streamIDFromURL(audioURI)]
	var st *trackStream
	if audio != nil {
		st = s.streams[streamIDFromURL(audio.videoStream)]
	}
	s.mu.RUnlock()
	if st == nil || !st.video {
		return video.Frame{}, nil
	}
	state := &s.videoState
	if state.key != audioURI || position+video.Interval < state.last || position > state.last+2*time.Second {
		s.resetVideoLocked()
		state = &s.videoState
		state.key = audioURI
		var elapsed time.Duration
		for state.segment+1 < len(st.segments) && elapsed+time.Duration(st.segments[state.segment].duration*float64(time.Second)) <= position {
			elapsed += time.Duration(st.segments[state.segment].duration * float64(time.Second))
			state.segment++
		}
	}
	if state.idle != nil {
		state.idle.Stop()
	}
	state.generation++
	generation := state.generation
	defer func() {
		if failure != nil {
			s.resetVideoLocked()
			return
		}
		state.last = position
		state.idle = time.AfterFunc(2*time.Second, func() {
			s.videoMu.Lock()
			defer s.videoMu.Unlock()
			if s.videoState.generation == generation {
				s.resetVideoLocked()
			}
		})
	}()
	chosen := cdm.VideoFrame{}
	for steps := 0; steps < 4096; steps++ {
		if err := ctx.Err(); err != nil {
			return video.Frame{}, err
		}
		if len(state.future.RGBA) > 0 {
			if time.Duration(state.future.Timestamp)*time.Microsecond <= position {
				chosen = state.future
				state.future = cdm.VideoFrame{}
			} else {
				break
			}
		}
		if state.index >= len(state.samples) {
			if len(state.samples) > 0 {
				state.segment++
				state.samples = nil
				state.index = 0
			}
			if state.segment >= len(st.segments) {
				break
			}
			seg := st.segments[state.segment]
			if state.init != seg.initURI {
				init, err := s.mediaBytes(ctx, seg.initURI, seg.initOffset, seg.initLength, 4<<20)
				if err != nil {
					return video.Frame{}, err
				}
				cfg, err := mp4.ParseVideoConfig(init)
				if err != nil {
					return video.Frame{}, err
				}
				extra := cfg.Extra
				if err = s.cdmEngine.ConfigureVideo(extra, cfg.Width, cfg.Height); err != nil {
					return video.Frame{}, err
				}
				state.config, state.init = cfg, seg.initURI
			}
			protection, err := s.stationSegmentProtection(ctx, st, seg)
			if err != nil {
				return video.Frame{}, err
			}
			if protection.encryption == nil || !protection.encryption.CBCS {
				return video.Frame{}, fmt.Errorf("this video does not offer supported CBCS playback")
			}
			var data []byte
			if p := state.prefetch; p != nil && p.index == state.segment {
				select {
				case <-ctx.Done():
					return video.Frame{}, ctx.Err()
				case <-p.done:
				}
				if p.err == nil {
					data = p.data
				}
				state.prefetch = nil
			}
			if data == nil {
				data, err = s.mediaBytes(ctx, seg.uri, seg.offset, seg.length, 32<<20)
			}
			if err != nil {
				return video.Frame{}, err
			}
			var start time.Duration
			for i := 0; i < state.segment; i++ {
				start += time.Duration(st.segments[i].duration * float64(time.Second))
			}
			samples, err := mp4.ParseTrackSamples(data, protection.kid, *protection.encryption, state.config, start)
			if err != nil {
				return video.Frame{}, err
			}
			state.samples, state.index, state.protection = samples, 0, protection
			state.prefetch = s.prefetchVideo(st, state.segment+1)
		}
		sample := state.samples[state.index]
		state.index++
		subs := make([]cdm.Subsample, len(sample.Subsamples))
		for i, sub := range sample.Subsamples {
			subs[i] = cdm.Subsample{ClearBytes: sub.ClearBytes, CipherBytes: sub.CipherBytes}
		}
		encryption := state.protection.encryption
		data, err := mp4.AVCFrameData(sample)
		if err != nil {
			return video.Frame{}, err
		}
		if state.index == 1 {
			header, err := mp4.AVCDecoderData(state.config.Extra)
			if err != nil {
				return video.Frame{}, err
			}
			data = append(header, data...)
			if len(subs) == 0 {
				subs = []cdm.Subsample{{ClearBytes: uint16(len(header)), CipherBytes: uint32(len(sample.Data))}}
			} else {
				if int(subs[0].ClearBytes)+len(header) > 65535 {
					return video.Frame{}, fmt.Errorf("AVC parameters exceed clear range")
				}
				subs[0].ClearBytes += uint16(len(header))
			}
		}
		decoded, err := s.cdmEngine.DecodeVideo(sample.KID, sample.IV, data, subs, encryption.Crypt, encryption.Skip, sample.Timestamp, (position - 100*time.Millisecond).Microseconds())
		if err != nil {
			return video.Frame{}, err
		}
		if len(decoded.RGBA) == 0 {
			continue
		}
		if time.Duration(decoded.Timestamp)*time.Microsecond <= position {
			chosen = decoded
		} else {
			state.future = decoded
			break
		}
	}
	if len(chosen.RGBA) == 0 && len(state.frame.PNG) == 0 {
		chosen = state.future
	}
	if len(chosen.RGBA) > 0 {
		frame, err := video.Encode(chosen.RGBA, chosen.Width, chosen.Height, time.Duration(chosen.Timestamp)*time.Microsecond)
		if err != nil {
			return video.Frame{}, err
		}
		state.frame = frame
	}
	return state.frame, nil
}
