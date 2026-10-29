package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Keys map[string][]string
type keyDefinition struct {
	action, scopes, description string
	bindings                    []string
}

// Dispatch, help and validation share this ordered registry. Panel bindings may
// reuse keys, but cannot shadow Global bindings.
var keyDefinitions = []keyDefinition{
	{"left", "Global", "Focus left panel", []string{"ctrl+h"}},
	{"right", "Global", "Focus right panel", []string{"ctrl+l"}},
	{"panel_down", "Global", "Focus lower panel", []string{"ctrl+j"}},
	{"panel_up", "Global", "Focus upper panel", []string{"ctrl+k"}},
	{"focus_next", "Global", "Next panel", []string{"tab"}},
	{"focus_previous", "Global", "Previous panel", []string{"shift+tab"}},
	{"focus_1", "Global", "Navigation", []string{"1"}},
	{"focus_2", "Global", "Content", []string{"2"}},
	{"focus_3", "Global", "Queue", []string{"3"}},
	{"focus_4", "Global", "Player", []string{"4"}},
	{"focus_5", "Global", "Log", []string{"5"}},
	{"focus_6", "Global", "Sound", []string{"6"}},
	{"toggle", "Global", "Play / pause", []string{"space"}},
	{"volume_up", "Global", "System volume +5%", []string{"="}},
	{"volume_down", "Global", "System volume -5%", []string{"-"}},
	{"resize_up", "Global", "Resize up", []string{"K"}},
	{"resize_down", "Global", "Resize down", []string{"J"}},
	{"resize_left", "Global", "Resize left", []string{"H"}},
	{"resize_right", "Global", "Resize right", []string{"L"}},
	{"layout_reset", "Global", "Reset layout", []string{"ctrl+="}},
	{"zen", "Global", "Toggle Zen", []string{"ctrl++", "ctrl+shift++", "ctrl+shift+=", "ctrl+-", "ctrl+_", "ctrl+shift+_", "ctrl+shift+-"}},
	{"layout_next", "Global", "Next layout", []string{"+", "shift+="}},
	{"layout_previous", "Global", "Previous layout", []string{"_", "shift+-"}},
	{"logs", "Global", "Toggle runtime log", []string{"@"}},
	{"login", "Global", "Login / switch source", nil},
	{"help", "Global", "Help for current panel", []string{"?"}},
	{"leave", "Global", "Leave: q background / Q quit", []string{"q", "Q", "ctrl+c"}},
	{"command", "Global", "Command line (:seek, :vol, :play…)", []string{":"}},
	{"down", "Navigation Content Queue Log Login Dialog Sound", "Move down", []string{"j", "down"}},
	{"up", "Navigation Content Queue Log Login Dialog Sound", "Move up", []string{"k", "up"}},
	{"top", "Navigation Content Queue Log Dialog", "First item", []string{"home", "gg"}},
	{"bottom", "Navigation Content Queue Log Dialog", "Last item", []string{"G", "end"}},
	{"page_up", "Navigation Content Queue Log Dialog", "Half page up", []string{"ctrl+u", "pgup"}},
	{"page_down", "Navigation Content Queue Log Dialog", "Half page down", []string{"ctrl+d", "pgdown"}},
	{"open", "Navigation Content Login Dialog Sound", "Open / enter child", []string{"l", "right"}},
	{"activate", "Navigation Content Queue Player Login Dialog", "Open / play selection", []string{"enter"}},
	{"back", "Content Login Dialog Sound", "Back / parent / cancel", []string{"h", "left", "esc", "backspace"}},
	{"filter", "Content", "Filter loaded items", []string{"/"}},
	{"query", "Content", "Edit Search query", []string{"i"}},
	{"scope", "Content", "Switch Search catalog / library", []string{"s"}},
	{"category_next", "Content Navigation Player Sound", "Next category / mode / visualization", []string{"."}},
	{"category_previous", "Content Navigation Player Sound", "Previous category / mode / visualization", []string{","}},
	{"refresh", "Content", "Refresh content", []string{"r"}},
	{"login_retry", "Login", "Sign in again (account options)", []string{"r"}},
	{"add", "Content Queue Player", "Add to queue / library / playlist", []string{"a"}},
	{"enqueue", "Content", "Copy selection to queue", []string{"y"}},
	{"play_next", "Content", "Play selection next", []string{"P"}},
	{"play_selection", "Content Queue", "Play song / entire collection", []string{"p"}},
	{"remove", "Content Queue", "Remove selected item", []string{"d"}},
	{"clear", "Queue", "Clear playback queue", []string{"D"}},
	{"queue_up", "Queue", "Move song up", []string{"["}},
	{"queue_down", "Queue", "Move song down", []string{"]"}},
	{"related", "Content Queue Player Dialog", "Go to album / artist", []string{"o"}},
	{"credits", "Content Queue Player Dialog", "View credits", []string{"c"}},
	{"playback_mode", "Global", "Cycle playback mode", []string{"m"}},
	{"stop", "Global", "Stop playback", []string{"x"}},
	{"next", "Global", "Next song", []string{">"}},
	{"previous", "Global", "Previous song", []string{"<"}},
	{"seek_forward", "Global", "Seek forward 10s", []string{"ctrl+>"}},
	{"seek_backward", "Global", "Seek backward 10s", []string{"ctrl+<"}},
	{"repeat", "Queue Player", "Cycle repeat mode", []string{"r"}},
	{"shuffle", "Queue Player", "Toggle shuffle", []string{"s"}},
	{"quality", "Player", "Choose playback quality", []string{"b"}},
	{"eq_toggle", "Sound", "Toggle equalizer bypass", []string{"e"}},
	{"eq_reset", "Sound", "Reset equalizer to flat", []string{"r"}},
	{"eq_preset", "Sound", "Next equalizer preset", []string{"p"}},
}

