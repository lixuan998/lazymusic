package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsLockAndProfileIdentity(t *testing.T) {
	dir := t.TempDir()
	first, err := acquireLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := acquireLock(dir); err == nil {
		second.Close()
		t.Fatal("second lock acquired")
	}
	first.Close()
	next, err := acquireLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
	a, err := SocketPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := SocketPath(filepath.Join(dir, "."))
	if err != nil || a != b {
		t.Fatalf("unstable pipe: %s %s %v", a, b, err)
	}
	other, _ := SocketPath(t.TempDir())
	if other == a {
		t.Fatal("profiles share pipe")
	}
	if !processAlive(os.Getpid()) || processAlive(-1) {
		t.Fatal("wrong process liveness")
	}
}
