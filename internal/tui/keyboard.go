package tui

import (
	tea "charm.land/bubbletea/v2"
	"unicode"
)

// KeyString keeps printable Shift symbols as text, but routes control chords
// by physical key and modifiers. Associated text in CSI-u must not turn a
// modified shortcut into a filter/search character.
func KeyString(msg tea.KeyPressMsg) string {
	k := tea.Key(msg)
	if k.Mod.Contains(tea.ModCtrl) || k.Mod.Contains(tea.ModAlt) || k.Mod.Contains(tea.ModSuper) {
		// Ctrl+< and Ctrl+> are also encoded as Ctrl+Shift+comma/period.
		if k.Mod.Contains(tea.ModCtrl) && !k.Mod.Contains(tea.ModAlt) && !k.Mod.Contains(tea.ModSuper) {
			if k.Code == '<' || (k.Mod.Contains(tea.ModShift) && (k.Code == ',' || k.ShiftedCode == '<')) {
				return "ctrl+<"
			}
			if k.Code == '>' || (k.Mod.Contains(tea.ModShift) && (k.Code == '.' || k.ShiftedCode == '>')) {
				return "ctrl+>"
			}
		}
		k.Text = ""
		if k.Mod.Contains(tea.ModCtrl) && k.Mod.Contains(tea.ModShift) && unicode.IsLetter(k.Code) {
			k.Code = unicode.ToLower(k.Code)
			if k.BaseCode != 0 {
				k.BaseCode = unicode.ToLower(k.BaseCode)
			}
		}
		return k.Keystroke()
	}
	return msg.String()
}
