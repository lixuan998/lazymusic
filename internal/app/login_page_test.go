package app

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"lazymusic/internal/backend"
	"lazymusic/internal/music"
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

func TestManualLoginURLIsVisibleInTheSmallestSupportedFrame(t *testing.T) {
	for _, width := range []int{60, 80, 100} {
		a := keyboardApplication()
		a.music.OpenLogin()
		a.Update(tea.WindowSizeMsg{Width: width, Height: 16})
		a.login.phase, a.login.busy = "account", true
		a.login.loginURL = "http://localhost:38747/login?state=0123456789abcdef0123456789abcdef"
		view := ansi.Strip(a.View().Content)
		for _, chunk := range strings.Split(ansi.Hardwrap(a.login.loginURL, a.music.ContentWidth(), false), "\n") {
			if !strings.Contains(view, chunk) {
				t.Fatalf("login link is cropped at %dx16", width)
			}
		}
		_, cmd := a.Update(press("y"))
		if cmd == nil || fmt.Sprint(cmd()) != a.login.loginURL {
			t.Fatal("copy did not preserve the entire unwrapped authorization URL")
		}
	}
}

func TestLoginPreservesAndWrapsActionableFailureReason(t *testing.T) {
	m := &loginPage{phase: "account", generation: 2, busy: true, loginURL: "expired"}
	m.Update(loginDone{generation: 2, err: fmt.Errorf("Apple sign-in service DNS lookup failed; check the WSL network and DNS settings")})
	rows := m.RowsForWidth(28)
	joined := ansi.Strip(strings.Join(rows, " "))
	if !strings.Contains(joined, "DNS") || strings.Contains(joined, "expired") || strings.Contains(joined, "did not complete; try again") {
		t.Fatal("failure lost its actionable diagnostic or retained an expired URL")
	}
	for _, row := range rows[2 : len(rows)-2] {
		if ansi.StringWidth(row) > 28 {
			t.Fatal("failure reason is truncated in narrow login content")
		}
	}
}

func TestAppleLoginKeepsTheHelpersSanitizedFailure(t *testing.T) {
	helper := filepath.Join(t.TempDir(), "helper")
	script := "#!/bin/sh\n[ \"$1\" = version ] && exit 0\nprintf 'x509: certificate signed by unknown authority; token=SECRET\\n' >&2\nexit 1\n"
	if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAZYMUSIC_VIBEZ_HELPER", helper)
	t.Setenv("LAZYMUSIC_DEVELOPER_TOKEN", "")
	m := &loginPage{ctx: context.Background(), phase: "account", source: music.Apple, dir: t.TempDir()}
	batch, ok := m.connect(false)().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatal("Apple sign-in did not start its isolated worker and progress stream")
	}
	m.Update(batch[0]())
	if m.busy || m.complete || !strings.Contains(m.problem, "certificate verification failed") || strings.Contains(m.problem, "SECRET") {
		t.Fatalf("sign-in lost its safe failure reason: %s", m.problem)
	}
}

func TestLoginCopyUsesTheCurrentPanelBinding(t *testing.T) {
	a := keyboardApplication()
	a.music.OpenLogin()
	a.login.phase, a.login.busy, a.login.loginURL = "account", true, "fixture-link"
	a.keys["login_copy"] = []string{"Y"}
	if _, cmd := a.Update(press("y")); cmd != nil {
		t.Fatal("removed default copy binding still performed an action")
	}
	_, cmd := a.Update(press("Y"))
	if cmd == nil || fmt.Sprint(cmd()) != "fixture-link" {
		t.Fatal("custom Login copy binding did not reach the clipboard action")
	}
}
