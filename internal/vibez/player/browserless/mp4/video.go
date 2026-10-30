package mp4

import (
	"encoding/binary"
	"fmt"
)

// AVCDecoderData converts codec configuration to the Annex B form used by the
// native CDM decoder. These are public codec parameters, never key material.
func AVCDecoderData(extra []byte) ([]byte, error) {
	if len(extra) < 7 || extra[0] != 1 || extra[4]&3 != 3 {
		return nil, fmt.Errorf("unsupported AVC NAL framing")
	}
	var out []byte
	p := 6
	for group := 0; group < 2; group++ {
		n := int(extra[5] & 31)
		if group == 1 {
			if p >= len(extra) {
				return nil, fmt.Errorf("truncated AVC picture parameters")
			}
			n = int(extra[p])
			p++
		}
		for i := 0; i < n; i++ {
			if p+2 > len(extra) {
				return nil, fmt.Errorf("truncated AVC parameters")
			}
			size := int(binary.BigEndian.Uint16(extra[p : p+2]))
			p += 2
			if size == 0 || p+size > len(extra) {
				return nil, fmt.Errorf("invalid AVC parameters")
			}
			out = append(out, 0, 0, 0, 1)
			out = append(out, extra[p:p+size]...)
			p += size
		}
	}
	return out, nil
}

func AVCFrameData(sample EncryptedSample) ([]byte, error) {
	out := append([]byte(nil), sample.Data...)
	for p := 0; p < len(out); {
		if p+4 > len(out) {
			return nil, fmt.Errorf("truncated AVC NAL size")
		}
		clear := false
		offset := 0
		for _, sub := range sample.Subsamples {
			if p >= offset && p+4 <= offset+int(sub.ClearBytes) {
				clear = true
			}
			offset += int(sub.ClearBytes) + int(sub.CipherBytes)
		}
		if !clear {
			return nil, fmt.Errorf("AVC length prefix is encrypted")
		}
		n := int(binary.BigEndian.Uint32(out[p : p+4]))
		if n < 1 || n > len(out)-p-4 {
			return nil, fmt.Errorf("invalid AVC NAL size")
		}
		copy(out[p:p+4], []byte{0, 0, 0, 1})
		p += 4 + n
	}
	return out, nil
}
