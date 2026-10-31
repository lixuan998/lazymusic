package app

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// ensureAuthHelper downloads only the pinned official release and verifies the
// published archive digest before installing its single executable.
func ensureAuthHelper(ctx context.Context, dir string) (string, error) {
	if path, err := FindVibezHelper(dir); err == nil {
		return path, nil
	}
	digests := map[string]string{
		"darwin_arm64":  "ccfa68e0b76df89307d0056725196720f8ab6163c1c32df16dcb890acf09d839",
		"darwin_amd64":  "a04c17ed153fb0651dd1267067355b29c9566c543ee76ced45ba0c2b9c79c74c",
		"windows_amd64": "dc19d322de1e903d3964d2b3b97bb01421174f38d5dc7ba68fb2ebc1c804d777",
		"linux_amd64":   "d68cd38f465218424553fba6df34eb2211f8cf647bc34136608639093aef521d",
	}
	platform := runtime.GOOS + "_" + runtime.GOARCH
	digest, ok := digests[platform]
	if !ok {
		return "", fmt.Errorf("Apple sign-in is unavailable on this architecture")
	}
	extension := ".tar.gz"
	if runtime.GOOS == "windows" {
		extension = ".zip"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://github.com/simonepelosi/vibez/releases/download/v2.0.6/vibez_"+platform+extension, nil)
	if err != nil {
		return "", err
	}
	response, err := (&http.Client{Timeout: 90 * time.Second}).Do(request)
	if err != nil {
		return "", fmt.Errorf("could not download Apple sign-in component; check the network and retry")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", fmt.Errorf("Apple sign-in component download failed (HTTP %d)", response.StatusCode)
	}
	folder := filepath.Join(dir, "helpers", "vibez", "v2.0.6")
	if err = os.MkdirAll(folder, 0700); err != nil {
		return "", err
	}
	archive, err := os.CreateTemp(folder, ".release-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(archive, hash), io.LimitReader(response.Body, 64<<20+1))
	if err != nil || size > 64<<20 {
		return "", fmt.Errorf("incomplete or oversized Apple sign-in component")
	}
	if hex.EncodeToString(hash.Sum(nil)) != digest {
		return "", fmt.Errorf("Apple sign-in component checksum mismatch")
	}
	if _, err = archive.Seek(0, 0); err != nil {
		return "", err
	}
	return extractAuthHelper(archive, folder, runtime.GOOS == "windows")
}

func extractAuthHelper(archive *os.File, folder string, zipped bool) (string, error) {
	if zipped {
		fi, err := archive.Stat()
		if err != nil {
			return "", err
		}
		zr, err := zip.NewReader(archive, fi.Size())
		if err != nil {
			return "", err
		}
		for _, entry := range zr.File {
			if filepath.Base(entry.Name) != "vibez.exe" || !entry.Mode().IsRegular() {
				continue
			}
			if entry.UncompressedSize64 == 0 || entry.UncompressedSize64 > 50<<20 {
				return "", fmt.Errorf("invalid Apple sign-in executable")
			}
			reader, err := entry.Open()
			if err != nil {
				return "", err
			}
			defer reader.Close()
			return installAuthExecutable(reader, int64(entry.UncompressedSize64), folder, "vibez.exe")
		}
	} else {
		gz, err := gzip.NewReader(archive)
		if err != nil {
			return "", err
		}
		defer gz.Close()
		reader := tar.NewReader(gz)
		for {
			header, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", err
			}
			if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != "vibez" {
				continue
			}
			return installAuthExecutable(reader, header.Size, folder, "vibez")
		}
	}
	return "", fmt.Errorf("Apple sign-in component did not contain its executable")
}

func installAuthExecutable(reader io.Reader, size int64, folder, name string) (string, error) {
	if size <= 0 || size > 50<<20 {
		return "", fmt.Errorf("invalid Apple sign-in executable")
	}
	file, err := os.CreateTemp(folder, ".helper-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0700); err == nil {
		_, err = io.CopyN(file, reader, size)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	dest := filepath.Join(folder, name)
	if err := os.Rename(file.Name(), dest); err != nil {
		return "", err
	}
	return dest, nil
}
