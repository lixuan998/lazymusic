package account

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingVibezHelperDoesNotCreateProfile(t *testing.T) {
	state := filepath.Join(t.TempDir(), "not-created")
	if err := LoginViaVibez(context.Background(), state, filepath.Join(t.TempDir(), "missing"), 38747); err == nil {
		t.Fatal("missing helper was accepted")
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("unexpected profile creation: %v", err)
	}
}

func TestVibezLoginUsesIsolatedProfileAndRecordsApplicationIdentity(t *testing.T) {
	t.Setenv("LAZYMUSIC_DEVELOPER_TOKEN", "")
	dir := t.TempDir()
	helper := filepath.Join(dir, "helper")
	argsFile := filepath.Join(dir, "args")
	t.Setenv("LAZYMUSIC_TEST_HELPER_ARGS", argsFile)
	script := `#!/bin/sh
printf '%s\n' "$@" > "$LAZYMUSIC_TEST_HELPER_ARGS"
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--config" ]; then shift; profile=$1; fi
  shift
done
printf '%s' '{"apple_developer_token":"fixture.dev.token","apple_user_token":"fixture-user"}' > "$profile"
`
	if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dir, "isolated")
	if err := LoginViaVibez(context.Background(), state, helper, 38747); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "auth\nlogin\n--config\n"+filepath.Join(state, "credentials.json")) {
		t.Fatalf("helper did not receive explicit profile: %s", args)
	}
	b, err := os.ReadFile(filepath.Join(state, "credential-source.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"application_identity": "Vibez"`) || strings.Contains(string(b), "fixture.dev.token") {
		t.Fatalf("invalid credential provenance: %s", b)
	}
}
