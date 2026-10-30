package mp4

import (
	"encoding/binary"
	"fmt"
	"time"
)

// EncryptedSample is an encrypted access unit, kept only in the playback process.
// Timestamp is the presentation time in microseconds expected by the CDM.
type EncryptedSample struct {
	Data, KID, IV []byte
	Subsamples    []Subsample
	Timestamp     int64
}
type TrackConfig struct {
	audio          bool
	TrackID        uint32
	Extra          []byte
	Width, Height  int
	Scale          int64
	Duration, Size int64
}

func ParseVideoConfig(data []byte) (TrackConfig, error)      { return parseTrackConfig(data, false) }
func ParseAudioTrackConfig(data []byte) (TrackConfig, error) { return parseTrackConfig(data, true) }
func parseTrackConfig(data []byte, audio bool) (TrackConfig, error) {
	var result TrackConfig
	defaults := map[uint32][2]int64{}
	depth := 0
	var walk func([]byte, *TrackConfig) error
	walk = func(data []byte, c *TrackConfig) error {
		depth++
		defer func() { depth-- }()
		if depth > 32 {
			return fmt.Errorf("media box nesting exceeds limit")
		}
		return eachBox(data, func(kind string, box []byte) error {
			switch kind {
			case "moov", "mdia", "minf", "stbl", "mvex":
				return walk(box[8:], c)
			case "trak":
				var track TrackConfig
				if err := walk(box[8:], &track); err != nil {
					return err
				}
				if audio && track.audio || !audio && len(track.Extra) > 0 && track.Width > 0 {
					result = track
				}
			case "tkhd":
				if len(box) < 12 {
					return fmt.Errorf("truncated track header")
				}
				offset := 20
				if box[8] == 1 {
					offset = 28
				}
				if offset+4 > len(box) {
					return fmt.Errorf("invalid track id")
				}
				c.TrackID = binary.BigEndian.Uint32(box[offset : offset+4])
			case "trex":
				if len(box) < 32 {
					return fmt.Errorf("invalid video defaults")
				}
				defaults[binary.BigEndian.Uint32(box[12:16])] = [2]int64{int64(binary.BigEndian.Uint32(box[20:24])), int64(binary.BigEndian.Uint32(box[24:28]))}
			case "mdhd":
				if len(box) < 12 {
					return fmt.Errorf("truncated video timescale")
				}
				offset := 20
				if box[8] == 1 {
					offset = 28
				}
				if offset+4 > len(box) {
					return fmt.Errorf("invalid video timescale")
				}
				c.Scale = int64(binary.BigEndian.Uint32(box[offset : offset+4]))
			case "stsd":
				if len(box) < 16 {
					return fmt.Errorf("invalid video description")
				}
				return walk(box[16:], c)
			case "enca", "mp4a":
				c.audio = true
			case "encv", "avc1":
				if len(box) < 86 {
					return fmt.Errorf("invalid video entry")
				}
				c.Width = int(binary.BigEndian.Uint16(box[32:34]))
				c.Height = int(binary.BigEndian.Uint16(box[34:36]))
				return walk(box[86:], c)
			case "avcC":
				c.Extra = append([]byte(nil), box[8:]...)
			}
			return nil
		})
	}
	if err := walk(data, &result); err != nil {
		return result, err
	}
	c := result
	c.Duration, c.Size = defaults[c.TrackID][0], defaults[c.TrackID][1]
	if c.TrackID == 0 || c.Scale < 1 || c.Scale > 100000000 || (!audio && (c.Width < 1 || c.Height < 1 || c.Width > 1920 || c.Height > 1080 || len(c.Extra) < 4)) {
		return c, fmt.Errorf("unsupported media track configuration")
	}
	return c, nil
}

