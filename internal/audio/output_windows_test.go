//go:build windows && cgo

package audio

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWindowsFilePlaybackLifecycle(t *testing.T) {
	t.Setenv("VIBEZ_NULL_AUDIO", "1")
	// A silent PCM fixture exercises the OS decoder without changing device volume.
	const rate = 44100
	pcm := make([]byte, rate*2*3)
	wav := make([]byte, 44+len(pcm))
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], rate)
	binary.LittleEndian.PutUint32(wav[28:], rate*2)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], uint32(len(pcm)))
	path := filepath.Join(t.TempDir(), "音乐 test.wav")
	if err := os.WriteFile(path, wav, 0600); err != nil {
		t.Fatal(err)
	}
	o, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if err := o.Open(path); err != nil {
		t.Fatal(err)
	}
	if d := o.Status().Duration; d < 2900*time.Millisecond || d > 3100*time.Millisecond {
		t.Fatalf("duration=%s", d)
	}
	time.Sleep(80 * time.Millisecond)
	if p := o.Status().Position; p != 0 {
		t.Fatalf("Open auto-played: %s", p)
	}
	if err := o.SetEqualizer([]float64{3, 2, 1}); err != nil {
		t.Fatal(err)
	}
	if err := o.Play(); err != nil {
		t.Fatal(err)
	}
	awaitAudio(t, func() bool { return o.Status().Position > 0 })
	o.Pause()
	time.Sleep(100 * time.Millisecond)
	before := o.Status().Position
	time.Sleep(100 * time.Millisecond)
	if after := o.Status().Position; after != before {
		t.Fatalf("pause advanced: %s -> %s", before, after)
	}
	o.Seek(2 * time.Second)
	awaitAudio(t, func() bool { p := o.Status().Position; return p >= 2*time.Second && p < 2200*time.Millisecond })
	if err := o.Play(); err != nil {
		t.Fatal(err)
	}
	awaitAudio(t, func() bool { return o.Status().Ended })
	o.Stop()
	if s := o.Status(); s.Position != 0 || s.Ended {
		t.Fatalf("stop=%+v", s)
	}
}
func awaitAudio(t *testing.T, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("audio state did not settle")
}
func TestWindowsFileRejectsMissingFile(t *testing.T) {
	t.Setenv("VIBEZ_NULL_AUDIO", "1")
	o, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if err := o.Open(filepath.Join(t.TempDir(), "missing.mp3")); err == nil {
		t.Fatal("missing file accepted")
	}
}
