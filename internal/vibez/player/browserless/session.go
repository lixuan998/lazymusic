// lazymusic additions: restore and enqueue metadata without starting audio.
//go:build linux || darwin

package browserless

import (
	"fmt"
	"time"

	"lazymusic/internal/vibez/provider"
)

// SeekStream preserves the requested position to one AAC frame instead of
// rewinding to the beginning of an HLS segment. Raw uploads retain their
// upstream segment-based behavior until their formats have been verified.
func (s *StreamServer) SeekStream(id, base string, target time.Duration) (string, time.Duration, bool) {
	segment, offset, ok := s.SeekTarget(id, target)
	if !ok {
		return "", 0, false
	}
	uri := fmt.Sprintf("%s?start_seg=%d", base, segment)
	s.mu.RLock()
	st := s.streams[id]
	s.mu.RUnlock()
	if st == nil {
		return "", 0, false
	}
	rates := []int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}
	if st.raw || st.audioCfg.FreqIndex < 0 || st.audioCfg.FreqIndex >= len(rates) {
		return uri, offset, true
	}
	rate := rates[st.audioCfg.FreqIndex]
	frames := int(max(0, target-offset).Seconds() * float64(rate) / 1024)
	if frames > 0 {
		uri += fmt.Sprintf("&skip_frames=%d", frames)
		offset += time.Duration(float64(frames) * 1024 / float64(rate) * float64(time.Second))
	}
	return uri, offset, true
}

func trimADTSFrames(data []byte, frames int) ([]byte, int, error) {
	for frames > 0 && len(data) > 0 {
		if len(data) < 7 || data[0] != 0xff || data[1]&0xf6 != 0xf0 {
			return nil, frames, fmt.Errorf("invalid ADTS header while seeking")
		}
		size := int(data[3]&3)<<11 | int(data[4])<<3 | int(data[5]>>5)
		if size < 7 || size > len(data) {
			return nil, frames, fmt.Errorf("truncated ADTS frame while seeking")
		}
		if data[6]&3 != 0 {
			return nil, frames, fmt.Errorf("multiple AAC blocks in one ADTS frame are not supported for precise seeking")
		}
		data = data[size:]
		frames--
	}
	return data, frames, nil
}

func (p *Player) RestoreTracks(tracks []provider.Track, index int, position time.Duration) {
	p.mu.Lock()
	if p.playCtx != nil {
		p.playCtx()
		p.playCtx = nil
	}
	p.cancelPrefetchLocked()
	p.stagedPreps = nil
	p.queue = append([]provider.Track(nil), tracks...)
	p.idx = max(0, min(index, len(tracks)-1))
	p.curStream = ""
	p.curStreamURL = ""
	p.streamErr = ""
	p.ended = false
	p.seekOffset = 0
	p.state.Playing = false
	p.state.Loading = false
	p.state.Error = ""
	p.state.Position = max(0, position)
	p.state.Track = nil
	p.resumePosition = 0
	p.resumeTrackID = ""
	if len(tracks) > 0 {
		t := tracks[p.idx]
		p.state.Track = &t
		if t.Duration > 0 {
			p.state.Position = min(p.state.Position, max(0, t.Duration-time.Second))
		}
		p.resumePosition = p.state.Position
		p.resumeTrackID = t.ID
	}
	s := p.state
	p.mu.Unlock()
	if p.sink != nil {
		p.sink.Stop()
	}
	p.bcast.Send(s)
}

func (p *Player) AppendTracks(tracks []provider.Track, next bool) {
	p.mu.Lock()
	if next && len(p.queue) > 0 {
		i := p.idx + 1
		p.queue = append(append(append([]provider.Track{}, p.queue[:i]...), tracks...), p.queue[i:]...)
	} else {
		p.queue = append(p.queue, tracks...)
	}
	if p.state.Track == nil && len(p.queue) > 0 {
		p.idx = 0
		t := p.queue[0]
		p.state.Track = &t
	}
	s := p.state
	p.mu.Unlock()
	p.bcast.Send(s)
	p.scheduleNextPrefetch()
}

// InsertTracks changes queue metadata atomically, retaining the active stream,
// current song and position even when insertion shifts its queue index.
func (p *Player) InsertTracks(tracks []provider.Track, at int) error {
	p.mu.Lock()
	if at < 0 || at > len(p.queue) {
		p.mu.Unlock()
		return fmt.Errorf("queue insertion index out of range")
	}
	p.cancelPrefetchLocked()
	if len(p.queue) > 0 && at <= p.idx {
		p.idx += len(tracks)
	}
	q := make([]provider.Track, 0, len(p.queue)+len(tracks))
	q = append(q, p.queue[:at]...)
	q = append(q, tracks...)
	p.queue = append(q, p.queue[at:]...)
	if p.state.Track == nil && len(p.queue) > 0 {
		p.idx = 0
		t := p.queue[0]
		p.state.Track = &t
	}
	s := p.state
	p.mu.Unlock()
	p.bcast.Send(s)
	p.scheduleNextPrefetch()
	return nil
}
