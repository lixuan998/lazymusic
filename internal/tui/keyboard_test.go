package tui

import (
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"testing"
)

// Exercise terminal wire encodings rather than only synthetic shortcut strings.
func decodeKey(t *testing.T, wire string) tea.KeyPressMsg {
	t.Helper()
	var decoder uv.EventDecoder
	n, event := decoder.Decode([]byte(wire))
	key, ok := event.(uv.KeyPressEvent)
	if !ok || n != len(wire) {
		t.Fatalf("invalid key event: %T", event)
	}
	return tea.KeyPressMsg(key)
}
func TestQuestionMarkOpensHelpAndSlashOpensFilter(t *testing.T) {
	for _, wire := range []string{"?", "\x1b[63;2u", "\x1b[47:63;2u", "\x1b[47;2;63u"} {
		m, _ := testModel()
		m.Update(decodeKey(t, wire))
		if !m.help || m.mode != "" {
			t.Fatalf("question mark entered %q instead of help (%q)", m.mode, wire)
		}
		m.Update(decodeKey(t, wire))
		if m.help {
			t.Fatal("question mark did not close help")
		}
	}
	m, _ := testModel()
	m.Update(decodeKey(t, "/"))
	if m.help || m.mode != "filter" {
		t.Fatal("unshifted slash did not open filter")
	}
}
func TestShiftedSymbolsKeepTheirActionsAndInputText(t *testing.T) {
	for _, wire := range []string{"+", "\x1b[61:43;2u", "\x1b[61;2;43u"} {
		m, _ := testModel()
		m.Update(decodeKey(t, wire))
		if m.UI.Layout != "content" {
			t.Fatal("shifted plus changed volume instead of layout")
		}
	}
	m, _ := testModel()
	m.mode = "query"
	for _, wire := range []string{"\x1b[47:63;2u", "\x1b[50;2;64u", "\x1b[61:43;2u"} {
		m.Update(decodeKey(t, wire))
	}
	if m.draft != "?@+" {
		t.Fatalf("shifted text corrupted: %q", m.draft)
	}
	m, _ = testModel()
	m.UI.Focus = 0
	before := m.navWidth()
	m.Update(decodeKey(t, "\x1b[108;6u"))
	if m.navWidth() <= before {
		t.Fatal("Ctrl+Shift+l lost resize behavior")
	}
	m.Update(decodeKey(t, "\x1b[61;5u"))
	if m.UI.NavWidth != 0 || m.UI.Layout != "full" {
		t.Fatal("Ctrl+= lost reset behavior")
	}
}
func TestKeyboardNegotiationPreservesPrintableTextInEveryView(t *testing.T) {
	// With all-keys-as-escape-codes but no shifted/associated text, some terminals
	// send CSI 47;2u for Shift+/, which can only be decoded as the base slash.
	// Keep printable input as text; request alternate codes for modified shortcuts.
	for _, mode := range []string{"normal", "help", "prompt", "quality", "small"} {
		m, _ := testModel()
		switch mode {
		case "help":
			m.help = true
		case "prompt":
			m.prompt = true
		case "quality":
			m.qualityDialog = true
		case "small":
			m.width = 40
		}
		flags := m.View().KeyboardEnhancements
		if flags.ReportAllKeysAsEscapeCodes || !flags.ReportAlternateKeys {
			t.Fatalf("%s view negotiates ambiguous shifted input", mode)
		}
	}
}

func TestCtrlShiftVimKeysResizeTheFocusedPanel(t *testing.T) {
	for _, wire := range []string{"\x1b[104;6u", "\x1b[106;6u", "\x1b[107;6u", "\x1b[108:76;6u"} {
		m, _ := testModel()
		m.UI.Focus = 0
		m.Update(decodeKey(t, wire))
		if m.UI.Focus != 0 || (m.UI.NavWidth == 0 && m.UI.NavHeight == 0) {
			t.Fatalf("Ctrl+Shift+hjkl moved focus or did not resize: %q", wire)
		}
	}
}

func TestTerminalControlVimKeysRemainDistinctFromBackspaceAndEnter(t *testing.T) {
	m, _ := testModel()
	for _, step := range []struct {
		wire  string
		focus int
	}{{"\x1b[108;5u", 2}, {"\x1b[106;5u", 3}, {"\x1b[107;5u", 2}, {"\x1b[104;5u", 1}} {
		m.Update(decodeKey(t, step.wire))
		if m.UI.Focus != step.focus || m.prompt {
			t.Fatalf("control key lost focus navigation: %q", step.wire)
		}
	}
}
