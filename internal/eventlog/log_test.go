package eventlog

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHistoryIsBoundedPrivatePersistentAndDeduplicated(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, nil)
	s.Add("warn", "player", "same")
	s.Add("warn", "player", "same")
	if len(s.Entries()) != 1 {
		t.Fatal("duplicates not coalesced")
	}
	for i := 0; i < 510; i++ {
		s.Add("info", "player", fmt.Sprint(i))
	}
	if len(s.Entries()) != 500 {
		t.Fatal("unbounded ring")
	}
	fi, err := os.Stat(filepath.Join(dir, "runtime.log"))
	if err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0600) {
		t.Fatal("log permissions", err)
	}
	restored := New(dir, nil)
	if len(restored.Entries()) != 500 || restored.Entries()[499].Message != "509" {
		t.Fatal("history lost on restart")
	}
	restored.Add("info", "test", "next")
	e := restored.Entries()
	if e[499].ID <= e[498].ID {
		t.Fatal("IDs not increasing")
	}
}
func TestRedactionCoversSignedPathsQueriesAndTokens(t *testing.T) {
	secret := "opaque-user-value"
	s := New(t.TempDir(), func(v string) string { return strings.ReplaceAll(v, secret, "[REDACTED]") })
	s.Add("error", "apple", "Bearer private-token eyJabc.def.ghi https://localhost/stream/secret.aac?token=value "+secret)
	got := s.Entries()[0].Message
	for _, v := range []string{"private-token", "eyJabc", "secret.aac", "token=value", secret} {
		if strings.Contains(got, v) {
			t.Fatal("log leaked credential", v)
		}
	}
}
