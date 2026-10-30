package mp4

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

func videoInts(ns ...uint32) []byte {
	b := make([]byte, len(ns)*4)
	for i, n := range ns {
		binary.BigEndian.PutUint32(b[i*4:], n)
	}
	return b
}

func TestVideoConfigurationUsesVideoTrackNotCaptionClock(t *testing.T) {
	track := func(id, scale uint32, video bool) []byte {
		parts := append(makeBox("tkhd", videoInts(0, 0, 0, id)), makeBox("mdhd", videoInts(0, 0, 0, scale))...)
		if video {
			entry := make([]byte, 78)
			binary.BigEndian.PutUint16(entry[24:], 640)
			binary.BigEndian.PutUint16(entry[26:], 360)
			entry = append(entry, makeBox("avcC", []byte{1, 100, 0, 31, 255, 225, 0, 1, 103, 1, 0, 1, 104})...)
			parts = append(parts, makeBox("stsd", append(videoInts(0, 1), makeBox("encv", entry)...))...)
		}
		return makeBox("trak", parts)
	}
	data := append(track(7, 12800, true), track(9, 1000, false)...)
	data = append(data, makeBox("mvex", makeBox("trex", videoInts(0, 7, 1, 512, 8, 0)))...)
	cfg, err := ParseVideoConfig(makeBox("moov", data))
	if err != nil || cfg.TrackID != 7 || cfg.Scale != 12800 || cfg.Width != 640 || cfg.Height != 360 || cfg.Duration != 512 || cfg.Size != 8 {
		t.Fatal("caption/defaults overwrote video configuration", cfg, err)
	}
}

// This synthetic fragment models multiple truns, a caption track without senc,
// missing tfdt, and a ten-second composition epoch. No licensed media is stored.
func syntheticVideoFragment() []byte {
	header := makeBox("tfhd", videoInts(0x200018, 7, 40, 8))
	senc := makeBox("senc", append(videoInts(2, 2), []byte{0, 1, 0, 4, 0, 0, 0, 4, 0, 1, 0, 4, 0, 0, 0, 4}...))
	run := func(offset uint32) []byte { return makeBox("trun", videoInts(0x801, 1, offset, 10000)) }
	build := func(offset uint32) []byte {
		body := append(append(append([]byte{}, header...), senc...), run(offset)...)
		body = append(body, run(offset+8)...)
		body = append(makeBox("traf", body), makeBox("traf", makeBox("tfhd", videoInts(0, 9)))...)
		return makeBox("moof", body)
	}
	moof := build(0)
	moof = build(uint32(len(moof) + 8))
	return append(moof, makeBox("mdat", append(videoInts(4, 0x65010203), videoInts(4, 0x41040506)...))...)
}

func TestVideoTimelineAnchorsHLSSegmentsAndPreservesSubsamples(t *testing.T) {
	data := syntheticVideoFragment()
	cfg := TrackConfig{TrackID: 7, Scale: 1000}
	e := Encryption{CBCS: true, ConstantIV: make([]byte, 16)}
	frames, err := ParseTrackSamples(data, make([]byte, 16), e, cfg, 12*time.Second)
	if err != nil || len(frames) != 2 {
		t.Fatal("cannot read multi-run encrypted fragment", err)
	}
	if frames[0].Timestamp != 12000000 || frames[1].Timestamp != 12040000 {
		t.Fatal("nonzero epoch was mistaken for playback time", frames)
	}
	count := 0
	audio, err := DecryptAudioFragment(data, make([]byte, 16), cfg, DefaultAudioConfig(), e, func(_, _, input []byte, subs []Subsample) ([]byte, error) {
		count++
		if len(subs) != 1 {
			t.Fatal("audio protection lost")
		}
		return input, nil
	})
	if err != nil || count != 2 || len(audio) != 30 || audio[0] != 255 || audio[15] != 255 {
		t.Fatal("multi-run video audio lost frames/ADTS headers", count, err)
	}
	packet, err := AVCFrameData(frames[0])
	if err != nil || !bytes.Equal(packet[:4], []byte{0, 0, 0, 1}) || !bytes.Equal(packet[4:], frames[0].Data[4:]) || frames[0].Data[3] != 4 {
		t.Fatal("NAL conversion changed encrypted bytes or source buffer", err)
	}
	for _, n := range []int{0, 7, len(data) - 1} {
		if _, err := ParseTrackSamples(data[:n], make([]byte, 16), e, cfg, 0); err == nil {
			t.Fatal("accepted truncated fragment", n)
		}
	}
}

func TestAVCRejectsEncryptedPrefixesAndMalformedParameters(t *testing.T) {
	packet := EncryptedSample{Data: videoInts(4, 0x65010203)}
	if _, err := AVCFrameData(packet); err == nil {
		t.Fatal("read NAL lengths from encrypted range")
	}
	packet.Subsamples = []Subsample{{ClearBytes: 3, CipherBytes: 5}}
	if _, err := AVCFrameData(packet); err == nil {
		t.Fatal("prefix crosses clear/encrypted boundary")
	}
	extra := []byte{1, 100, 0, 31, 255, 225, 0, 1, 103, 1, 0, 1, 104}
	if data, err := AVCDecoderData(extra); err != nil || !bytes.Equal(data, []byte{0, 0, 0, 1, 103, 0, 0, 0, 1, 104}) {
		t.Fatal("missing in-band codec parameters", data, err)
	}
	for _, n := range []int{0, 6, len(extra) - 1} {
		if _, err := AVCDecoderData(extra[:n]); err == nil {
			t.Fatal("accepted incomplete codec parameters", n)
		}
	}
}
