//go:build darwin || linux

package daemon

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func SocketPath(stateDir string) (string, error) {
	abs, err := filepath.Abs(stateDir)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	h := sha256.Sum256([]byte(abs))
	dir := filepath.Join("/tmp", fmt.Sprintf("lazymusic-%d-%x", os.Getuid(), h[:8]))
	if err = os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 || !ok || int(st.Uid) != os.Getuid() {
		return "", fmt.Errorf("unsafe runtime directory %s", dir)
	}
	if err = os.Chmod(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "ipc.sock"), nil
}

func acquireLock(dir string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(filepath.Join(dir, "daemon.lock"), syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "daemon.lock")
	if err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("background service already running for this profile")
	}
	return f, nil
}

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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
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
