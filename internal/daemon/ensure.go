package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func Ensure(ctx context.Context, dir string, source ...string) (*Client, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	c, err := NewClient(abs)
	if err != nil {
		return nil, err
	}
	if state, err := c.State(ctx); err == nil {
		return checkBackend(c, state, source...)
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return nil, err
	}
	log, err := os.OpenFile(filepath.Join(abs, "service.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer log.Close()
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := []string{"daemon", "--state-dir", abs}
	if len(source) > 0 && source[0] != "" {
		args = append(args, "--source", source[0])
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = abs
	cmd.Stdin = nil
	cmd.Stdout = log
	cmd.Stderr = log
	configureBackground(cmd)
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	go func() { _ = cmd.Wait() }()
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, fmt.Errorf("background service did not start; see %s", filepath.Join(abs, "service.log"))
		case <-tick.C:
			if state, err := c.State(ctx); err == nil {
				return checkBackend(c, state, source...)
			}
		}
	}
}

func checkBackend(c *Client, s State, source ...string) (*Client, error) {
	want := "apple"
	if len(source) > 0 && source[0] != "" {
		want = source[0]
	}
	if s.Protocol != ProtocolVersion {
		return nil, fmt.Errorf("background protocol changed; run lazymusic quit --source %s, then restart", s.Backend)
	}
	if s.Backend != want {
		return nil, fmt.Errorf("profile is running %s backend; choose another --state-dir or stop it first", s.Backend)
	}
	return c, nil
}