func inScope(d keyDefinition, scope string) bool {
	for _, s := range strings.Fields(d.scopes) {
		if s == scope {
			return true
		}
	}
	return false
}
func DefaultKeys() Keys {
	k := Keys{}
	for _, d := range keyDefinitions {
		k[d.action] = append([]string(nil), d.bindings...)
	}
	return k
}
func LoadKeys(dir string) (Keys, error) {
	keys := DefaultKeys()
	b, err := os.ReadFile(filepath.Join(dir, "keys.json"))
	if os.IsNotExist(err) {
		return keys, nil
	}
	if err != nil {
		return nil, err
	}
	var custom Keys
	if err = json.Unmarshal(b, &custom); err != nil {
		return nil, fmt.Errorf("keys.json: %w", err)
	}
	for action, value := range custom {
		if _, ok := keys[action]; !ok {
			return nil, fmt.Errorf("unknown key action %q", action)
		}
		keys[action] = value
	}
	for _, scope := range []string{"Navigation", "Content", "Queue", "Player", "Log", "Sound", "Login", "Dialog"} {
		seen := map[string]string{}
		for _, d := range keyDefinitions {
			if !inScope(d, scope) && !inScope(d, "Global") {
				continue
			}
			for _, key := range keys[d.action] {
				if key == "" {
					return nil, fmt.Errorf("empty key for %s", d.action)
				}
				if old, ok := seen[key]; ok && old != d.action {
					return nil, fmt.Errorf("key %q conflicts in %s: %s and %s", key, scope, old, d.action)
				}
				seen[key] = d.action
			}
		}
	}
	return keys, nil
}
func (k Keys) ActionFor(scope, key string) string {
	for _, d := range keyDefinitions {
		if !inScope(d, scope) && !inScope(d, "Global") {
			continue
		}
		for _, candidate := range k[d.action] {
			if candidate == key {
				return d.action
			}
		}
	}
	return ""
}

func (m *Model) keyScope() string {
	if m.UI.Focus == 1 && m.LoginSelected() {
		return "Login"
	}
	return []string{"Navigation", "Content", "Queue", "Player", "Log", "Sound"}[max(0, min(5, m.UI.Focus))]
}
