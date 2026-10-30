package mp4

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Encryption describes the declared track encryption; no decryption key is
// exposed. CBCS audio commonly uses a constant IV and no per-sample IV bytes.
type Encryption struct {
	KID, ConstantIV     []byte
	IVSize, Crypt, Skip int
	CBCS                bool
}

func ParseEncryption(init []byte) (Encryption, error) {
	var e Encryption
	scheme := bytes.Index(init, []byte("schm"))
	if scheme < 0 || scheme+12 > len(init) {
		return e, fmt.Errorf("missing encryption scheme")
	}
	e.CBCS = string(init[scheme+8:scheme+12]) == "cbcs"
	i := bytes.Index(init, []byte("tenc"))
	if i < 4 || i+28 > len(init) {
		return e, fmt.Errorf("missing track encryption parameters")
	}
	size := int(binary.BigEndian.Uint32(init[i-4 : i]))
	end := i - 4 + size
	if size < 32 || end > len(init) {
		return e, fmt.Errorf("invalid track encryption box")
	}
	version := init[i+4]
	if version > 1 {
		return e, fmt.Errorf("unsupported track encryption version")
	}
	if version == 1 {
		e.Crypt = int(init[i+9] >> 4)
		e.Skip = int(init[i+9] & 15)
	}
	e.IVSize = int(init[i+11])
	e.KID = append([]byte(nil), init[i+12:i+28]...)
	if e.IVSize != 0 && e.IVSize != 8 && e.IVSize != 16 {
		return e, fmt.Errorf("invalid encryption IV size")
	}
	if e.IVSize == 0 {
		if i+29 > end {
			return e, fmt.Errorf("missing constant IV")
		}
		n := int(init[i+28])
		if (n != 8 && n != 16) || i+29+n > end {
			return e, fmt.Errorf("invalid constant IV")
		}
		e.ConstantIV = append([]byte(nil), init[i+29:i+29+n]...)
	}
	return e, nil
}

// WidevineKID reads the public key identifier from protection metadata. A
// station's init segment may declare an empty default KID; the HLS PSSH names
// the actual key. Key material remains inside the licensed CDM.
func WidevineKID(pssh []byte) ([]byte, error) {
	if len(pssh) < 32 || string(pssh[4:8]) != "pssh" || int(binary.BigEndian.Uint32(pssh[:4])) != len(pssh) {
		return nil, fmt.Errorf("invalid protection box")
	}
	if pssh[8] == 1 {
		if len(pssh) < 52 || binary.BigEndian.Uint32(pssh[28:32]) < 1 {
			return nil, fmt.Errorf("protection box has no key ID")
		}
		return append([]byte(nil), pssh[32:48]...), nil
	}
	if pssh[8] != 0 {
		return nil, fmt.Errorf("unsupported protection box version")
	}
	data := pssh[32:]
	read := func(data []byte) (uint64, int) {
		var value uint64
		for i, b := range data {
			if i >= 10 {
				return 0, 0
			}
			value |= uint64(b&127) << uint(i*7)
			if b&128 == 0 {
				return value, i + 1
			}
		}
		return 0, 0
	}
	for len(data) > 0 {
		tag, n := read(data)
		if n == 0 {
			break
		}
		data = data[n:]
		switch tag & 7 {
		case 0:
			_, n = read(data)
			if n == 0 {
				return nil, fmt.Errorf("invalid protection metadata")
			}
			data = data[n:]
		case 2:
			size, n := read(data)
			if n == 0 || size > uint64(len(data)-n) {
				return nil, fmt.Errorf("invalid protection metadata")
			}
			data = data[n:]
			if tag>>3 == 2 && size == 16 {
				return append([]byte(nil), data[:16]...), nil
			}
			data = data[int(size):]
		default:
			return nil, fmt.Errorf("unsupported protection metadata")
		}
	}
	return nil, fmt.Errorf("protection metadata has no key ID")
}
