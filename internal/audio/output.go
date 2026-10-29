// Package audio provides native local-file output; it has no account or catalog logic.
package audio

import "time"

type Status struct {
	Position, Duration time.Duration
	Ended              bool
	Error              string
}
type Output interface {
	Open(string) error
	Play() error
	Pause()
	Stop()
	Seek(time.Duration)
	SetVolume(float64)
	SetEqualizer([]float64) error
	Status() Status
	Close()
}
