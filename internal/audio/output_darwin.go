//go:build darwin && cgo

package audio

/*
#cgo darwin LDFLAGS: -framework AVFoundation -framework Foundation
#include <stdlib.h>
#include "output_darwin.h"
*/
import "C"
import (
	"fmt"
	"time"
	"unsafe"
)

type output struct {
	gains           [10]float64
	handle          unsafe.Pointer
	duration        time.Duration
	started, paused bool
}

func New() (Output, error) { return &output{}, nil }
func (o *output) Open(path string) error {
	o.Close()
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	var message *C.char
	var seconds C.double
	o.handle = C.lm_audio_open(p, &message, &seconds)
	if o.handle == nil {
		defer C.free(unsafe.Pointer(message))
		return fmt.Errorf("local audio: %s", C.GoString(message))
	}
	_ = o.SetEqualizer(o.gains[:])
	o.duration = time.Duration(float64(seconds) * float64(time.Second))
	o.started = false
	o.paused = true
	return nil
}
func (o *output) Play() error {
	if o.handle == nil {
		return fmt.Errorf("no audio file loaded")
	}
	if !bool(C.lm_audio_play(o.handle)) {
		return fmt.Errorf("native local player could not start")
	}
	o.started = true
	o.paused = false
	return nil
}
func (o *output) Pause() {
	if o.handle != nil {
		C.lm_audio_pause(o.handle)
	}
	o.paused = true
}
func (o *output) Stop() {
	o.Pause()
	if o.handle != nil {
		C.lm_audio_seek(o.handle, 0)
	}
	o.started = false
}
func (o *output) Seek(pos time.Duration) {
	if o.handle != nil {
		C.lm_audio_seek(o.handle, C.double(pos.Seconds()))
	}
}
func (o *output) SetVolume(v float64) {
	if o.handle != nil {
		C.lm_audio_volume(o.handle, C.double(v))
	}
}
func (o *output) Status() Status {
	if o.handle == nil {
		return Status{}
	}
	return Status{Position: time.Duration(float64(C.lm_audio_position(o.handle)) * float64(time.Second)), Duration: o.duration, Ended: o.started && !o.paused && !bool(C.lm_audio_playing(o.handle))}
}
func (o *output) Close() {
	if o.handle != nil {
		C.lm_audio_close(o.handle)
		o.handle = nil
	}
	o.started = false
	o.paused = true
}

func (o *output) SetEqualizer(g []float64) error {
	if len(g) != 10 {
		return fmt.Errorf("equalizer requires 10 bands")
	}
	copy(o.gains[:], g)
	if o.handle != nil {
		var values [10]C.double
		for i := range values {
			values[i] = C.double(g[i])
		}
		C.lm_audio_equalizer(o.handle, &values[0])
	}
	return nil
}
