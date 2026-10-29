package app

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"lazymusic/internal/backend"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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

func TestManualLoginLinkIsWrappedAndClearedOnCancel(t *testing.T) {
	events := make(chan loginProgress, 1)
	m := &loginPage{phase: "account", source: "apple", busy: true, generation: 4, progress: events}
	link := "http://localhost:38747/login?state=0123456789abcdef0123456789abcdef"
	m.Update(loginProgress{generation: 4, stream: events, status: "Complete Apple authorization.", url: link})
	rows := m.RowsForWidth(24)
	if !strings.Contains(ansi.Strip(strings.Join(rows, "")), link) {
		t.Fatal("manual authorization URL was truncated rather than wrapped")
	}
	for _, line := range rows {
		if strings.Contains(line, "localhost") && ansi.StringWidth(line) > 24 {
			t.Fatal("login link exceeds actual content width")
		}
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.loginURL != "" || m.busy {
		t.Fatal("cancel retained an expired authorization URL")
	}
	m.Update(loginProgress{generation: 4, stream: events, url: link})
	if m.loginURL != "" {
		t.Fatal("late progress restored a canceled login link")
	}
}

func TestLoginProgressCannotCrossIntoAnotherAttemptWithSameGeneration(t *testing.T) {
	previous, current := make(chan loginProgress), make(chan loginProgress)
	m := &loginPage{phase: "account", source: "apple", busy: true, generation: 1, progress: current}
	m.Update(loginProgress{generation: 1, stream: previous, url: "previous-url"})
	if m.loginURL != "" {
		t.Fatal("old login attempt contaminated a new source selection")
	}
}

func TestResetLoginCancelsOldAttemptAndAdvancesGeneration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a := &application{ctx: context.Background(), login: &loginPage{generation: 7, cancel: cancel}}
	a.resetLogin()
	if ctx.Err() == nil || a.login.generation <= 7 {
		t.Fatal("reset retained an active old sign-in or reused its generation")
	}
	a.login.Update(loginDone{generation: 7, source: "apple", dir: "old-profile"})
	if a.login.complete {
		t.Fatal("a late login result completed the replacement Login page")
	}
}