func ParseTrackSamples(data, kid []byte, e Encryption, config TrackConfig, start time.Duration) ([]EncryptedSample, error) {
	scale := config.Scale
	if scale < 1 || scale > 100000000 || len(kid) != 16 {
		return nil, fmt.Errorf("invalid video timeline or identity")
	}
	var samples []EncryptedSample
	var media [][2]int
	if err := indexedBoxes(data, func(kind string, box []byte, offset int) error {
		if kind == "mdat" {
			media = append(media, [2]int{offset + 8, offset + len(box)})
			if len(media) > 1024 {
				return fmt.Errorf("too many media data boxes")
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	clock := int64(start/time.Second)*scale + int64(start%time.Second)*scale/int64(time.Second)
	err := indexedBoxes(data, func(kind string, box []byte, start int) error {
		if kind != "moof" {
			return nil
		}
		return eachBox(box[8:], func(kind string, traf []byte) error {
			if kind != "traf" {
				return nil
			}
			var trackID uint32
			if err := eachBox(traf[8:], func(kind string, box []byte) error {
				if kind == "tfhd" {
					if len(box) < 16 {
						return fmt.Errorf("truncated video track header")
					}
					trackID = binary.BigEndian.Uint32(box[12:16])
				}
				return nil
			}); err != nil {
				return err
			}
			if trackID != config.TrackID {
				return nil
			}
			part, err := trackRunSamples(data, traf, start, kid, e, config, &clock, media)
			if err != nil {
				return err
			}
			if len(samples)+len(part) > 65536 {
				return fmt.Errorf("too many video samples")
			}
			samples = append(samples, part...)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("video fragment has no access units")
	}
	// HLS starts at EXTINF boundaries; fragments may use a nonzero epoch.
	// Preserve composition order and spacing while aligning to the audio clock.
	origin := samples[0].Timestamp
	for i := range samples {
		samples[i].Timestamp += start.Microseconds() - origin
	}
	return samples, nil
}

func trackRunSamples(data, traf []byte, moof int, kid []byte, e Encryption, config TrackConfig, clock *int64, media [][2]int) ([]EncryptedSample, error) {
	var header, enc, timing []byte
	var runs [][]byte
	err := eachBox(traf[8:], func(kind string, box []byte) error {
		switch kind {
		case "tfhd":
			header = box
		case "senc":
			enc = box
		case "tfdt":
			timing = box
		case "trun":
			runs = append(runs, box)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(header) < 16 || len(enc) == 0 || len(runs) == 0 {
		return nil, fmt.Errorf("incomplete video fragment")
	}
	encrypted, err := fragmentEncryption(enc, e)
	if err != nil {
		return nil, err
	}
	scale, dts := config.Scale, *clock
	if len(timing) > 0 && len(timing) < 16 {
		return nil, fmt.Errorf("invalid video timing box")
	}
	if len(timing) > 0 && timing[8] == 1 {
		if len(timing) < 20 {
			return nil, fmt.Errorf("truncated video timestamp")
		}
		n := binary.BigEndian.Uint64(timing[12:20])
		if n > uint64(scale)*172800 {
			return nil, fmt.Errorf("excessive video timestamp")
		}
		dts = int64(n)
	} else if len(timing) > 0 {
		dts = int64(binary.BigEndian.Uint32(timing[12:16]))
	}
	flags := binary.BigEndian.Uint32(header[8:12]) & 0xffffff
	p, base, defaultSize := 16, moof, int(config.Size)
	if flags&1 != 0 {
		if p+8 > len(header) {
			return nil, fmt.Errorf("truncated video base offset")
		}
		n := binary.BigEndian.Uint64(header[p : p+8])
		if n > uint64(len(data)) {
			return nil, fmt.Errorf("unsupported absolute video offset")
		}
		base = int(n)
		p += 8
	}
	if flags&2 != 0 {
		p += 4
	}
	defaultDuration := config.Duration
	if flags&8 != 0 {
		if p+4 > len(header) {
			return nil, fmt.Errorf("invalid default duration")
		}
		defaultDuration = int64(binary.BigEndian.Uint32(header[p : p+4]))
		p += 4
	}
	if flags&16 != 0 {
		if p+4 > len(header) {
			return nil, fmt.Errorf("invalid default size")
		}
		defaultSize = int(binary.BigEndian.Uint32(header[p : p+4]))
		p += 4
	}
	if flags&32 != 0 {
		p += 4
	}
	if p > len(header) {
		return nil, fmt.Errorf("truncated video header")
	}
	samples := make([]EncryptedSample, 0, len(encrypted))
	cursor := 0
	for _, run := range runs {
		if len(run) < 16 {
			return nil, fmt.Errorf("truncated video run")
		}
		flags := binary.BigEndian.Uint32(run[8:12]) & 0xffffff
		n := int(binary.BigEndian.Uint32(run[12:16]))
		p := 16
		if n < 1 || n > len(encrypted)-len(samples) {
			return nil, fmt.Errorf("invalid video run sample count")
		}
		if flags&1 != 0 {
			if p+4 > len(run) {
				return nil, fmt.Errorf("truncated video offset")
			}
			cursor = base + int(int32(binary.BigEndian.Uint32(run[p:p+4])))
			p += 4
		}
		if flags&4 != 0 {
			p += 4
		}
		for i := 0; i < n; i++ {
			duration, size := defaultDuration, defaultSize
			if flags&0x100 != 0 {
				if p+4 > len(run) {
					return nil, fmt.Errorf("truncated video duration")
				}
				duration = int64(binary.BigEndian.Uint32(run[p : p+4]))
				p += 4
			}
			if flags&0x200 != 0 {
				if p+4 > len(run) {
					return nil, fmt.Errorf("truncated video size")
				}
				size = int(binary.BigEndian.Uint32(run[p : p+4]))
				p += 4
			}
			if flags&0x400 != 0 {
				p += 4
			}
			var composition int64
			if flags&0x800 != 0 {
				if p+4 > len(run) {
					return nil, fmt.Errorf("truncated video composition")
				}
				n := binary.BigEndian.Uint32(run[p : p+4])
				composition = int64(n)
				if run[8] == 1 {
					composition = int64(int32(n))
				}
				p += 4
			}
			if p > len(run) || duration < 1 || size < 1 || cursor < 0 || cursor > len(data)-size || dts+composition < -scale*60 || dts+composition > scale*172800 {
				return nil, fmt.Errorf("invalid video sample bounds or timing")
			}
			valid := false
			for _, region := range media {
				if cursor >= region[0] && cursor+size <= region[1] {
					valid = true
					break
				}
			}
			if !valid {
				return nil, fmt.Errorf("video sample is outside media data")
			}
			protection := encrypted[len(samples)]
			var total uint64
			for _, sub := range protection.subs {
				total += uint64(sub.ClearBytes) + uint64(sub.CipherBytes)
			}
			if len(protection.subs) > 0 && total != uint64(size) {
				return nil, fmt.Errorf("invalid video subsample byte count")
			}
			samples = append(samples, EncryptedSample{Data: data[cursor : cursor+size], KID: kid, IV: protection.iv, Subsamples: protection.subs, Timestamp: (dts + composition) * 1000000 / scale})
			dts += duration
			cursor += size
		}
	}
	if len(samples) != len(encrypted) {
		return nil, fmt.Errorf("video run/encryption count mismatch")
	}
	*clock = dts
	return samples, nil
}

func eachBox(data []byte, visit func(string, []byte) error) error {
	return indexedBoxes(data, func(kind string, box []byte, _ int) error { return visit(kind, box) })
}
func indexedBoxes(data []byte, visit func(string, []byte, int) error) error {
	start := 0
	for len(data) > 0 {
		if len(data) < 8 {
			return fmt.Errorf("truncated media box")
		}
		n := uint64(binary.BigEndian.Uint32(data[:4]))
		if n < 8 || n > uint64(len(data)) {
			return fmt.Errorf("invalid media box size")
		}
		if err := visit(string(data[4:8]), data[:int(n)], start); err != nil {
			return err
		}
		data = data[int(n):]
		start += int(n)
	}
	return nil
}

type sampleEncryption struct {
	iv   []byte
	subs []Subsample
}

// DecryptAudioFragment handles the multi-run fragments used by video audio
// renditions without changing the established song/station parsing path.
func DecryptAudioFragment(data, kid []byte, track TrackConfig, audio AudioConfig, protection Encryption, decrypt DecryptFunc) ([]byte, error) {
	samples, err := ParseTrackSamples(data, kid, protection, track, 0)
	if err != nil {
		return nil, err
	}
	var output []byte
	for _, sample := range samples {
		if len(sample.Data) > 8184 {
			return nil, fmt.Errorf("AAC access unit exceeds ADTS limit")
		}
		clear, err := decrypt(sample.KID, sample.IV, sample.Data, sample.Subsamples)
		if err != nil {
			return nil, err
		}
		if len(clear) != len(sample.Data) {
			return nil, fmt.Errorf("invalid decoded AAC sample length")
		}
		output = append(output, MakeADTSHeader(len(clear), audio)...)
		output = append(output, clear...)
	}
	return output, nil
}

func fragmentEncryption(box []byte, e Encryption) ([]sampleEncryption, error) {
	if len(box) < 16 {
		return nil, fmt.Errorf("truncated video encryption")
	}
	flags := binary.BigEndian.Uint32(box[8:12]) & 0xffffff
	if flags&1 != 0 {
		return nil, fmt.Errorf("unsupported encryption override")
	}
	count := int(binary.BigEndian.Uint32(box[12:16]))
	if count < 1 || count > 65536 {
		return nil, fmt.Errorf("invalid encrypted video sample count")
	}
	samples := make([]sampleEncryption, 0, count)
	p := 16
	for i := 0; i < count; i++ {
		iv := e.ConstantIV
		if e.IVSize > 0 {
			if p+e.IVSize > len(box) {
				return nil, fmt.Errorf("truncated video IV")
			}
			iv = box[p : p+e.IVSize]
			p += e.IVSize
		}
		if len(iv) != 8 && len(iv) != 16 {
			return nil, fmt.Errorf("invalid video IV")
		}
		sample := sampleEncryption{iv: iv}
		if flags&2 != 0 {
			if p+2 > len(box) {
				return nil, fmt.Errorf("truncated subsample count")
			}
			n := int(binary.BigEndian.Uint16(box[p : p+2]))
			p += 2
			if n > (len(box)-p)/6 {
				return nil, fmt.Errorf("truncated video subsamples")
			}
			for j := 0; j < n; j++ {
				sample.subs = append(sample.subs, Subsample{ClearBytes: binary.BigEndian.Uint16(box[p : p+2]), CipherBytes: binary.BigEndian.Uint32(box[p+2 : p+6])})
				p += 6
			}
		}
		samples = append(samples, sample)
	}
	if p != len(box) {
		return nil, fmt.Errorf("inconsistent video encryption length")
	}
	return samples, nil
}
