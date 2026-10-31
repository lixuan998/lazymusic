package account

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestLoginURLWriterOnlyReportsExactLocalAuthorizationLink(t *testing.T) {
	var urls []string
	w := &loginURLWriter{port: 38747, onURL: func(url string) { urls = append(urls, url) }}
	for _, part := range []string{
		"private helper output must never be forwarded\n",
		"http://example.com:38747/login?state=0123456789abcdef\n",
		"http://localhost:9999/login?state=0123456789abcdef\n",
		"http://localhost:38747/login?state=0123456789abcdef&token=hidden\n",
		"If your browser did not open, visit:\n  http://local",
		"host:38747/login?state=0123456789abcdef\n",
		"http://localhost:38747/login?state=0123456789abcdef\n",
	} {
		if n, err := w.Write([]byte(part)); err != nil || n != len(part) {
			t.Fatal("writer did not consume helper output")
		}
	}
	if !reflect.DeepEqual(urls, []string{"http://localhost:38747/login?state=0123456789abcdef"}) {
		t.Fatalf("unexpected authorization links: %v", urls)
	}
}

func TestQuietLoginReportsFallbackURLAndStillImportsCredentials(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture requires a POSIX shell")
	}
	t.Setenv("LAZYMUSIC_DEVELOPER_TOKEN", "")
	dir := t.TempDir()
	helper := filepath.Join(dir, "helper")
	script := `#!/bin/sh
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--config" ]; then shift; profile=$1; fi
  shift
done
printf 'If your browser did not open, visit:\n  http://localhost:38747/login?state=0123456789abcdef\n'
printf '%s' '{"apple_developer_token":"fixture.dev.token","apple_user_token":"fixture-user"}' > "$profile"
`
	if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	var got string
	err := LoginViaVibezProgress(context.Background(), filepath.Join(dir, "profile"), helper, 38747, func(url string) { got = url }, "")
	if err != nil || got != "http://localhost:38747/login?state=0123456789abcdef" {
		t.Fatalf("fallback link was not reported: %v", err)
	}
}

func TestFallbackURLSurvivesColoredStderrAndMissingFinalNewline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture requires a POSIX shell")
	}
	t.Setenv("LAZYMUSIC_DEVELOPER_TOKEN", "")
	dir := t.TempDir()
	helper := filepath.Join(dir, "helper")
	script := `#!/bin/sh
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--config" ]; then shift; profile=$1; fi
  shift
done
printf '\033[32mhttp://localhost:38747/login?state=0123456789abcdef\033[0m' >&2
printf '%s' '{"apple_developer_token":"fixture.dev.token","apple_user_token":"fixture-user"}' > "$profile"
`
	if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	var got string
	err := LoginViaVibezProgress(context.Background(), filepath.Join(dir, "profile"), helper, 38747, func(url string) { got = url }, "")
	if err != nil || got != "http://localhost:38747/login?state=0123456789abcdef" {
		t.Fatalf("stderr/EOF authorization link was lost: %v", err)
	}
}

func TestOversizedHelperLineIsDiscardedUntilItsBoundary(t *testing.T) {
	var got string
	w := &loginURLWriter{port: 38747, onURL: func(url string) { got = url }}
	w.Write(make([]byte, 8193))
	w.Write([]byte("http://localhost:38747/login?state=0123456789abcdef\r"))
	if got != "" {
		t.Fatal("an oversized private output line was partially forwarded")
	}
	w.Write([]byte("http://localhost:38747/login?state=0123456789abcdef\r"))
	if got == "" {
		t.Fatal("writer did not recover at the next output line")
	}
}
