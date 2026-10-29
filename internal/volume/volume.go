// Package volume controls the operating system's current default output device.
package volume

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

type State struct {
	Level     float64 `json:"level"`
	Muted     bool    `json:"muted"`
	Available bool    `json:"available"`
	Error     string  `json:"error,omitempty"`
}
type Driver interface {
	Read(context.Context) (float64, bool, error)
	Write(context.Context, float64) error
}
type Controller struct {
	mu     sync.Mutex
	driver Driver
	cached State
	at     time.Time
}

func New() *Controller                { return &Controller{driver: newDriver()} }
func WithDriver(d Driver) *Controller { return &Controller{driver: d} }
func (c *Controller) Read(ctx context.Context) State {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && time.Since(c.at) < time.Second {
		return c.cached
	}
	v, m, err := c.driver.Read(ctx)
	c.cached = State{Level: v, Muted: m, Available: err == nil}
	if err != nil {
		c.cached.Error = err.Error()
	}
	c.at = time.Now()
	return c.cached
}
func (c *Controller) Set(ctx context.Context, v float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
		return fmt.Errorf("system volume must be between 0 and 1")
	}
	if err := c.driver.Write(ctx, v); err != nil {
		return err
	}
	c.at = time.Time{}
	return nil
}
