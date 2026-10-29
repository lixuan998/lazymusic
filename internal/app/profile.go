package app

import (
	"fmt"
	"os"
	"path/filepath"
)

func DefaultDir() (string, error) {
	if v := os.Getenv("LAZYMUSIC_STATE_DIR"); v != "" {
		return filepath.Abs(v)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lazymusic"), nil
}

func FindVibezHelper(dir string) (string, error) {
	if p := os.Getenv("LAZYMUSIC_VIBEZ_HELPER"); p != "" {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0111 != 0 {
			return p, nil
		}
		return "", fmt.Errorf("configured authorization component is unavailable")
	}
	path := filepath.Join(dir, "helpers", "vibez", "v2.0.6", "vibez")
	if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0111 != 0 {
		return path, nil
	}
	return "", fmt.Errorf("authorization component is not installed")
}
