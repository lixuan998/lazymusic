//go:build linux && cgo

package audio

import (
	"fmt"
	"lazymusic/internal/vibez/player/gst"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

type output struct {
	p       *gst.Player
	ended   atomic.Bool
	mu      sync.Mutex
	problem string
}

func New() (Output, error) {
	p, err := gst.New()
	if err != nil {
		return nil, err
	}
	o := &output{p: p}
	p.OnEOS(func() { o.ended.Store(true) })
	p.OnError(func(err error) { o.mu.Lock(); o.problem = err.Error(); o.mu.Unlock() })
	return o, nil
}
func (o *output) Open(path string) error {
	o.p.Stop()
	o.mu.Lock()
	o.problem = ""
	o.mu.Unlock()
	o.ended.Store(false)
	o.p.PlayURI((&url.URL{Scheme: "file", Path: path}).String())
	o.p.Pause()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		s := o.Status()
		if s.Error != "" {
			return fmt.Errorf("local audio: %s", s.Error)
		}
		if s.Duration > 0 {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("local decoder could not prepare file; check installed GStreamer format plugins")
}
func (o *output) Play() error          { o.p.Play(); return nil }
func (o *output) Pause()               { o.p.Pause() }
func (o *output) Stop()                { o.p.Stop(); o.ended.Store(false) }
func (o *output) Seek(p time.Duration) { o.p.Seek(p); o.ended.Store(false) }
func (o *output) SetVolume(v float64)  { o.p.SetVolume(v) }
func (o *output) Status() Status {
	o.mu.Lock()
	problem := o.problem
	o.mu.Unlock()
	return Status{Position: o.p.Position(), Duration: o.p.Duration(), Ended: o.ended.Load(), Error: problem}
}
func (o *output) Close() { o.p.Destroy() }

func (o *output) SetEqualizer(g []float64) error { o.p.SetEqualizer(g); return nil }
