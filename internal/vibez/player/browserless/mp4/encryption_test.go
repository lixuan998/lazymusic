package mp4

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestCBCSConstantIVAndManifestKID(t *testing.T) {
	iv := bytes.Repeat([]byte{7}, 16)
	data := []byte{1, 0, 0, 0, 0, 0, 1, 0}
	data = append(data, make([]byte, 16)...)
	data = append(data, 16)
	data = append(data, iv...)
	init := append(makeBox("schm", append(make([]byte, 4), []byte("cbcs")...)), makeBox("tenc", data)...)
	enc, err := ParseEncryption(init)
	if err != nil || !enc.CBCS || enc.IVSize != 0 || !bytes.Equal(enc.ConstantIV, iv) || enc.Crypt != 0 || enc.Skip != 0 {
		t.Fatalf("CBCS params: %+v %v", enc, err)
	}
	kid := bytes.Repeat([]byte{9}, 16)
	payload := append([]byte{0x12, 0x10}, kid...)
	pssh := makeBox("pssh", append(make([]byte, 24), payload...))
	binary.BigEndian.PutUint32(pssh[28:32], uint32(len(payload)))
	got, err := WidevineKID(pssh)
	if err != nil || !bytes.Equal(got, kid) || bytes.Equal(got, enc.KID) {
		t.Fatal("did not use PSSH's real key ID")
	}
	for _, bad := range [][]byte{nil, init[:12], init[:len(init)-1]} {
		if _, err = ParseEncryption(bad); err == nil {
			t.Fatal("accepted truncated encryption metadata")
		}
	}
}

func TestCBCSNoPerSampleIVUsesConstantIV(t *testing.T) {
	iv := bytes.Repeat([]byte{3}, 16)
	sample := bytes.Repeat([]byte{4}, 32)
	senc := makeBox("senc", []byte{0, 0, 0, 0, 0, 0, 0, 1})
	trunData := []byte{0, 0, 2, 1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 32}
	moof := makeBox("moof", makeBox("traf", append(senc, makeBox("trun", trunData)...)))
	binary.BigEndian.PutUint32(trunData[8:12], uint32(len(moof)+8))
	moof = makeBox("moof", makeBox("traf", append(senc, makeBox("trun", trunData)...)))
	segment := append(moof, makeBox("mdat", sample)...)
	called := false
	out, err := DecryptSegment(segment, make([]byte, 16), DefaultAudioConfig(), func(_, gotIV, input []byte, _ []Subsample) ([]byte, error) {
		called = true
		if !bytes.Equal(iv, gotIV) {
			t.Fatal("wrong constant IV")
		}
		return input, nil
	}, Encryption{IVSize: 0, ConstantIV: iv, CBCS: true})
	if err != nil || !called || len(out) != 39 || !bytes.Equal(out[7:], sample) {
		t.Fatalf("CBCS frame extraction: %v", err)
	}
}
