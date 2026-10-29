package account

import (
	"os"
	"path/filepath"
	"runtime"
)

func CDMPath(stateDir string) string {
	if p := os.Getenv("LAZYMUSIC_CDM_PATH"); p != "" {
		return p
	}
	name := "libwidevinecdm.so"
	if runtime.GOOS == "darwin" {
		name = "libwidevinecdm.dylib"
	}
	return filepath.Join(stateDir, "cdm", name)
}
