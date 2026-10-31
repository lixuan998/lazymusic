package nvimconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCreateUpdateAndBackupAreIdempotent(t *testing.T) {
	dir := t.TempDir()
	plugin, binary := filepath.Join(dir, "shared plugin"), filepath.Join(dir, "bin", "lazymusic")
	first, err := Write(dir, plugin, binary)
	if err != nil || !first.Changed || first.Backup != "" {
		t.Fatalf("initial install: %+v %v", first, err)
	}
	content, _ := os.ReadFile(first.Path)
	if !strings.Contains(string(content), `dir = `+luaString(plugin)) || !strings.Contains(string(content), `"<leader>mm"`) {
		t.Fatal("generated spec does not use the installed plugin or requested key")
	}
	second, err := Write(dir, plugin, binary)
	if err != nil || second.Changed || second.Backup != "" {
		t.Fatalf("repeated install changed config: %+v %v", second, err)
	}
	custom := []byte("return { custom = true }\n")
	if err := os.WriteFile(first.Path, custom, 0600); err != nil {
		t.Fatal(err)
	}
	update, err := Write(dir, plugin, binary)
	if err != nil || !update.Changed || update.Backup == "" {
		t.Fatalf("update did not back up config: %+v %v", update, err)
	}
	backup, _ := os.ReadFile(update.Backup)
	if string(backup) != string(custom) {
		t.Fatal("original Neovim config was not preserved")
	}
}

func TestExistingConfigSymlinkKeepsItsTargetAndLink(t *testing.T) {
	dir := t.TempDir()
	plugins := filepath.Join(dir, "lua", "plugins")
	if err := os.MkdirAll(plugins, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "dotfile.lua")
	if err := os.WriteFile(target, []byte("-- previous config"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(plugins, "lazymusic.lua")
	if err := os.Symlink(target, path); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("Windows symlink privilege unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if _, err := Write(dir, "/installed/plugin", "/installed/bin/lazymusic"); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Lstat(path)
	content, _ := os.ReadFile(target)
	if info.Mode()&os.ModeSymlink == 0 || !strings.Contains(string(content), "<leader>mm") {
		t.Fatal("config symlink was replaced or target was not updated")
	}
}

func TestLuaPathQuotingPreservesSpecialCharacters(t *testing.T) {
	got := luaString("/Music/中文/\"quoted\"/back\\slash\n")
	if got != "\"/Music/中文/\\\"quoted\\\"/back\\\\slash\\010\"" {
		t.Fatalf("invalid Lua string: %s", got)
	}
}

func TestHelpIndexIsReadableAndIncludesMainTag(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lazymusic.txt"), []byte("*lazymusic.txt* *lazymusic*\n*setup-music*"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteHelpTags(dir); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(filepath.Join(dir, "tags"))
	info, _ := os.Stat(filepath.Join(dir, "tags"))
	if !strings.Contains(string(content), "lazymusic\tlazymusic.txt\t/*lazymusic*\n") || (runtime.GOOS != "windows" && info.Mode().Perm() != 0644) {
		t.Fatal("installed help cannot be found or read")
	}
}
