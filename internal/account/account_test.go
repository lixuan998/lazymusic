package account

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMissingCredentialsCannotAuthorize(t *testing.T) {
	t.Setenv("LAZYMUSIC_DEVELOPER_TOKEN", "")
	dir := filepath.Join(t.TempDir(), "absent")
	cfg, err := ReadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ValidateAccount(context.Background(), cfg); err == nil {
		t.Fatal("missing credentials accepted")
	}
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("credential read mutated profile")
	}
}
func TestCredentialOverrideAndLogout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(path, []byte(`{"apple_developer_token":"stored","apple_user_token":"user"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAZYMUSIC_DEVELOPER_TOKEN", "override")
	cfg, err := ReadConfig(dir)
	if err != nil || cfg.AppleDeveloperToken != "override" || cfg.AppleUserToken != "user" {
		t.Fatal("credential loading failed", err)
	}
	if err = Logout(dir); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("logout kept credentials")
	}
	if err = Logout(dir); err != nil {
		t.Fatal("logout not idempotent", err)
	}
}
func TestCorruptCredentialsAreNotSilentlyAccepted(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "credentials.json"), []byte("{"), 0600)
	if _, err := ReadConfig(dir); err == nil {
		t.Fatal("corrupt credentials ignored")
	}
}
