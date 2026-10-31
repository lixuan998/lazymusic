//go:build windows && cgo

package audio

import (
	"lazymusic/internal/vibez/player/browserless"
	"sync"
	"sync/atomic"
	"time"
)

type output struct {
	p       browserless.FileSink
	ended   atomic.Bool
	mu      sync.Mutex
	problem string
}

func New() (Output, error) {
	p, err := browserless.NewFileSink()
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
	o.ended.Store(false)
	o.mu.Lock()
	o.problem = ""
	o.mu.Unlock()
	return o.p.OpenFile(path)
}
func (o *output) Play() error                    { o.p.Play(); return nil }
func (o *output) Pause()                         { o.p.Pause() }
func (o *output) Stop()                          { o.p.Stop(); o.ended.Store(false) }
func (o *output) Seek(p time.Duration)           { o.p.SeekFile(p); o.ended.Store(false) }
func (o *output) SetVolume(v float64)            { o.p.SetVolume(v) }
func (o *output) SetEqualizer(g []float64) error { o.p.SetEqualizer(g); return nil }
func (o *output) Status() Status {
	o.mu.Lock()
	problem := o.problem
	o.mu.Unlock()
	return Status{Position: o.p.Position(), Duration: o.p.Duration(), Ended: o.ended.Load(), Error: problem}
}
func (o *output) Close() { o.p.Destroy() }
