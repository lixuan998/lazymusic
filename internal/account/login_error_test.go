package account

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestHelperFailuresAreUsefulWithoutForwardingPrivateOutput(t *testing.T) {
	for _, tc := range []struct{ output, want string }{
		{"/private/helper: /lib/libstdc++.so.6: version `GLIBCXX_3.4.32' not found", "GLIBCXX_3.4.32"},
		{"helper: error while loading shared libraries: libgstreamer-1.0.so.0: cannot open shared object file", "missing libgstreamer-1.0.so.0"},
		{"starting auth server: bind: address already in use", "port is already in use"},
		{"fetching developer token from broker: Get https://private.example/SECRET: x509: certificate signed by unknown authority", "certificate verification failed"},
		{"fetching developer token from broker: lookup private.example: no such host", "DNS lookup failed"},
		{"fetching developer token from broker: context deadline exceeded", "timed out"},
		{"fetching developer token from broker: connection refused", "could not be reached"},
		{"fetching developer token from broker: token broker returned 503 Service Unavailable", "HTTP 503"},
		{"fetching developer token from broker: SECRET", "token service failed"},
	} {
		err := helperProblem(tc.output)
		if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "private.example") {
			t.Fatalf("unexpected sanitized diagnostic for %q: %v", tc.want, err)
		}
	}
	if !errors.Is(helperProblem("version `GLIBCXX_3.4.32' not found"), ErrHelperRuntime) {
		t.Fatal("C++ ABI failure cannot trigger the isolated compatibility runtime")
	}
	if errors.Is(helperProblem("version `GLIBC_2.38' not found"), ErrHelperRuntime) {
		t.Fatal("C++ runtime cannot repair glibc incompatibility")
	}
	if helperProblem("unrecognized failure with token=SECRET") != nil {
		t.Fatal("unknown raw helper output was exposed")
	}
}

func TestQuietLoginReportsStderrAndSafeErrors(t *testing.T) {
	t.Setenv("LAZYMUSIC_DEVELOPER_TOKEN", "")
	dir := t.TempDir()
	helper := filepath.Join(dir, "helper")
	script := "#!/bin/sh\nprintf 'private token=SECRET\\n'\nprintf \"helper: version GLIBCXX_3.4.32 not found\" >&2\nexit 127\n"
	if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	err := LoginViaVibezQuiet(context.Background(), filepath.Join(dir, "profile"), helper, 38747)
	if !errors.Is(err, ErrHelperRuntime) || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("stderr failure was discarded or leaked: %v", err)
	}
}

func TestHelperLibraryPathDoesNotEscapeToBrowserEnvironment(t *testing.T) {
	t.Setenv("LD_LIBRARY_PATH", "/existing")
	cmd := exec.Command("/private/helper with spaces", "auth", "login")
	setHelperLibrary(cmd, "/private/runtime with spaces")
	want := []string{"/lib64/ld-linux-x86-64.so.2", "--library-path", "/private/runtime with spaces", "/private/helper with spaces", "auth", "login"}
	if cmd.Env != nil || !reflect.DeepEqual(cmd.Args, want) || cmd.Path != want[0] {
		t.Fatal("runtime setup exported the library path or changed argument boundaries")
	}
	if os.Getenv("LD_LIBRARY_PATH") != "/existing" {
		t.Fatal("compatibility libraries leaked to the TUI/daemon environment")
	}
}

func TestCanceledHelperDoesNotReportAnAuthorizationFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := CheckLoginHelper(ctx, "/unused", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel became a runtime failure: %v", err)
	}
}
