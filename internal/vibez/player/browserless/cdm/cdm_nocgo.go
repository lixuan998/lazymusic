//go:build !cgo || (!linux && !darwin && !windows)

package cdm

import (
	"errors"
)

var errNoCgo = errors.New("browserless CDM requires CGo")

type CDM struct{}

type Subsample struct {
	ClearBytes  uint16
	CipherBytes uint32
}

func New(_ string) (*CDM, error) {
	return nil, errNoCgo
}

func (c *CDM) SetServerCertificate(_ []byte) error {
	return errNoCgo
}

func (c *CDM) GenerateChallenge(_ []byte) ([]byte, string, error) {
	return nil, "", errNoCgo
}

func (c *CDM) UpdateSession(_ string, _ []byte) error {
	return errNoCgo
}

func (c *CDM) Decrypt(_, _, _ []byte) ([]byte, error) {
	return nil, errNoCgo
}

func (c *CDM) DecryptSubsamples(_, _, _ []byte, _ []Subsample) ([]byte, error) {
	return nil, errNoCgo
}

func (c *CDM) Close() error {
	return nil
}

func (c *CDM) DecryptCBCS(_, _, _ []byte, _ []Subsample, _, _ int) ([]byte, error) {
	return nil, errNoCgo
}

type VideoFrame struct {
	RGBA          []byte
	Width, Height int
	Timestamp     int64
}

func (c *CDM) ConfigureVideo([]byte, int, int) error { return errNoCgo }
func (c *CDM) ResetVideo()                           {}
func (c *CDM) DecodeVideo([]byte, []byte, []byte, []Subsample, int, int, int64, int64) (VideoFrame, error) {
	return VideoFrame{}, errNoCgo
}
