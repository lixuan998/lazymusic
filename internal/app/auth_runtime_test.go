package app

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func fixtureRuntime(t *testing.T, header *tar.Header, content []byte) []byte {
	t.Helper()
	var payload bytes.Buffer
	compressed, err := zstd.NewWriter(&payload)
	if err != nil {
		t.Fatal(err)
	}
	tarball := tar.NewWriter(compressed)
	if err := tarball.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if len(content) > 0 {
		if _, err := tarball.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarball.Close(); err != nil {
		t.Fatal(err)
	}
	compressed.Close()
	var archive bytes.Buffer
	zipper := zip.NewWriter(&archive)
	file, err := zipper.Create("pkg-fixture.tar.zst")
	if err != nil {
		t.Fatal(err)
	}
	file.Write(payload.Bytes())
	zipper.Close()
	return archive.Bytes()
}

func TestAuthRuntimeExtractsOnlyTheRegularExpectedLibrary(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind byte
		ok   bool
	}{
		{"lib/libstdc++.so.6.0.33", tar.TypeReg, true},
		{"lib/libstdc++.so.6.0.33", tar.TypeSymlink, false},
		{"../../outside", tar.TypeReg, false},
		{"lib/libgcc_s.so.1", tar.TypeReg, false},
	} {
		t.Run(tc.name+string(tc.kind), func(t *testing.T) {
			dir := t.TempDir()
			content := []byte("fixture-library")
			h := &tar.Header{Name: tc.name, Typeflag: tc.kind, Size: int64(len(content)), Mode: 0600}
			if tc.kind == tar.TypeSymlink {
				h.Size, h.Linkname, content = 0, "../../outside", nil
			}
			err := extractAuthLibrary(fixtureRuntime(t, h, content), dir, "libstdc++.so.6")
			if (err == nil) != tc.ok {
				t.Fatalf("unexpected extraction result: %v", err)
			}
			if tc.ok {
				got, err := os.ReadFile(filepath.Join(dir, "libstdc++.so.6"))
				if err != nil || !bytes.Equal(got, content) {
					t.Fatal("library was not extracted intact")
				}
			}
		})
	}
}

func TestAuthRuntimeCacheRejectsIncompleteTamperedAndLinkedLibraries(t *testing.T) {
	dir := t.TempDir()
	if validAuthRuntime(dir) {
		t.Fatal("empty runtime cache accepted")
	}
	for _, p := range authRuntimePackages {
		if err := os.WriteFile(filepath.Join(dir, p.library), []byte("tampered"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if validAuthRuntime(dir) {
		t.Fatal("unverified runtime libraries accepted")
	}
	path := filepath.Join(dir, authRuntimePackages[0].library)
	os.Remove(path)
	if err := os.Symlink("/outside", path); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("Windows symlink privilege unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if validAuthRuntime(dir) {
		t.Fatal("linked runtime library accepted")
	}
}
