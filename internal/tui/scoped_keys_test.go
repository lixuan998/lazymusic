package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestHelpAndDispatchUseOnlyFocusedScope(t *testing.T) {
	m, api := testModel()
	for _, tc := range []struct {
		focus        int
		want, absent string
	}{
		{0, "Navigation", "Play song / entire collection"},
		{1, "Content", "Move song up"},
		{2, "Queue", "Refresh content"},
		{3, "Player", "Filter loaded items"},
		{4, "Log", "Play song / entire collection"},
	} {
		m.UI.Focus = tc.focus
		rows := ansi.Strip(strings.Join(m.helpLines(), "\n"))
		if !strings.Contains(rows, tc.want) || !strings.Contains(rows, "Global") || strings.Contains(rows, tc.absent) {
			t.Fatalf("scope %d help: %s", tc.focus, rows)
		}
	}
	for _, focus := range []int{0, 2, 3, 4, 5} {
		m.UI.Focus = focus
		m.normal("/")
		m.normal("i")
		if m.mode != "" || m.UI.Focus != focus {
			t.Fatal("inactive Content shortcuts changed a different panel")
		}
	}
	m.UI.Focus = 3
	cmd := m.normal("r")
	cmd()
	if api.lastCommand.Op != "repeat" {
		t.Fatal("Player r did not repeat")
	}
	m.UI.Focus = 1
	m.normal("r")
	if !m.loading || api.lastCommand.Op != "repeat" {
		t.Fatal("Content r did not refresh independently")
	}
}

func TestKeyConfigurationAllowsReuseOnlyAcrossDisjointScopes(t *testing.T) {
	for _, tc := range []struct {
		config string
		valid  bool
	}{
		{`{}`, true},
		{`{"query":["i"],"eq_reset":["i"]}`, true},
		{`{"refresh":["a"]}`, false},
		{`{"refresh":["space"]}`, false},
		{`{"leave":["ctrl+q"],"refresh":["q"]}`, true},
		{`{"queue_focus_playing":["/"]}`, true},
		{`{"queue_focus_playing":["space"]}`, false},
		{`{"queue_focus_playing":["d"]}`, false},
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "keys.json"), []byte(tc.config), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := LoadKeys(dir)
		if (err == nil) != tc.valid {
			t.Fatalf("%s: %v", tc.config, err)
		}
	}
}

func TestPlaybackCommandsAreGlobalInEveryPanel(t *testing.T) {
	for focus := 0; focus < 6; focus++ {
		for key, op := range map[string]string{"space": "toggle", "x": "stop", "<": "previous", ">": "next", "ctrl+<": "seek", "ctrl+>": "seek", "m": "playback-mode"} {
			m, api := testModel()
			m.UI.Focus = focus
			m.State.Source.Capabilities.Equalizer = true
			m.UI.Visualizer = 2
			cmd := m.normal(key)
			if cmd == nil {
				t.Fatalf("panel %d does not handle global %s", focus, key)
			}
			cmd()
			if api.lastCommand.Op != op || m.UI.Focus != focus {
				t.Fatalf("panel %d %s: %+v", focus, key, api.lastCommand)
			}
		}
	}
}

func TestHelpScrollsWithinSmallTerminal(t *testing.T) {
	m, _ := testModel()
	m.width, m.height = 60, 16
	m.normal("?")
	m.helpKey("G")
	v := m.View().Content
	if len(strings.Split(v, "\n")) != 16 || m.helpScroll == 0 {
		t.Fatal("help cannot scroll to Global commands")
	}
	for _, row := range strings.Split(v, "\n") {
		if ansi.StringWidth(row) > 60 {
			t.Fatal("help overflows terminal")
		}
	}
}

func TestModifiedChordsNeverBecomeFilterText(t *testing.T) {
	for _, wire := range []string{
		"\x1b[106;6u", "\x1b[74;6u", "\x1b[106:74;6u", "\x1b[106;6;47u",
		"\x1b[106:74;6;47:116:109:112u", "\x1b[27;6;74~",
	} {
		m, _ := testModel()
		m.UI.Focus = 0
		msg := decodeKey(t, wire)
		m.Update(msg)
		if m.UI.Focus != 0 || m.mode != "" || m.draft != "" {
			t.Fatalf("resize became input: %q, %q, %+v", wire, KeyString(msg), m.UI)
		}
	}
	for _, direction := range []string{"H", "J", "K", "L"} {
		m, _ := testModel()
		m.UI.Focus = 0
		m.normal(direction)
		if m.UI.NavWidth == 0 && m.UI.NavHeight == 0 {
			t.Fatal("Shift resize did not resize")
		}
		if m.mode != "" || m.UI.Focus != 0 {
			t.Fatal("Shift resize changed focus/input mode")
		}
	}
}

func TestUppercaseResizeLettersRemainTextWhileEditing(t *testing.T) {
	for _, mode := range []string{"query", "filter", "command"} {
		m, _ := testModel()
		m.mode = mode
		for _, r := range "HJKL?" {
			m.Update(key(string(r)))
		}
		if m.draft != "HJKL?" || m.UI.NavWidth != 0 || m.UI.NavHeight != 0 || m.help {
			t.Fatalf("%s text was treated as commands: %q", mode, m.draft)
		}
	}
}
