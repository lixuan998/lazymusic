package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAuthorizationHelperOnlyUsesExplicitOrPrivateRuntimePath(t *testing.T) {
	t.Setenv("LAZYMUSIC_VIBEZ_HELPER", "")
	root := t.TempDir()
	if _, err := FindVibezHelper(root); err == nil {
		t.Fatal("missing helper accepted")
	}
	helper := filepath.Join(root, "helpers", "vibez", "v2.0.6", helperName())
	if err := os.MkdirAll(filepath.Dir(helper), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helper, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	if p, err := FindVibezHelper(root); err != nil || p != helper {
		t.Fatal(p, err)
	}
	t.Setenv("LAZYMUSIC_VIBEZ_HELPER", filepath.Join(root, "missing"))
	if _, err := FindVibezHelper(root); err == nil {
		t.Fatal("invalid explicit helper silently ignored")
	}
}
