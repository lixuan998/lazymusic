//go:build linux

package volume

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type driver struct{ tool string }

func newDriver() Driver {
	for _, name := range []string{"wpctl", "pactl"} {
		if path, err := exec.LookPath(name); err == nil {
			return driver{path}
		}
	}
	return driver{}
}
func (d driver) run(ctx context.Context, args ...string) (string, error) {
	if d.tool == "" {
		return "", fmt.Errorf("system volume requires PipeWire wpctl or PulseAudio pactl")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	// pactl output is parsed using its stable C locale.
	defer cancel()
	cmd := exec.CommandContext(ctx, d.tool, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	b, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("system audio service unavailable: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}
func (d driver) Read(ctx context.Context) (float64, bool, error) {
	if strings.HasSuffix(d.tool, "wpctl") {
		s, err := d.run(ctx, "get-volume", "@DEFAULT_AUDIO_SINK@")
		if err != nil {
			return 0, false, err
		}
		f := strings.Fields(s)
		if len(f) < 2 {
			return 0, false, fmt.Errorf("invalid system volume response")
		}
		v, err := strconv.ParseFloat(f[1], 64)
		return min(1, max(0, v)), strings.Contains(s, "MUTED"), err
	}
	s, err := d.run(ctx, "get-sink-volume", "@DEFAULT_SINK@")
	if err != nil {
		return 0, false, err
	}
	for _, f := range strings.Fields(s) {
		if strings.HasSuffix(f, "%") {
			v, e := strconv.ParseFloat(strings.TrimSuffix(f, "%"), 64)
			if e != nil {
				return 0, false, e
			}
			m, e := d.run(ctx, "get-sink-mute", "@DEFAULT_SINK@")
			return min(1, max(0, v/100)), strings.Contains(m, "yes"), e
		}
	}
	return 0, false, fmt.Errorf("invalid system volume response")
}
func (d driver) Write(ctx context.Context, v float64) error {
	if strings.HasSuffix(d.tool, "wpctl") {
		_, err := d.run(ctx, "set-volume", "-l", "1", "@DEFAULT_AUDIO_SINK@", strconv.FormatFloat(v, 'f', 3, 64))
		if err == nil && v > 0 {
			_, err = d.run(ctx, "set-mute", "@DEFAULT_AUDIO_SINK@", "0")
		}
		return err
	}
	_, err := d.run(ctx, "set-sink-volume", "@DEFAULT_SINK@", fmt.Sprintf("%.0f%%", v*100))
	if err == nil && v > 0 {
		_, err = d.run(ctx, "set-sink-mute", "@DEFAULT_SINK@", "0")
	}
	return err
}
