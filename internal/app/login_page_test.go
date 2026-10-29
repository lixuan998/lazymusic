package app

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"lazymusic/internal/backend"
	"os"
	"path/filepath"
	"testing"
)

func TestInAppLoginSelectsFolderWithoutAuthorizationCommands(t *testing.T) {
	profile, root := t.TempDir(), t.TempDir()
	m := &loginPage{ctx: context.Background(), opts: LaunchOptions{Base: profile, Dir: profile, Explicit: true}, entries: backend.Sources(), phase: "sources"}
	_, _ = m.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.phase != "folder" || m.source != "local" {
		t.Fatal("in-app Login did not open LOCAL folder page")
	}
	m.root = root
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("folder confirmation did not configure source")
	}
	m.Update(cmd())
	if !m.complete {
		t.Fatal(m.problem)
	}
	if _, err := os.Stat(filepath.Join(profile, "local.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(profile, "credentials.json")); !os.IsNotExist(err) {
		t.Fatal("LOCAL created Apple credentials")
	}
}
func TestInAppLoginEscCanReturnToExistingSession(t *testing.T) {
	m := &loginPage{phase: "sources", allowBack: true}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !m.back || m.complete {
		t.Fatal("Esc did not return to prior session")
	}
}
