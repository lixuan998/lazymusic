package app

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"lazymusic/internal/account"

	"github.com/klauspost/compress/zstd"
)

// These stable conda-forge GCC runtimes target glibc 2.17. They are used only
// by the official Linux auth helper when the host lacks its C++ ABI; the TUI,
// daemon and browser do not inherit this library path. No system packages change.
var authRuntimePackages = []struct{ name, digest, library, libraryDigest string }{
	{"libstdcxx-14.3.0-h934c35e_20.conda", "83447d514a21052674806585721fef05979dd6700ca199dbfdef713b6113cb2b", "libstdc++.so.6", "7b366da4a7cde5b7d52f51b29693b949c625e7a4646dd6b823d2f6c450b3bc77"},
	{"libgcc-14.3.0-he0feb66_20.conda", "ee549953022da53692ae9a5da17a5d4a1829b9a173b68433e529279f0d2f9305", "libgcc_s.so.1", "ea6c8a14912794456c75cf4e5f13a0831282e68a19c548fd695cd688143dff5d"},
}

func prepareAuthRuntime(ctx context.Context, dir, helper string, report func(string)) (string, error) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return "", nil
	}
	err := account.CheckLoginHelper(ctx, helper, "")
	if !errors.Is(err, account.ErrHelperRuntime) {
		return "", err
	}
	folder := filepath.Join(dir, "helpers", "runtime", "gcc-14.3.0-20-linux-amd64")
	if !validAuthRuntime(folder) {
		report("Preparing compatible sign-in runtime…")
		if err := installAuthRuntime(ctx, folder); err != nil {
			return "", err
		}
	}
	if err := account.CheckLoginHelper(ctx, helper, folder); err != nil {
		return "", err
	}
	return folder, nil
}

func validAuthRuntime(dir string) bool {
	if fi, err := os.Lstat(dir); err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		return false
	}
	for _, p := range authRuntimePackages {
		path := filepath.Join(dir, p.library)
		fi, err := os.Lstat(path)
		if err != nil || !fi.Mode().IsRegular() || fi.Size() > 32<<20 {
			return false
		}
		b, err := os.ReadFile(path)
		if err != nil || !matchesDigest(b, p.libraryDigest) {
			return false
		}
	}
	return true
}

func installAuthRuntime(ctx context.Context, folder string) error {
	parent := filepath.Dir(folder)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".runtime-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	client := &http.Client{Timeout: 90 * time.Second}
	for _, p := range authRuntimePackages {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://conda.anaconda.org/conda-forge/linux-64/"+p.name, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("could not download sign-in compatibility runtime; check the network/proxy and retry")
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20+1))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || readErr != nil || len(data) > 8<<20 {
			return fmt.Errorf("sign-in compatibility runtime download failed (HTTP %d); retry", resp.StatusCode)
		}
		if !matchesDigest(data, p.digest) {
			return fmt.Errorf("sign-in compatibility runtime checksum mismatch")
		}
		if err := extractAuthLibrary(data, stage, p.library); err != nil {
			return err
		}
	}
	if !validAuthRuntime(stage) {
		return fmt.Errorf("sign-in compatibility library checksum mismatch")
	}
	if err := os.MkdirAll(folder, 0700); err != nil {
		return err
	}
	if fi, err := os.Lstat(folder); err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("sign-in compatibility runtime directory is invalid")
	}
	for _, p := range authRuntimePackages {
		if err := os.Rename(filepath.Join(stage, p.library), filepath.Join(folder, p.library)); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(folder, "NOTICE"), []byte("GCC 14.3.0 runtime libraries from conda-forge, build 20.\nLicense: GPL-3.0-only WITH GCC-exception-3.1.\nSource: https://github.com/conda-forge/ctng-compilers-feedstock\nGCC source: https://gcc.gnu.org/releases.html\nLicense: https://www.gnu.org/licenses/gpl-3.0.html\nRuntime exception: https://www.gnu.org/licenses/gcc-exception-3.1.html\nOnly the Apple sign-in helper loads these libraries.\n"), 0600)
}

func matchesDigest(data []byte, digest string) bool {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) == digest
}

// Extract a single allowlisted regular library. Package links, paths, scripts
// and metadata are never installed or executed. Both archive and library hashes
// are pinned independently. An interrupted install cannot make the cache valid.
func extractAuthLibrary(data []byte, folder, library string) error {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("invalid sign-in runtime package")
	}
	for _, file := range archive.File {
		if !strings.HasPrefix(file.Name, "pkg-") || !strings.HasSuffix(file.Name, ".tar.zst") {
			continue
		}
		compressed, err := file.Open()
		if err != nil {
			return err
		}
		decoder, err := zstd.NewReader(compressed, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(64<<20), zstd.WithDecoderMaxWindow(32<<20))
		if err != nil {
			compressed.Close()
			return err
		}
		reader := tar.NewReader(io.LimitReader(decoder, 128<<20))
		for {
			h, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				decoder.Close()
				compressed.Close()
				return err
			}
			wanted := h.Name == "lib/libgcc_s.so.1" && library == "libgcc_s.so.1" || h.Name == "lib/libstdc++.so.6.0.33" && library == "libstdc++.so.6"
			if !wanted || h.Typeflag != tar.TypeReg {
				continue
			}
			if h.Size <= 0 || h.Size > 32<<20 {
				decoder.Close()
				compressed.Close()
				return fmt.Errorf("invalid sign-in runtime library size")
			}
			b, err := io.ReadAll(reader)
			decoder.Close()
			compressed.Close()
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(folder, library), b, 0600)
		}
		decoder.Close()
		compressed.Close()
	}
	return fmt.Errorf("sign-in runtime package did not contain its library")
}
