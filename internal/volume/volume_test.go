package volume

import (
	"context"
	"errors"
	"math"
	"testing"
)

type fakeDriver struct {
	level         float64
	reads, writes int
	err           error
}

func (d *fakeDriver) Read(context.Context) (float64, bool, error) {
	d.reads++
	return d.level, false, d.err
}
func (d *fakeDriver) Write(_ context.Context, v float64) error {
	d.writes++
	if d.err == nil {
		d.level = v
	}
	return d.err
}
func TestControllerReadsSystemStateCachesAndInvalidatesAfterWrite(t *testing.T) {
	d := &fakeDriver{level: .4}
	c := WithDriver(d)
	ctx := context.Background()
	if s := c.Read(ctx); !s.Available || s.Level != .4 {
		t.Fatal(s)
	}
	c.Read(ctx)
	if d.reads != 1 {
		t.Fatal("read not cached")
	}
	if err := c.Set(ctx, .6); err != nil {
		t.Fatal(err)
	}
	if s := c.Read(ctx); s.Level != .6 || d.reads != 2 {
		t.Fatal("write did not invalidate cache")
	}
	for _, v := range []float64{-1, 1.1, math.NaN(), math.Inf(1)} {
		if err := c.Set(ctx, v); err == nil {
			t.Fatal("invalid volume accepted")
		}
	}
	if d.writes != 1 {
		t.Fatal("invalid input reached driver")
	}
}
func TestUnavailableOutputIsExplicit(t *testing.T) {
	d := &fakeDriver{err: errors.New("no default sink")}
	c := WithDriver(d)
	if s := c.Read(context.Background()); s.Available || s.Error == "" {
		t.Fatal("unavailable volume pretended to work")
	}
	if err := c.Set(context.Background(), .5); err == nil {
		t.Fatal("write error lost")
	}
}
