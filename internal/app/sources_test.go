package app

import (
	tea "charm.land/bubbletea/v2"
	"lazymusic/internal/backend"
	"lazymusic/internal/music"
	"os"
	"path/filepath"
	"testing"
)

func TestLoginSelectionIncludesLocalAndDoesNotSelectImplicitly(t *testing.T) {
	m := &loginPage{entries: backend.Sources(), phase: "sources"}
	if len(m.entries) != 2 || m.source != "" {
		t.Fatal("source menu selected a platform before user input")
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.source != music.Local {
		t.Fatal("menu did not select LOCAL")
	}
}
func TestSourceProfilesAreSeparateAndExplicitProfileStaysExact(t *testing.T) {
	base := t.TempDir()
	if err := SaveSource(base, music.Local); err != nil {
		t.Fatal(err)
	}
	dir, source, err := ResolveProfile(base, false, "")
	if err != nil || source != music.Local || dir != filepath.Join(base, "profiles", "local") {
		t.Fatalf("default local profile: %s %s %v", dir, source, err)
	}
	dir, source, err = ResolveProfile(base, false, music.Apple)
	if err != nil || dir != base || source != music.Apple {
		t.Fatal("Apple legacy profile moved")
	}
	custom := t.TempDir()
	_ = os.WriteFile(filepath.Join(custom, "local.json"), []byte(`{"root":"/tmp"}`), 0600)
	dir, source, err = ResolveProfile(custom, true, "")
	if err != nil || dir != custom || source != music.Local {
		t.Fatal("explicit source profile was nested or sent to Apple")
	}
}

func TestExplicitProfileCannotChangeSourceBehindExistingSession(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "session.json"), []byte(`{"backend":"apple"}`), 0600)
	if _, _, err := ResolveProfile(dir, true, music.Local); err == nil {
		t.Fatal("LOCAL would reuse Apple credentials/session directory")
	}
}
