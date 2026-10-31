package app

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractWindowsHelper(t *testing.T) {
	for _, name := range []string{"vibez.exe", "nested/vibez.exe"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			f, err := os.CreateTemp(dir, "archive-*")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			z := zip.NewWriter(f)
			w, err := z.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			w.Write([]byte("test helper"))
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			dest, err := extractAuthHelper(f, dir, true)
			if err != nil {
				t.Fatal(err)
			}
			if dest != filepath.Join(dir, "vibez.exe") {
				t.Fatal(dest)
			}
			got, err := os.ReadFile(dest)
			if err != nil || string(got) != "test helper" {
				t.Fatalf("helper=%q %v", got, err)
			}
		})
	}
}
func TestExtractWindowsHelperRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "archive-*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z := zip.NewWriter(f)
	h := &zip.FileHeader{Name: "vibez.exe"}
	h.SetMode(os.ModeSymlink | 0777)
	w, err := z.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("outside"))
	z.Close()
	if _, err := extractAuthHelper(f, dir, true); err == nil {
		t.Fatal("symlink helper accepted")
	}
}
