package music

import (
	"fmt"
	"math"
)

var EQFrequencies = [10]float64{32, 64, 125, 250, 500, 1000, 2000, 4000, 8000, 16000}
var EQLabels = [10]string{"32", "64", "125", "250", "500", "1k", "2k", "4k", "8k", "16k"}

type Equalizer struct {
	Gains    [10]float64 `json:"gains"`
	Bypassed bool        `json:"bypassed"`
}

func (e Equalizer) Validate() error {
	for _, g := range e.Gains {
		if math.IsNaN(g) || math.IsInf(g, 0) || g < -12 || g > 12 {
			return fmt.Errorf("equalizer gains must be between -12 and +12 dB")
		}
	}
	return nil
}
func (e Equalizer) Effective() []float64 {
	g := make([]float64, 10)
	if !e.Bypassed {
		copy(g, e.Gains[:])
	}
	return g
}
func EQPreset(name string) (Equalizer, error) {
	e := Equalizer{}
	switch name {
	case "flat":
	case "bass":
		e.Gains = [10]float64{5, 4, 3, 1, 0, 0, 0, 0, 0, 0}
	case "vocal":
		e.Gains = [10]float64{-2, -1, 0, 1, 3, 3, 2, 0, -1, -2}
	case "treble":
		e.Gains = [10]float64{0, 0, 0, 0, 0, 1, 2, 3, 4, 4}
	default:
		return e, fmt.Errorf("unknown equalizer preset")
	}
	return e, nil
}
